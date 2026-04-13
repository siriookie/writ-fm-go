// Package musicgen provides a client for the ACE-Step music generation server
// and a bumper generator that writes audio files and sidecar metadata JSON.
package musicgen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GenerateRequest is the payload sent to POST /generate.
type GenerateRequest struct {
	Caption        string  `json:"caption"`
	Duration       float64 `json:"duration"`
	AudioFormat    string  `json:"audio_format"`
	Seed           int     `json:"seed"`
	Instrumental   bool    `json:"instrumental"`
	Lyrics         string  `json:"lyrics,omitempty"`
	GuidanceScale  float64 `json:"guidance_scale"`
	InferenceSteps int     `json:"inference_steps"`
	Thinking       bool    `json:"thinking"`
	// Always send these so ACE-Step never receives nil (metadata_utils.py bug).
	Keyscale      string `json:"keyscale"`
	TimeSignature string `json:"timesignature"`
}

// generateResponse is the server's response envelope.
type generateResponse struct {
	Audios []string `json:"audios"`
}

// Client calls the ACE-Step music generation HTTP server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a Client pointing at baseURL (e.g. "http://localhost:4009").
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 600 * time.Second,
		},
	}
}

// Generate sends a generation request and returns the decoded audio bytes.
func (c *Client) Generate(ctx context.Context, req GenerateRequest) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("musicgen: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("musicgen: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("musicgen: generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("musicgen: server returned %d: %s", resp.StatusCode, preview)
	}

	var gr generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, fmt.Errorf("musicgen: decode response: %w", err)
	}
	if len(gr.Audios) == 0 {
		return nil, fmt.Errorf("musicgen: server returned empty audios array")
	}

	audio, err := base64.StdEncoding.DecodeString(gr.Audios[0])
	if err != nil {
		return nil, fmt.Errorf("musicgen: decode base64 audio: %w", err)
	}
	return audio, nil
}

// Health checks that the generation server is reachable.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("musicgen: health request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("musicgen: health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("musicgen: health check failed: status %d", resp.StatusCode)
	}
	return nil
}
