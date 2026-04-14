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

const defaultKokoroModalTimeout = 120 * time.Second

// kokoroModalRequest is the JSON body sent to the Modal Kokoro endpoint.
type kokoroModalRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
}

// KokoroModalTTS synthesizes speech by calling a Modal-hosted Kokoro HTTP endpoint.
// The endpoint is expected to accept POST {"text": "...", "voice": "..."} and
// return raw WAV bytes with Content-Type: audio/wav.
//
// Use NewKokoroModalTTS to construct. The endpoint URL comes from the
// KOKORO_MODAL_URL environment variable and is injected via cmd/generator/config.go.
type KokoroModalTTS struct {
	endpoint   string
	timeout    time.Duration
	httpClient *http.Client
}

// NewKokoroModalTTS returns a Modal Kokoro HTTP client.
// endpoint must be the full HTTPS URL of the Modal web endpoint, e.g.
// "https://your-org--writ-fm-kokoro-synthesize-http.modal.run".
func NewKokoroModalTTS(endpoint string) *KokoroModalTTS {
	return &KokoroModalTTS{
		endpoint: strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		timeout:  defaultKokoroModalTimeout,
		httpClient: &http.Client{
			Timeout: defaultKokoroModalTimeout,
		},
	}
}

// Synthesize sends text + voice to the Modal endpoint and streams the WAV
// response to dst. Satisfies tts.Client.
func (k *KokoroModalTTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return ErrEmptyText
	}
	if dst == nil {
		return fmt.Errorf("generator/tts: destination writer is required")
	}
	if k.endpoint == "" {
		return fmt.Errorf("generator/tts: KokoroModal endpoint is required (set KOKORO_MODAL_URL)")
	}

	body, err := json.Marshal(kokoroModalRequest{Text: text, Voice: voice})
	if err != nil {
		return fmt.Errorf("generator/tts: marshal KokoroModal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("generator/tts: create KokoroModal request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("generator/tts: KokoroModal request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("generator/tts: KokoroModal returned %d: %s",
			resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("generator/tts: read KokoroModal response: %w", err)
	}
	if len(audio) == 0 {
		return fmt.Errorf("generator/tts: KokoroModal returned empty audio")
	}

	if _, err := io.Copy(dst, bytes.NewReader(audio)); err != nil {
		return fmt.Errorf("generator/tts: write output: %w", err)
	}
	return nil
}
