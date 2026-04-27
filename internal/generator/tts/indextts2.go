package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultIndexTTS2Timeout = 180 * time.Second

type indexTTS2Request struct {
	Text       string  `json:"text"`
	Voice      string  `json:"voice"`
	EmoText    string  `json:"emo_text,omitempty"`
	UseEmoText bool    `json:"use_emo_text,omitempty"`
	EmoAlpha   float64 `json:"emo_alpha,omitempty"`
	UseRandom  bool    `json:"use_random,omitempty"`
}

// IndexTTS2TTS synthesizes speech by calling a Modal-hosted IndexTTS2 HTTP endpoint.
type IndexTTS2TTS struct {
	endpoint   string
	timeout    time.Duration
	httpClient *http.Client
}

// NewIndexTTS2TTS returns an HTTP client for a Modal-hosted IndexTTS2 endpoint.
func NewIndexTTS2TTS(endpoint string) *IndexTTS2TTS {
	return &IndexTTS2TTS{
		endpoint: strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		timeout:  defaultIndexTTS2Timeout,
		httpClient: &http.Client{
			Timeout: defaultIndexTTS2Timeout,
		},
	}
}

// Synthesize sends text + voice to the IndexTTS2 endpoint and writes raw WAV output to dst.
func (k *IndexTTS2TTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return ErrEmptyText
	}
	if dst == nil {
		return fmt.Errorf("generator/tts: destination writer is required")
	}
	if k.endpoint == "" {
		return fmt.Errorf("generator/tts: IndexTTS2 endpoint is required (set INDEXTTS2_MODAL_URL)")
	}

	body, err := json.Marshal(indexTTS2Request{
		Text:  text,
		Voice: strings.TrimSpace(voice),
	})
	if err != nil {
		return fmt.Errorf("generator/tts: marshal IndexTTS2 request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("generator/tts: create IndexTTS2 request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("generator/tts: IndexTTS2 request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("generator/tts: IndexTTS2 returned %d: %s",
			resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("generator/tts: read IndexTTS2 response: %w", err)
	}
	if len(audio) == 0 {
		return fmt.Errorf("generator/tts: IndexTTS2 returned empty audio")
	}
	if _, err := io.Copy(dst, bytes.NewReader(audio)); err != nil {
		return fmt.Errorf("generator/tts: write output: %w", err)
	}
	return nil
}
