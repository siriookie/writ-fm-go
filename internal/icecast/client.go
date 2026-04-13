// Package icecast provides a thin client for reading Icecast server stats.
package icecast

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client fetches stats from an Icecast server.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient returns a Client targeting the Icecast server at baseURL
// (e.g. "http://localhost:8000"). Trailing slashes are stripped.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Listeners returns the current listener count for mountpoint (e.g. "/stream").
// Returns 0 without error if the mountpoint is not currently active.
func (c *Client) Listeners(mountpoint string) (int, error) {
	if !strings.HasPrefix(mountpoint, "/") {
		mountpoint = "/" + mountpoint
	}

	url := c.baseURL + "/status-json.xsl"
	resp, err := c.http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("icecast: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("icecast: status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("icecast: read body: %w", err)
	}

	return parseListeners(body, mountpoint)
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
