package generator

import (
	"strings"
	"testing"
)

func TestNormalizePerformanceCuesConstrainedDropsUnknownBrackets(t *testing.T) {
	t.Parallel()

	normalized := NormalizePerformanceCues("开场 [pause] [mystery] 继续。", PerformanceModeConstrained, "kokoro")
	rendered := RenderPerformanceForBackend(normalized, "kokoro")

	if strings.Contains(rendered, "mystery") {
		t.Fatalf("unexpected unknown cue in %q", rendered)
	}
	if !strings.Contains(rendered, "……继续。") {
		t.Fatalf("missing mapped pause in %q", rendered)
	}
}

func TestNormalizePerformanceCuesExpressiveMapsCommonChineseCues(t *testing.T) {
	t.Parallel()

	normalized := NormalizePerformanceCues("（深呼吸）先别急。（小声）我们慢慢说。（沉默片刻）", PerformanceModeExpressive, "mimo")
	if len(normalized.Cues) != 3 {
		t.Fatalf("len(cues) = %d, want 3", len(normalized.Cues))
	}
	if normalized.Cues[0].Token != PerformanceTokenBreath {
		t.Fatalf("first cue = %q, want breath", normalized.Cues[0].Token)
	}
	if normalized.Cues[1].Token != PerformanceTokenWhisper {
		t.Fatalf("second cue = %q, want whisper", normalized.Cues[1].Token)
	}
	if normalized.Cues[2].Token != PerformanceTokenPause {
		t.Fatalf("third cue = %q, want pause", normalized.Cues[2].Token)
	}
}

func TestRenderPerformanceForBackendMimoLeadingStyleOnly(t *testing.T) {
	t.Parallel()

	normalized := NormalizePerformanceCues("[happy]今天见到你很开心。[soft]接下来我们慢一点。", PerformanceModeConstrained, "mimo")
	rendered := RenderPerformanceForBackend(normalized, "mimo")
	if strings.Count(rendered, "<style>") != 1 {
		t.Fatalf("expected exactly one mimo <style> tag, got %q", rendered)
	}
	if !strings.Contains(rendered, "<style>开心</style>今天见到你很开心。") {
		t.Fatalf("unexpected mimo render: %q", rendered)
	}
	if !strings.Contains(rendered, "（放轻声音）接下来我们慢一点。") {
		t.Fatalf("expected downgraded mid-stream style cue, got %q", rendered)
	}
}

func TestRenderPerformanceForBackendMicrosoft(t *testing.T) {
	t.Parallel()

	normalized := NormalizePerformanceCues("[calm][slow]今晚我们把问题说清楚。", PerformanceModeConstrained, "microsoft")
	rendered := RenderPerformanceForBackend(normalized, "microsoft")
	if !strings.Contains(rendered, `<mstts:express-as style="calm">`) {
		t.Fatalf("missing express-as: %q", rendered)
	}
	if !strings.Contains(rendered, `<prosody rate="slow"`) {
		t.Fatalf("missing prosody rate: %q", rendered)
	}
}

func TestRenderPerformanceForBackendKokoro(t *testing.T) {
	t.Parallel()

	normalized := NormalizePerformanceCues("[whisper]别吵醒夜色。", PerformanceModeConstrained, "kokoro")
	rendered := RenderPerformanceForBackend(normalized, "kokoro")
	if !strings.Contains(rendered, "（小声）别吵醒夜色。") {
		t.Fatalf("unexpected kokoro render: %q", rendered)
	}
}
