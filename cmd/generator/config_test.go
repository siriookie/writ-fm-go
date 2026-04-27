package main

import (
	"fmt"
	"strings"
	"testing"

	gen "github.com/writ-fm/go/internal/generator"
)

func TestBuildTTSClientMimo25VoiceDesignRequiresAPIKey(t *testing.T) {
	t.Parallel()

	_, err := buildTTSClient(config{TTSBackend: "mimo25_voicedesign"})
	if err == nil || !strings.Contains(err.Error(), "MIMO_API_KEY is required") {
		t.Fatalf("buildTTSClient() error = %v, want MIMO_API_KEY error", err)
	}
}

func TestBuildTTSClientMimo25VoiceDesign(t *testing.T) {
	t.Parallel()

	client, err := buildTTSClient(config{
		TTSBackend:     "mimo25_voicedesign",
		MimoAPIKey:     "key",
		Mimo25BaseURL:  "http://example.test/tts",
		Mimo25TTSModel: "mimo-v2.5-tts-voicedesign",
	})
	if err != nil {
		t.Fatalf("buildTTSClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("buildTTSClient() returned nil client")
	}
}

func TestBuildTTSClientMimoLegacyBackendAutoRoutesVoiceDesignModel(t *testing.T) {
	t.Parallel()

	client, err := buildTTSClient(config{
		TTSBackend:   "mimo",
		MimoAPIKey:   "key",
		MimoBaseURL:  "http://example.test/tts",
		MimoTTSModel: "mimo-v2.5-tts-voicedesign",
	})
	if err != nil {
		t.Fatalf("buildTTSClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("buildTTSClient() returned nil client")
	}
	if got := fmt.Sprintf("%T", client); !strings.Contains(got, "Mimo25VoiceDesignTTS") {
		t.Fatalf("client type = %s, want Mimo25VoiceDesignTTS", got)
	}
}

func TestConfigFromEnvReadsOutlineMode(t *testing.T) {
	t.Setenv("GENERATOR_OUTLINE_MODE", gen.OutlineModeOff)
	t.Setenv("GENERATOR_TTS_CHUNK_UNITS", "360")
	t.Setenv("GENERATOR_TTS_CONCAT_FADE_SECONDS", "0.02")

	cfg := configFromEnv()
	if cfg.OutlineMode != gen.OutlineModeOff {
		t.Fatalf("OutlineMode = %q, want %q", cfg.OutlineMode, gen.OutlineModeOff)
	}
	if cfg.TTSChunkWords != 360 {
		t.Fatalf("TTSChunkWords = %d, want 360", cfg.TTSChunkWords)
	}
	if cfg.TTSConcatFade != 0.02 {
		t.Fatalf("TTSConcatFade = %v, want 0.02", cfg.TTSConcatFade)
	}
}

func TestConfigFromEnvDefaultsToOpenAICompatible(t *testing.T) {
	t.Setenv("LLM_BACKEND", "")

	cfg := configFromEnv()
	if cfg.LLMBackend != "openai_compatible" {
		t.Fatalf("LLMBackend = %q, want openai_compatible", cfg.LLMBackend)
	}
}

func TestBuildLLMClientOpenAICompatibleAliasRequiresModel(t *testing.T) {
	t.Parallel()

	_, err := buildLLMClient(config{LLMBackend: "openai_compatible"})
	if err == nil || !strings.Contains(err.Error(), "OPENAI_MODEL is required") {
		t.Fatalf("buildLLMClient() error = %v, want OPENAI_MODEL error", err)
	}
}

func TestBuildLLMClientOpenAICompatibleAlias(t *testing.T) {
	t.Parallel()

	client, err := buildLLMClient(config{
		LLMBackend:    "openai-compatible",
		OpenAIBaseURL: "http://example.test/v1",
		OpenAIModel:   "model",
	})
	if err != nil {
		t.Fatalf("buildLLMClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("buildLLMClient() returned nil client")
	}
}
