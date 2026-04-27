package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	defaultMimo25Timeout          = 60 * time.Second
	defaultMimo25BaseURL          = "https://api.xiaomimimo.com/v1/chat/completions"
	defaultMimo25VoiceDesignModel = "mimo-v2.5-tts-voicedesign"
	mimo25VoiceDesignPrefix       = "voice_design:"
)

var mimo25VoiceDesignPrompts = map[string]string{
	"am_michael":   "男性声音，35到45岁，中低音区，沉稳、温暖的午夜电台男声。语速自然，停顿自然稍慢，像在凌晨两点给失眠的人讲故事。声音克制、亲密、有陪伴感，不夸张，不播音腔。不要生成女声、童声或高亮青年音。",
	"mimo_default": "男性声音，35到45岁，中低音区，低沉、温暖、稳定的午夜电台男声。语速偏慢，停顿自然，像在凌晨两点给失眠的人讲故事。声音克制、亲密、有陪伴感，不夸张，不播音腔。不要生成女声、童声或高亮青年音。",
	"bm_daniel":    "男性声音，40到55岁，中低音区，成熟、克制、有学者气质的男声，清晰可靠，带轻微音乐考古叙述感。语速中等偏慢，表达亲切但不卖弄，适合讲述音乐史、档案和声音线索。不要生成女声或年轻偶像化声线。",
	"am_onyx":      "Male Mandarin Chinese voice, age 35 to 50. Medium-low pitch, mature, clear, steady, analytical news commentator. Calm but firm delivery, precise articulation, controlled urgency, no melodrama. The voice must sound unmistakably male and mature; do not generate a female voice, young female voice, sweet voice, child voice, or high-pitched announcer voice.",
	"af_heart":     "轻柔、清醒、带夜色诗意的女性声音，语速偏慢，留白明显。声音柔和但不软弱，安静、诚实，像在夜里低声陪伴听众。",
	"af_bella":     "温暖、自然、有律动感的女性声音，亲切但不综艺化。语气有人情味，可以轻轻一笑，适合讲 soul、funk、记忆和人与音乐的靠近。",
}

// Mimo25VoiceDesignTTS synthesizes speech through MiMo 2.5 VoiceDesign.
type Mimo25VoiceDesignTTS struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

type mimo25VoiceDesignRequest struct {
	Model    string                 `json:"model"`
	Messages []mimoMessage          `json:"messages"`
	Audio    mimo25VoiceDesignAudio `json:"audio"`
}

type mimo25VoiceDesignAudio struct {
	Format string `json:"format"`
}

// NewMimo25VoiceDesignTTS returns a MiMo 2.5 VoiceDesign adapter.
func NewMimo25VoiceDesignTTS(apiKey, baseURL, model string) *Mimo25VoiceDesignTTS {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultMimo25BaseURL
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultMimo25VoiceDesignModel
	}
	return &Mimo25VoiceDesignTTS{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: defaultMimo25Timeout,
		},
	}
}

// Synthesize renders text to WAV audio using a voice id or voice_design: prompt.
func (m *Mimo25VoiceDesignTTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return ErrEmptyText
	}
	if dst == nil {
		return fmt.Errorf("generator/tts: destination writer is required")
	}
	if m.apiKey == "" {
		return fmt.Errorf("generator/tts: MiMo 2.5 API key is required")
	}

	voicePrompt, err := mimo25VoiceDesignPrompt(voice)
	if err != nil {
		return err
	}

	reqBody, err := json.Marshal(mimo25VoiceDesignRequest{
		Model: m.model,
		Messages: []mimoMessage{
			{
				Role:    "user",
				Content: voicePrompt,
			},
			{
				Role:    "assistant",
				Content: text,
			},
		},
		Audio: mimo25VoiceDesignAudio{
			Format: "wav",
		},
	})
	if err != nil {
		return fmt.Errorf("generator/tts: marshal MiMo 2.5 request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("generator/tts: create MiMo 2.5 request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", m.apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("generator/tts: MiMo 2.5 request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("generator/tts: read MiMo 2.5 response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("generator/tts: MiMo 2.5 returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded mimoResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("generator/tts: decode MiMo 2.5 response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return fmt.Errorf("generator/tts: MiMo 2.5 returned no choices")
	}
	if decoded.Choices[0].Message.ErrorMessage != "" {
		return fmt.Errorf("generator/tts: MiMo 2.5 error: %s", decoded.Choices[0].Message.ErrorMessage)
	}
	if decoded.Choices[0].Message.Audio == nil || strings.TrimSpace(decoded.Choices[0].Message.Audio.Data) == "" {
		return fmt.Errorf("generator/tts: MiMo 2.5 returned empty audio")
	}

	audio, err := base64.StdEncoding.DecodeString(decoded.Choices[0].Message.Audio.Data)
	if err != nil {
		return fmt.Errorf("generator/tts: decode MiMo 2.5 audio: %w", err)
	}
	if len(audio) == 0 {
		return fmt.Errorf("generator/tts: MiMo 2.5 returned empty decoded audio")
	}
	if _, err := io.Copy(dst, bytes.NewReader(audio)); err != nil {
		return fmt.Errorf("generator/tts: write output: %w", err)
	}
	return nil
}

func mimo25VoiceDesignPrompt(voice string) (string, error) {
	voice = strings.TrimSpace(voice)
	if strings.HasPrefix(voice, mimo25VoiceDesignPrefix) {
		prompt := strings.TrimSpace(strings.TrimPrefix(voice, mimo25VoiceDesignPrefix))
		if prompt == "" {
			return "", fmt.Errorf("generator/tts: empty MiMo 2.5 voice design prompt")
		}
		return prompt, nil
	}
	if prompt, ok := mimo25VoiceDesignPrompts[strings.ToLower(voice)]; ok {
		return prompt, nil
	}
	return "", fmt.Errorf("generator/tts: unknown MiMo 2.5 voice %q (use one of: %s, or prefix a custom prompt with %q)",
		voice, strings.Join(mimo25KnownVoiceIDs(), ", "), mimo25VoiceDesignPrefix)
}

func mimo25KnownVoiceIDs() []string {
	ids := make([]string, 0, len(mimo25VoiceDesignPrompts))
	for id := range mimo25VoiceDesignPrompts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
