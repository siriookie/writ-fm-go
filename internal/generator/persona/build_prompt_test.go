package persona

import (
	"os"
	"path/filepath"
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

func TestBuildHostPrompt_Golden(t *testing.T) {
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

	assertGolden(t, filepath.Join("testdata", "signal_host_prompt.golden"), prompt)
}

func assertGolden(t *testing.T, path string, got string) {
	t.Helper()

	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	normalize := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
	}

	want := normalize(string(wantBytes))
	got = normalize(got)
	if got != want {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}
