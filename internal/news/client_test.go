package news

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientFetchHeadlinesParsesRSSAndAtomAndDedupes(t *testing.T) {
	t.Parallel()

	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss><channel><title>BBC 中文</title>
<item><title>同一条新闻</title><description><![CDATA[<p>第一条新闻的正文摘要。</p>]]></description><link>https://example.com/one</link><pubDate>Mon, 27 Apr 2026 10:00:00 GMT</pubDate></item>
<item><title>另一条新闻</title></item>
</channel></rss>`)
	}))
	t.Cleanup(rss.Close)

	atom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>NPR</title>
<entry><title>同一条新闻</title></entry>
<entry><title>第三条新闻</title></entry>
</feed>`)
	}))
	t.Cleanup(atom.Close)

	client := NewClient(
		WithFeeds([]string{rss.URL, atom.URL}),
		WithHTTPClient(rss.Client()),
		WithMaxItems(8),
	)
	client.httpClient = atom.Client()

	headlines, err := client.FetchHeadlines(context.Background())
	if err != nil {
		t.Fatalf("FetchHeadlines() error = %v", err)
	}
	if len(headlines) != 3 {
		t.Fatalf("len(headlines) = %d, want 3", len(headlines))
	}
	if headlines[0].Source != "BBC 中文" {
		t.Fatalf("headlines[0].Source = %q, want BBC 中文", headlines[0].Source)
	}
	if headlines[0].Summary != "第一条新闻的正文摘要。" {
		t.Fatalf("headlines[0].Summary = %q", headlines[0].Summary)
	}
	if headlines[0].Link != "https://example.com/one" {
		t.Fatalf("headlines[0].Link = %q", headlines[0].Link)
	}
	if headlines[0].Published == "" {
		t.Fatalf("headlines[0].Published is empty")
	}
	if headlines[2].Source != "NPR" {
		t.Fatalf("headlines[2].Source = %q, want NPR", headlines[2].Source)
	}
}

func TestClientFetchHeadlinesReturnsStaleThenRefreshes(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		switch call {
		case 1:
			fmt.Fprint(w, `<?xml version="1.0"?><rss><channel><title>Feed</title><item><title>旧标题</title></item></channel></rss>`)
		default:
			fmt.Fprint(w, `<?xml version="1.0"?><rss><channel><title>Feed</title><item><title>新标题</title></item></channel></rss>`)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		WithFeeds([]string{server.URL}),
		WithHTTPClient(server.Client()),
		WithCacheTTL(time.Minute),
		WithClock(func() time.Time { return now }),
	)

	first, err := client.FetchHeadlines(context.Background())
	if err != nil {
		t.Fatalf("first FetchHeadlines() error = %v", err)
	}
	if first[0].Title != "旧标题" {
		t.Fatalf("first title = %q, want 旧标题", first[0].Title)
	}

	now = now.Add(2 * time.Minute)
	second, err := client.FetchHeadlines(context.Background())
	if err != nil {
		t.Fatalf("second FetchHeadlines() error = %v", err)
	}
	if second[0].Title != "旧标题" {
		t.Fatalf("second title = %q, want stale 旧标题", second[0].Title)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		client.mu.Lock()
		refreshed := len(client.cache.items) > 0 && client.cache.items[0].Title == "新标题"
		client.mu.Unlock()
		if refreshed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("background refresh did not update cache")
}

func TestClientFetchHeadlinesFallsBackWhenOneFeedFails(t *testing.T) {
	t.Parallel()

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0"?><rss><channel><title>Feed</title><item><title>可用标题</title></item></channel></rss>`)
	}))
	t.Cleanup(okServer.Close)

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(badServer.Close)

	client := NewClient(
		WithFeeds([]string{badServer.URL, okServer.URL}),
		WithHTTPClient(okServer.Client()),
	)

	headlines, err := client.FetchHeadlines(context.Background())
	if err != nil {
		t.Fatalf("FetchHeadlines() error = %v", err)
	}
	if len(headlines) != 1 || headlines[0].Title != "可用标题" {
		t.Fatalf("unexpected headlines = %#v", headlines)
	}
}

func TestClientFetchArticleExtractsLinkedPageParagraphs(t *testing.T) {
	t.Parallel()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			fmt.Fprintf(w, `<?xml version="1.0"?><rss><channel><title>Feed</title><item><title>Selected story</title><description>RSS short summary.</description><link>%s/article</link></item></channel></rss>`, serverURL)
		case "/article":
			fmt.Fprint(w, `<html><head><script>ignore()</script></head><body><article><p>First real article paragraph with enough detail to keep.</p><p>Second paragraph names agencies, timeline, and consequences.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	serverURL = server.URL
	t.Cleanup(server.Close)

	client := NewClient(
		WithFeeds([]string{server.URL + "/feed"}),
		WithHTTPClient(server.Client()),
	)
	headlines, err := client.FetchHeadlines(context.Background())
	if err != nil {
		t.Fatalf("FetchHeadlines() error = %v", err)
	}
	enriched, err := client.FetchArticle(context.Background(), headlines[0])
	if err != nil {
		t.Fatalf("FetchArticle() error = %v", err)
	}
	if !strings.Contains(enriched.Content, "First real article paragraph") || !strings.Contains(enriched.Content, "Second paragraph names agencies") {
		t.Fatalf("enriched content = %q", enriched.Content)
	}
}
