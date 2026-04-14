package generator

import (
	"strings"
	"testing"
	"time"

	"github.com/writ-fm/go/internal/generator/persona"
)

func TestBuildGenerationPrompt(t *testing.T) {
	t.Parallel()

	builder := NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time {
			return time.Date(2026, 4, 14, 22, 5, 0, 0, time.Local)
		}).BuildHostPrompt,
		nil,
	)

	prompt, err := builder.Build(BuildRequest{
		HostID:          "liminal_operator",
		SegmentType:     "deep_dive",
		Topic:           "The archaeology of memory",
		ShowName:        "Midnight Signal",
		ShowDescription: "Late night transmissions.",
		TopicFocus:      "philosophy",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	for _, want := range []string{
		"SEGMENT: deep_dive",
		"TOPIC: The archaeology of memory",
		"TARGET LENGTH: 1500-2500 words",
		"CURRENT SHOW: Midnight Signal",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildGenerationPrompt_NewsAnalysisUsesHeadlines(t *testing.T) {
	t.Parallel()

	prompt, err := NewPromptBuilder().Build(BuildRequest{
		HostID:      "signal",
		SegmentType: "news_analysis",
		Topic:       "What the headlines aren't telling you",
		Headlines:   "- [NPR] Example headline",
	})
	if err != nil {
		t.Fatalf("BuildGenerationPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Example headline") {
		t.Fatalf("prompt missing formatted headlines:\n%s", prompt)
	}
}

func TestBuildGenerationPrompt_InterviewInjectsGuest(t *testing.T) {
	t.Parallel()

	builder := NewPromptBuilderWithDeps(nil, func(n int) int { return 0 })

	prompt, err := builder.Build(BuildRequest{
		HostID:      "dr_resonance",
		SegmentType: "interview",
		Topic:       "Pirate radio and memory",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(prompt, InterviewGuests[0].Name) {
		t.Fatalf("prompt missing guest name:\n%s", prompt)
	}
	if !strings.Contains(prompt, InterviewGuests[0].Context) {
		t.Fatalf("prompt missing guest context:\n%s", prompt)
	}
}
