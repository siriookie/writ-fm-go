package tts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultMicrosoftTimeout      = 60 * time.Second
	defaultMicrosoftOutputFormat = "riff-24khz-16bit-mono-pcm"
)

var defaultMicrosoftVoices = map[string]string{
	"am_michael":  "zh-CN-YunxiNeural",
	"bm_daniel":   "zh-CN-YunxiNeural",
	"af_heart":    "zh-CN-XiaoxiaoNeural",
	"am_onyx":     "zh-CN-YunxiNeural",
	"af_bella":    "zh-CN-XiaoxiaoNeural",
	"zh_xiaoxiao": "zh-CN-XiaoxiaoNeural",
	"zh_yunxi":    "zh-CN-YunxiNeural",
}

// MicrosoftTTS synthesizes speech via Azure Speech REST.
type MicrosoftTTS struct {
	apiKey     string
	region     string
	baseURL    string
	httpClient *http.Client
}

// NewMicrosoftTTS returns an Azure Speech REST client.
func NewMicrosoftTTS(apiKey, region string) *MicrosoftTTS {
	region = strings.TrimSpace(region)
	return &MicrosoftTTS{
		apiKey:  strings.TrimSpace(apiKey),
		region:  region,
		baseURL: microsoftBaseURL(region),
		httpClient: &http.Client{
			Timeout: defaultMicrosoftTimeout,
		},
	}
}

// Synthesize renders text to speech and writes the WAV payload to dst.
func (m *MicrosoftTTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return ErrEmptyText
	}
	if dst == nil {
		return fmt.Errorf("generator/tts: destination writer is required")
	}
	if m.apiKey == "" {
		return fmt.Errorf("generator/tts: Microsoft API key is required")
	}
	if m.region == "" {
		return fmt.Errorf("generator/tts: Microsoft region is required")
	}

	body := strings.NewReader(buildMicrosoftSSML(text, mapMicrosoftVoice(voice)))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL, body)
	if err != nil {
		return fmt.Errorf("generator/tts: create Microsoft request: %w", err)
	}
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", defaultMicrosoftOutputFormat)
	req.Header.Set("Ocp-Apim-Subscription-Key", m.apiKey)
	req.Header.Set("User-Agent", "writ-fm-generator")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("generator/tts: Microsoft request failed: %w", err)
	}
	defer resp.Body.Close()

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("generator/tts: read Microsoft response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("generator/tts: Microsoft returned %d: %s", resp.StatusCode, strings.TrimSpace(string(audio)))
	}
	if len(audio) == 0 {
		return fmt.Errorf("generator/tts: Microsoft returned empty audio")
	}
	if _, err := io.Copy(dst, bytes.NewReader(audio)); err != nil {
		return fmt.Errorf("generator/tts: write output: %w", err)
	}
	return nil
}

func microsoftBaseURL(region string) string {
	return fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", region)
}

func mapMicrosoftVoice(voice string) string {
	voice = strings.TrimSpace(voice)
	if mapped, ok := defaultMicrosoftVoices[voice]; ok {
		return mapped
	}
	if strings.Contains(voice, "-") {
		return voice
	}
	return defaultMicrosoftVoices["zh_xiaoxiao"]
}

func buildMicrosoftSSML(text, voice string) string {
	body := text
	if !containsMicrosoftSSML(text) {
		replacer := strings.NewReplacer(
			"&", "&amp;",
			"<", "&lt;",
			">", "&gt;",
		)
		body = replacer.Replace(text)
	}
	return fmt.Sprintf(
		`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xmlns:mstts="https://www.w3.org/2001/mstts" xml:lang="%s"><voice name="%s">%s</voice></speak>`,
		microsoftVoiceLang(voice),
		voice,
		body,
	)
}

func microsoftVoiceLang(voice string) string {
	voice = strings.TrimSpace(voice)
	parts := strings.Split(voice, "-")
	if len(parts) >= 2 {
		return parts[0] + "-" + parts[1]
	}
	return "zh-CN"
}

func containsMicrosoftSSML(text string) bool {
	return strings.Contains(text, "<break") ||
		strings.Contains(text, "<prosody") ||
		strings.Contains(text, "<mstts:express-as") ||
		strings.Contains(text, "</mstts:express-as>")
}
