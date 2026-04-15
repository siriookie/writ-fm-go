package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultMimoTimeout = 60 * time.Second
	defaultMimoBaseURL = "https://api.xiaomimimo.com/v1/chat/completions"
	defaultMimoModel   = "mimo-v2-tts"
)

var validMimoVoices = map[string]struct{}{
	"mimo_default": {},
	"default_en":   {},
	"default_zh":   {},
}

var mimoVoiceAliases = map[string]string{
	// Chinese / warm-female leaning aliases.
	"af_bella":    "default_zh",
	"af_heart":    "default_zh",
	"zf_xiaoyi":   "default_zh",
	"zf_xiaobei":  "default_zh",
	"zh_xiaoxiao": "default_zh",
	"jennifer":    "default_zh",
	"cherry":      "default_zh",
	"longtong":    "default_zh",
	"longhua":     "default_zh",
	"default_zh":  "default_zh",

	// Chinese / lower-male leaning aliases.
	"am_michael":   "mimo_default",
	"am_onyx":      "mimo_default",
	"bm_daniel":    "mimo_default",
	"zm_yunjian":   "mimo_default",
	"zh_yunxi":     "mimo_default",
	"elias":        "mimo_default",
	"ryan":         "mimo_default",
	"longjielidou": "mimo_default",
	"mimo_default": "mimo_default",

	// English fallback aliases.
	"default_en": "default_en",
}

// MimoTTS synthesizes speech through Xiaomi Mimo's OpenAI-compatible API.
type MimoTTS struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

type mimoRequest struct {
	Model    string        `json:"model"`
	Messages []mimoMessage `json:"messages"`
	Audio    mimoAudio     `json:"audio"`
}

type mimoMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mimoAudio struct {
	Format string `json:"format"`
	Voice  string `json:"voice"`
}

type mimoResponse struct {
	Choices []struct {
		Message struct {
			Audio *struct {
				Data string `json:"data"`
			} `json:"audio,omitempty"`
			ErrorMessage string `json:"error_message,omitempty"`
		} `json:"message"`
	} `json:"choices"`
}

// NewMimoTTS returns a Mimo TTS adapter with sane defaults.
func NewMimoTTS(apiKey, baseURL, model string) *MimoTTS {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultMimoBaseURL
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultMimoModel
	}
	return &MimoTTS{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: defaultMimoTimeout,
		},
	}
}

// Synthesize renders text to audio and writes the decoded bytes to dst.
func (m *MimoTTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return ErrEmptyText
	}
	if dst == nil {
		return fmt.Errorf("generator/tts: destination writer is required")
	}
	if m.apiKey == "" {
		return fmt.Errorf("generator/tts: Mimo API key is required")
	}

	reqBody, err := json.Marshal(mimoRequest{
		Model: m.model,
		Messages: []mimoMessage{
			{
				Role:    "assistant",
				Content: text,
			},
		},
		Audio: mimoAudio{
			Format: "wav",
			Voice:  mapMimoVoice(text, voice),
		},
	})
	if err != nil {
		return fmt.Errorf("generator/tts: marshal Mimo request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("generator/tts: create Mimo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("generator/tts: Mimo request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("generator/tts: read Mimo response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("generator/tts: Mimo returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded mimoResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("generator/tts: decode Mimo response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return fmt.Errorf("generator/tts: Mimo returned no choices")
	}
	if decoded.Choices[0].Message.ErrorMessage != "" {
		return fmt.Errorf("generator/tts: Mimo error: %s", decoded.Choices[0].Message.ErrorMessage)
	}
	if decoded.Choices[0].Message.Audio == nil || strings.TrimSpace(decoded.Choices[0].Message.Audio.Data) == "" {
		return fmt.Errorf("generator/tts: Mimo returned empty audio")
	}

	audio, err := base64.StdEncoding.DecodeString(decoded.Choices[0].Message.Audio.Data)
	if err != nil {
		return fmt.Errorf("generator/tts: decode Mimo audio: %w", err)
	}
	if len(audio) == 0 {
		return fmt.Errorf("generator/tts: Mimo returned empty decoded audio")
	}
	if _, err := io.Copy(dst, bytes.NewReader(audio)); err != nil {
		return fmt.Errorf("generator/tts: write output: %w", err)
	}
	return nil
}

func mapMimoVoice(text, voice string) string {
	voice = strings.TrimSpace(voice)
	if _, ok := validMimoVoices[voice]; ok {
		return voice
	}
	if mapped, ok := mimoVoiceAliases[strings.ToLower(voice)]; ok {
		return mapped
	}

	if containsHan(text) {
		return "default_zh"
	}
	if strings.Contains(strings.ToLower(voice), "zh") {
		return "default_zh"
	}
	return "default_en"
}

func containsHan(text string) bool {
	for _, r := range text {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF:
			return true
		case r >= 0x3400 && r <= 0x4DBF:
			return true
		case r >= 0x3000 && r <= 0x303F:
			return true
		}
	}
	return false
}
