package news

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	defaultCacheTTL = 10 * time.Minute
	defaultTimeout  = 6 * time.Second
	defaultMaxItems = 8
)

var defaultFeeds = []string{
	"https://feeds.bbci.co.uk/zhongwen/simp/rss.xml",
}

// Headline is one normalized feed item.
type Headline struct {
	Title  string
	Source string
}

// Client fetches, caches, and normalizes RSS/Atom headlines.
type Client struct {
	feeds      []string
	maxItems   int
	cacheTTL   time.Duration
	timeout    time.Duration
	httpClient *http.Client
	now        func() time.Time

	mu         sync.Mutex
	cache      cacheEntry
	refreshing bool
}

type cacheEntry struct {
	items     []Headline
	fetchedAt time.Time
}

// Option customizes a Client.
type Option func(*Client)

// WithFeeds overrides the feed list.
func WithFeeds(feeds []string) Option {
	return func(c *Client) {
		c.feeds = append([]string(nil), feeds...)
	}
}

// WithMaxItems overrides the fetch cap.
func WithMaxItems(maxItems int) Option {
	return func(c *Client) {
		if maxItems > 0 {
			c.maxItems = maxItems
		}
	}
}

// WithCacheTTL overrides the cache TTL.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		if ttl > 0 {
			c.cacheTTL = ttl
		}
	}
}

// WithTimeout overrides the per-feed timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

// WithHTTPClient injects a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithClock injects a custom clock for tests.
func WithClock(now func() time.Time) Option {
	return func(c *Client) {
		if now != nil {
			c.now = now
		}
	}
}

// NewClient returns a news client with production defaults.
func NewClient(opts ...Option) *Client {
	c := &Client{
		feeds:      append([]string(nil), defaultFeeds...),
		maxItems:   defaultMaxItems,
		cacheTTL:   defaultCacheTTL,
		timeout:    defaultTimeout,
		httpClient: &http.Client{},
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.feeds = compactFeeds(c.feeds)
	if len(c.feeds) == 0 {
		c.feeds = append([]string(nil), defaultFeeds...)
	}
	return c
}

// FetchHeadlines returns headlines from cache or refreshes them as needed.
func (c *Client) FetchHeadlines(ctx context.Context) ([]Headline, error) {
	now := c.now()

	c.mu.Lock()
	cached := cloneHeadlines(c.cache.items)
	stale := len(cached) == 0 || now.Sub(c.cache.fetchedAt) >= c.cacheTTL
	if !stale {
		c.mu.Unlock()
		return cached, nil
	}
	if len(cached) > 0 {
		if !c.refreshing {
			c.refreshing = true
			go c.refreshInBackground()
		}
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	items, err := c.fetchFresh(ctx)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Client) refreshInBackground() {
	defer func() {
		c.mu.Lock()
		c.refreshing = false
		c.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	_, _ = c.fetchFresh(ctx)
}

func (c *Client) fetchFresh(ctx context.Context) ([]Headline, error) {
	results := make(chan fetchResult, len(c.feeds))
	var wg sync.WaitGroup

	for _, feedURL := range c.feeds {
		feedURL := feedURL
		wg.Go(func() {
			items, err := c.fetchFeed(ctx, feedURL)
			results <- fetchResult{items: items, err: err}
		})
	}

	wg.Wait()
	close(results)

	seen := make(map[string]struct{}, c.maxItems)
	headlines := make([]Headline, 0, c.maxItems)
	var failures []error

	for result := range results {
		if result.err != nil {
			failures = append(failures, result.err)
			continue
		}
		for _, item := range result.items {
			key := normalizeTitle(item.Title)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			headlines = append(headlines, item)
			if len(headlines) >= c.maxItems {
				break
			}
		}
	}

	if len(headlines) == 0 {
		return nil, errorsJoin(failures)
	}

	c.mu.Lock()
	c.cache = cacheEntry{
		items:     cloneHeadlines(headlines),
		fetchedAt: c.now(),
	}
	c.mu.Unlock()

	return headlines, nil
}

type fetchResult struct {
	items []Headline
	err   error
}

func (c *Client) fetchFeed(ctx context.Context, feedURL string) ([]Headline, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("news: build request for %s: %w", feedURL, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("news: fetch %s: %w", feedURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("news: fetch %s: status %d", feedURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("news: read %s: %w", feedURL, err)
	}

	items, err := parseHeadlines(body, fallbackSource(feedURL))
	if err != nil {
		return nil, fmt.Errorf("news: parse %s: %w", feedURL, err)
	}
	return items, nil
}

func parseHeadlines(data []byte, fallback string) ([]Headline, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))

	var (
		stack          []string
		sourceTitle    strings.Builder
		entryTitle     strings.Builder
		inEntry        bool
		headlines      []Headline
		resolvedSource string
	)

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch token := tok.(type) {
		case xml.StartElement:
			name := token.Name.Local
			stack = append(stack, name)
			if name == "item" || name == "entry" {
				inEntry = true
				entryTitle.Reset()
			}
		case xml.EndElement:
			name := token.Name.Local
			if (name == "item" || name == "entry") && inEntry {
				title := strings.TrimSpace(entryTitle.String())
				if title != "" {
					source := strings.TrimSpace(resolvedSource)
					if source == "" {
						source = strings.TrimSpace(sourceTitle.String())
					}
					if source == "" {
						source = fallback
					}
					headlines = append(headlines, Headline{
						Title:  title,
						Source: source,
					})
				}
				inEntry = false
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if name == "channel" || name == "feed" {
				if strings.TrimSpace(sourceTitle.String()) != "" {
					resolvedSource = strings.TrimSpace(sourceTitle.String())
				}
			}
		case xml.CharData:
			text := strings.TrimSpace(string(token))
			if text == "" || len(stack) < 2 || stack[len(stack)-1] != "title" {
				continue
			}
			parent := stack[len(stack)-2]
			switch {
			case inEntry && (parent == "item" || parent == "entry"):
				entryTitle.WriteString(text)
			case !inEntry && (parent == "channel" || parent == "feed"):
				sourceTitle.WriteString(text)
			}
		}
	}

	if len(headlines) == 0 {
		return nil, fmt.Errorf("no headlines found")
	}
	return headlines, nil
}

func normalizeTitle(title string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			b.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func fallbackSource(feedURL string) string {
	parsed, err := url.Parse(feedURL)
	if err != nil || parsed.Host == "" {
		return "Unknown Source"
	}
	return parsed.Host
}

func compactFeeds(feeds []string) []string {
	out := make([]string, 0, len(feeds))
	for _, feed := range feeds {
		feed = strings.TrimSpace(feed)
		if feed != "" {
			out = append(out, feed)
		}
	}
	return out
}

func cloneHeadlines(src []Headline) []Headline {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Headline, len(src))
	copy(dst, src)
	return dst
}

func errorsJoin(errs []error) error {
	filtered := errs[:0]
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	return errors.Join(filtered...)
}
