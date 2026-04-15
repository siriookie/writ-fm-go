package news

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientFetchHeadlinesParsesRSSAndAtomAndDedupes(t *testing.T) {
	t.Parallel()

	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss><channel><title>BBC 中文</title>
<item><title>同一条新闻</title></item>
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
