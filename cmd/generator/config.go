package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	gen "github.com/writ-fm/go/internal/generator"
	genllm "github.com/writ-fm/go/internal/generator/llm"
	gentts "github.com/writ-fm/go/internal/generator/tts"
	"github.com/writ-fm/go/internal/news"
)

type config struct {
	LLMBackend        string
	TTSBackend        string
	OutlineMode       string
	PerformanceMode   string
	DebugScript       bool
	DebugLLM          bool
	DebugChunks       bool
	DebugChunkDir     string
	TTSChunkWords     int
	TTSConcatFade     float64
	ClaudeModel       string
	OpenAIBaseURL     string
	OpenAIAPIKey      string
	OpenAIModel       string
	LiteLLMBaseURL    string
	LiteLLMAPIKey     string
	LiteLLMModel      string
	LiteLLMTags       []string
	KokoroDir         string
	KokoroModalURL    string
	IndexTTS2ModalURL string
	MimoAPIKey        string
	MimoBaseURL       string
	MimoTTSModel      string
	Mimo25BaseURL     string
	Mimo25TTSModel    string
	AzureTTSKey       string
	AzureTTSRegion    string
	QwenTTSAPIKey     string
	QwenTTSModel      string
	CosyVoiceAPIKey   string
	NewsFeeds         []string
	NewsMaxItems      int
	NewsCacheTTL      time.Duration
	NewsTimeout       time.Duration
	SchedulePath      string
	TalkSegmentsDir   string
	ScriptsDir        string
}

type generateService interface {
	Generate(ctx context.Context, req gen.GenerateRequest) (*gen.Result, error)
}

func configFromEnv() config {
	return config{
		LLMBackend:        getenvDefault("LLM_BACKEND", "openai_compatible"),
		TTSBackend:        getenvDefault("TTS_BACKEND", "kokoro"),
		OutlineMode:       getenvDefault("GENERATOR_OUTLINE_MODE", gen.OutlineModeAuto),
		PerformanceMode:   getenvDefault("PERFORMANCE_MODE", "constrained"),
		DebugScript:       getenvBool("GENERATOR_DEBUG_SCRIPT", false),
		DebugLLM:          getenvBool("GENERATOR_DEBUG_LLM", true),
		DebugChunks:       getenvBool("GENERATOR_DEBUG_CHUNKS", false),
		DebugChunkDir:     getenvDefault("GENERATOR_DEBUG_CHUNK_DIR", filepath.Join("output", "debug_chunks")),
		TTSChunkWords:     getenvInt("GENERATOR_TTS_CHUNK_UNITS", 240),
		TTSConcatFade:     getenvFloat("GENERATOR_TTS_CONCAT_FADE_SECONDS", 0.012),
		ClaudeModel:       strings.TrimSpace(os.Getenv("CLAUDE_MODEL")),
		OpenAIBaseURL:     getenvDefault("OPENAI_BASE_URL", "https://api.openai.com"),
		OpenAIAPIKey:      strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:       strings.TrimSpace(os.Getenv("OPENAI_MODEL")),
		LiteLLMBaseURL:    getenvDefault("LITELLM_BASE_URL", "http://0.0.0.0:4000"),
		LiteLLMAPIKey:     strings.TrimSpace(os.Getenv("LITELLM_API_KEY")),
		LiteLLMModel:      getenvDefault("LITELLM_MODEL", "gpt-3.5-turbo"),
		LiteLLMTags:       splitCSV(os.Getenv("LITELLM_TAGS")),
		KokoroDir:         getenvDefault("KOKORO_DIR", defaultKokoroDir()),
		KokoroModalURL:    strings.TrimSpace(os.Getenv("KOKORO_MODAL_URL")),
		IndexTTS2ModalURL: strings.TrimSpace(os.Getenv("INDEXTTS2_MODAL_URL")),
		MimoAPIKey:        strings.TrimSpace(os.Getenv("MIMO_API_KEY")),
		MimoBaseURL:       getenvDefault("MIMO_BASE_URL", "https://api.xiaomimimo.com/v1/chat/completions"),
		MimoTTSModel:      getenvDefault("MIMO_TTS_MODEL", "mimo-v2-tts"),
		Mimo25BaseURL:     getenvDefault("MIMO25_BASE_URL", "https://api.xiaomimimo.com/v1/chat/completions"),
		Mimo25TTSModel:    getenvDefault("MIMO25_TTS_MODEL", "mimo-v2.5-tts-voicedesign"),
		AzureTTSKey:       strings.TrimSpace(os.Getenv("AZURE_TTS_KEY")),
		AzureTTSRegion:    getenvDefault("AZURE_TTS_REGION", "eastus"),
		QwenTTSAPIKey:     firstNonEmpty(strings.TrimSpace(os.Getenv("QWEN_TTS_API_KEY")), strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))),
		QwenTTSModel:      getenvDefault("QWEN_TTS_MODEL", "qwen3-tts-flash-realtime"),
		CosyVoiceAPIKey:   firstNonEmpty(strings.TrimSpace(os.Getenv("COSYVOICE_API_KEY")), strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))),
		NewsFeeds:         splitCSV(os.Getenv("NEWS_FEEDS")),
		NewsMaxItems:      getenvInt("NEWS_MAX_ITEMS", 8),
		NewsCacheTTL:      getenvDurationSeconds("NEWS_CACHE_TTL", 600),
		NewsTimeout:       getenvDurationSeconds("NEWS_TIMEOUT", 6),
		SchedulePath:      getenvDefault("SCHEDULE_PATH", "config/schedule.yaml"),
		TalkSegmentsDir:   getenvDefault("TALK_SEGMENTS_DIR", "output/talk_segments"),
		ScriptsDir:        getenvDefault("SCRIPTS_DIR", "output/scripts"),
	}
}

func buildGenerator(cfg config) (generateService, error) {
	llmClient, err := buildLLMClient(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.DebugLLM {
		llmClient = &loggingLLMClient{next: llmClient}
	}
	ttsClient, err := buildTTSClient(cfg)
	if err != nil {
		return nil, err
	}
	rendererOpts := []gen.RendererOption{gen.WithBackend(cfg.TTSBackend)}
	if cfg.DebugChunks {
		rendererOpts = append(rendererOpts, gen.WithChunkDebug(cfg.DebugChunkDir))
	}
	if cfg.TTSChunkWords > 0 {
		rendererOpts = append(rendererOpts, gen.WithChunkWords(cfg.TTSChunkWords))
	}
	if cfg.TTSConcatFade >= 0 {
		rendererOpts = append(rendererOpts, gen.WithConcatFade(cfg.TTSConcatFade))
	}
	renderer := gen.NewRenderer(ttsClient, rendererOpts...)
	newsClient := news.NewClient(
		news.WithFeeds(cfg.NewsFeeds),
		news.WithMaxItems(cfg.NewsMaxItems),
		news.WithCacheTTL(cfg.NewsCacheTTL),
		news.WithTimeout(cfg.NewsTimeout),
	)
	return gen.New(
		llmClient,
		renderer,
		cfg.TTSBackend,
		cfg.TalkSegmentsDir,
		cfg.ScriptsDir,
		gen.WithHeadlineProvider(newsClient),
		gen.WithOutlineMode(cfg.OutlineMode),
	), nil
}

type loggingLLMClient struct {
	next  gen.LLMClient
	calls int
}

func (l *loggingLLMClient) Generate(ctx context.Context, prompt string) (string, error) {
	l.calls++
	call := l.calls
	start := time.Now()
	log.Printf("generator/llm: request %d prompt BEGIN\n%s\ngenerator/llm: request %d prompt END", call, prompt, call)
	response, err := l.next.Generate(ctx, prompt)
	if err != nil {
		log.Printf("generator/llm: response %d error after %s: %v", call, time.Since(start).Round(time.Millisecond), err)
		return "", err
	}
	log.Printf("generator/llm: response %d after %s BEGIN\n%s\ngenerator/llm: response %d END", call, time.Since(start).Round(time.Millisecond), response, call)
	return response, nil
}

func buildLLMClient(cfg config) (gen.LLMClient, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.LLMBackend)) {
	case "claude_cli":
		return genllm.NewClaudeCLI(cfg.ClaudeModel), nil
	case "openai", "openai_compatible", "openai-compatible":
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
	case "indextts2":
		if strings.TrimSpace(cfg.IndexTTS2ModalURL) == "" {
			return nil, fmt.Errorf("INDEXTTS2_MODAL_URL is required for TTS_BACKEND=%q", cfg.TTSBackend)
		}
		return gentts.NewIndexTTS2TTS(cfg.IndexTTS2ModalURL), nil
	case "mimo":
		if strings.TrimSpace(cfg.MimoAPIKey) == "" {
			return nil, fmt.Errorf("MIMO_API_KEY is required for TTS_BACKEND=%q", cfg.TTSBackend)
		}
		if isMimo25VoiceDesignModel(cfg.MimoTTSModel) {
			return gentts.NewMimo25VoiceDesignTTS(cfg.MimoAPIKey, cfg.MimoBaseURL, cfg.MimoTTSModel), nil
		}
		return gentts.NewMimoTTS(cfg.MimoAPIKey, cfg.MimoBaseURL, cfg.MimoTTSModel), nil
	case "mimo25_voicedesign":
		if strings.TrimSpace(cfg.MimoAPIKey) == "" {
			return nil, fmt.Errorf("MIMO_API_KEY is required for TTS_BACKEND=%q", cfg.TTSBackend)
		}
		return gentts.NewMimo25VoiceDesignTTS(cfg.MimoAPIKey, cfg.Mimo25BaseURL, cfg.Mimo25TTSModel), nil
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

func isMimo25VoiceDesignModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	return normalized == "mimo-v2.5-tts-voicedesign"
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getenvFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	var parsed float64
	if _, err := fmt.Sscanf(value, "%f", &parsed); err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func getenvDurationSeconds(key string, fallbackSeconds int) time.Duration {
	return time.Duration(getenvInt(key, fallbackSeconds)) * time.Second
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
