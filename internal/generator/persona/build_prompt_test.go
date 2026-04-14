package persona

import (
	"strings"
	"testing"
	"time"
)

func TestGetOperatorContext(t *testing.T) {
	t.Parallel()

	ctx := GetOperatorContext(23)
	if ctx.Period != "night" {
		t.Fatalf("Period = %q, want %q", ctx.Period, "night")
	}
	if len(ctx.PreferredSegments) == 0 {
		t.Fatal("PreferredSegments should not be empty")
	}
}

func TestBuildHostPrompt(t *testing.T) {
	t.Parallel()

	builder := NewBuilderWithClock(func() time.Time {
		return time.Date(2026, 4, 14, 23, 47, 0, 0, time.Local)
	})

	prompt, err := builder.BuildHostPrompt("signal", &ShowContext{
		ShowName:        "Signal Report",
		ShowDescription: "Current events decoded after dark.",
		TopicFocus:      "current_events",
		SegmentType:     "news_analysis",
	})
	if err != nil {
		t.Fatalf("BuildHostPrompt() error = %v", err)
	}

	for _, want := range []string{
		"You are Signal",
		"CURRENT SHOW: Signal Report",
		"Segment Type: news_analysis",
		"Time: 23:47 (night)",
		"Current events decoded after dark.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
