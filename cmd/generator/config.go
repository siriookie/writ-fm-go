package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gen "github.com/writ-fm/go/internal/generator"
	genllm "github.com/writ-fm/go/internal/generator/llm"
	gentts "github.com/writ-fm/go/internal/generator/tts"
)

type config struct {
	LLMBackend      string
	TTSBackend      string
	ClaudeModel     string
	OpenAIBaseURL   string
	OpenAIAPIKey    string
	OpenAIModel     string
	LiteLLMBaseURL  string
	LiteLLMAPIKey   string
	LiteLLMModel    string
	LiteLLMTags     []string
	KokoroDir       string
	KokoroModalURL  string
	MimoAPIKey      string
	MimoBaseURL     string
	MimoTTSModel    string
	AzureTTSKey     string
	AzureTTSRegion  string
	QwenTTSAPIKey   string
	QwenTTSModel    string
	CosyVoiceAPIKey string
	SchedulePath    string
	TalkSegmentsDir string
	ScriptsDir      string
}

type generateService interface {
	Generate(ctx context.Context, req gen.GenerateRequest) (*gen.Result, error)
}

func configFromEnv() config {
	return config{
		LLMBackend:      getenvDefault("LLM_BACKEND", "claude_cli"),
		TTSBackend:      getenvDefault("TTS_BACKEND", "kokoro"),
		ClaudeModel:     strings.TrimSpace(os.Getenv("CLAUDE_MODEL")),
		OpenAIBaseURL:   getenvDefault("OPENAI_BASE_URL", "https://api.openai.com"),
		OpenAIAPIKey:    strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:     strings.TrimSpace(os.Getenv("OPENAI_MODEL")),
		LiteLLMBaseURL:  getenvDefault("LITELLM_BASE_URL", "http://0.0.0.0:4000"),
		LiteLLMAPIKey:   strings.TrimSpace(os.Getenv("LITELLM_API_KEY")),
		LiteLLMModel:    getenvDefault("LITELLM_MODEL", "gpt-3.5-turbo"),
		LiteLLMTags:     splitCSV(os.Getenv("LITELLM_TAGS")),
		KokoroDir:       getenvDefault("KOKORO_DIR", defaultKokoroDir()),
		KokoroModalURL:  strings.TrimSpace(os.Getenv("KOKORO_MODAL_URL")),
		MimoAPIKey:      strings.TrimSpace(os.Getenv("MIMO_API_KEY")),
		MimoBaseURL:     getenvDefault("MIMO_BASE_URL", "https://api.xiaomimimo.com/v1/chat/completions"),
		MimoTTSModel:    getenvDefault("MIMO_TTS_MODEL", "mimo-v2-tts"),
		AzureTTSKey:     strings.TrimSpace(os.Getenv("AZURE_TTS_KEY")),
		AzureTTSRegion:  getenvDefault("AZURE_TTS_REGION", "eastus"),
		QwenTTSAPIKey:   firstNonEmpty(strings.TrimSpace(os.Getenv("QWEN_TTS_API_KEY")), strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))),
		QwenTTSModel:    getenvDefault("QWEN_TTS_MODEL", "qwen3-tts-flash-realtime"),
		CosyVoiceAPIKey: firstNonEmpty(strings.TrimSpace(os.Getenv("COSYVOICE_API_KEY")), strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))),
		SchedulePath:    getenvDefault("SCHEDULE_PATH", "config/schedule.yaml"),
		TalkSegmentsDir: getenvDefault("TALK_SEGMENTS_DIR", "output/talk_segments"),
		ScriptsDir:      getenvDefault("SCRIPTS_DIR", "output/scripts"),
	}
}

func buildGenerator(cfg config) (generateService, error) {
	llmClient, err := buildLLMClient(cfg)
	if err != nil {
		return nil, err
	}
	ttsClient, err := buildTTSClient(cfg)
	if err != nil {
		return nil, err
	}
	renderer := gen.NewRenderer(ttsClient)
	return gen.New(llmClient, renderer, cfg.TTSBackend, cfg.TalkSegmentsDir, cfg.ScriptsDir), nil
}

func buildLLMClient(cfg config) (gen.LLMClient, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.LLMBackend)) {
	case "claude_cli":
		return genllm.NewClaudeCLI(cfg.ClaudeModel), nil
	case "openai":
		if strings.TrimSpace(cfg.OpenAIModel) == "" {
			return nil, fmt.Errorf("OPENAI_MODEL is required for LLM_BACKEND=%q", cfg.LLMBackend)
		}
		return genllm.NewOpenAIClient(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey, cfg.OpenAIModel), nil
	case "litellm":
		if strings.TrimSpace(cfg.LiteLLMModel) == "" {
			return nil, fmt.Errorf("LITELLM_MODEL is required for LLM_BACKEND=%q", cfg.LLMBackend)
		}
		metadata := map[string]any(nil)
		if len(cfg.LiteLLMTags) > 0 {
			metadata = map[string]any{"tags": append([]string(nil), cfg.LiteLLMTags...)}
		}
		return genllm.NewLiteLLMClient(cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey, cfg.LiteLLMModel, metadata), nil
	default:
		return nil, fmt.Errorf("unknown LLM_BACKEND: %q", cfg.LLMBackend)
	}
}

func buildTTSClient(cfg config) (gentts.Client, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.TTSBackend)) {
	case "kokoro":
		return gentts.NewKokoroTTS(cfg.KokoroDir), nil
	case "kokoro_modal":
		if strings.TrimSpace(cfg.KokoroModalURL) == "" {
			return nil, fmt.Errorf("KOKORO_MODAL_URL is required for TTS_BACKEND=%q", cfg.TTSBackend)
		}
		return gentts.NewKokoroModalTTS(cfg.KokoroModalURL), nil
	case "mimo":
		if strings.TrimSpace(cfg.MimoAPIKey) == "" {
			return nil, fmt.Errorf("MIMO_API_KEY is required for TTS_BACKEND=%q", cfg.TTSBackend)
		}
		return gentts.NewMimoTTS(cfg.MimoAPIKey, cfg.MimoBaseURL, cfg.MimoTTSModel), nil
	case "microsoft":
		if strings.TrimSpace(cfg.AzureTTSKey) == "" {
			return nil, fmt.Errorf("AZURE_TTS_KEY is required for TTS_BACKEND=%q", cfg.TTSBackend)
		}
		return gentts.NewMicrosoftTTS(cfg.AzureTTSKey, cfg.AzureTTSRegion), nil
	case "qwen":
		if strings.TrimSpace(cfg.QwenTTSAPIKey) == "" {
			return nil, fmt.Errorf("QWEN_TTS_API_KEY or DASHSCOPE_API_KEY is required for TTS_BACKEND=%q", cfg.TTSBackend)
		}
		return gentts.NewQwenTTS(cfg.QwenTTSAPIKey, cfg.QwenTTSModel), nil
	case "cosyvoice":
		if strings.TrimSpace(cfg.CosyVoiceAPIKey) == "" {
			return nil, fmt.Errorf("COSYVOICE_API_KEY or DASHSCOPE_API_KEY is required for TTS_BACKEND=%q", cfg.TTSBackend)
		}
		return gentts.NewCosyVoiceTTS(cfg.CosyVoiceAPIKey), nil
	default:
		return nil, fmt.Errorf("unknown TTS_BACKEND: %q", cfg.TTSBackend)
	}
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func defaultKokoroDir() string {
	return filepath.Join("mac", "kokoro")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
