// Package icecast provides a thin client for reading Icecast server stats.
package icecast

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultCacheTTL = 15 * time.Second

// rawCache holds the last-fetched Icecast status response body.
type rawCache struct {
	body []byte
	ts   time.Time
}

// Client fetches stats from an Icecast server.
type Client struct {
	baseURL  string
	http     *http.Client
	cacheTTL time.Duration

	mu  sync.Mutex
	raw *rawCache // cached /status-json.xsl body (nil = never fetched)
}

// NewClient returns a Client targeting the Icecast server at baseURL
// (e.g. "http://localhost:8000"). Trailing slashes are stripped.
func NewClient(baseURL string) *Client {
	return newClientWithTTL(baseURL, defaultCacheTTL)
}

// newClientWithTTL is used in tests to control the cache TTL.
func newClientWithTTL(baseURL string, ttl time.Duration) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		http:     &http.Client{Timeout: 5 * time.Second},
		cacheTTL: ttl,
	}
}

// Listeners returns the current listener count for mountpoint (e.g. "/stream").
// The underlying HTTP response is cached for cacheTTL so all mountpoints share
// one request per interval. On fetch failure, the last cached body is used
// (if any), so callers degrade gracefully.
// Returns 0 without error if the mountpoint is not currently active.
func (c *Client) Listeners(mountpoint string) (int, error) {
	if !strings.HasPrefix(mountpoint, "/") {
		mountpoint = "/" + mountpoint
	}

	c.mu.Lock()
	cached := c.raw
	c.mu.Unlock()

	if cached != nil && time.Since(cached.ts) < c.cacheTTL {
		return parseListeners(cached.body, mountpoint)
	}

	body, err := c.fetchBody()
	if err != nil {
		if cached != nil {
			// Fallback to stale cache rather than surfacing a transient error.
			return parseListeners(cached.body, mountpoint)
		}
		return 0, err
	}

	c.mu.Lock()
	c.raw = &rawCache{body: body, ts: time.Now()}
	c.mu.Unlock()

	return parseListeners(body, mountpoint)
}

// fetchBody performs the actual HTTP call and returns the raw response body.
func (c *Client) fetchBody() ([]byte, error) {
	url := c.baseURL + "/status-json.xsl"
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("icecast: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("icecast: status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("icecast: read body: %w", err)
	}
	return body, nil
}

// sourceInfo holds the per-source fields we care about.
type sourceInfo struct {
	ListenURL string `json:"listenurl"`
	Listeners int    `json:"listeners"`
}

// icestatResponse is the top-level Icecast JSON envelope.
// "source" may be absent, a single object, or an array of objects.
type icestatResponse struct {
	IceStats struct {
		Source json.RawMessage `json:"source"`
	} `json:"icestats"`
}

func parseListeners(body []byte, mountpoint string) (int, error) {
	var top icestatResponse
	if err := json.Unmarshal(body, &top); err != nil {
		return 0, fmt.Errorf("icecast: parse JSON: %w", err)
	}

	raw := top.IceStats.Source
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil // no active sources
	}

	// Icecast returns a single object when there is one source, an array otherwise.
	var sources []sourceInfo
	if err := json.Unmarshal(raw, &sources); err != nil {
		var single sourceInfo
		if err2 := json.Unmarshal(raw, &single); err2 != nil {
			return 0, fmt.Errorf("icecast: parse source field: %w", err)
		}
		sources = []sourceInfo{single}
	}

	for _, s := range sources {
		if strings.HasSuffix(s.ListenURL, mountpoint) {
			return s.Listeners, nil
		}
	}
	return 0, nil // mountpoint not found among active sources
}
