package generator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/writ-fm/go/internal/generator/persona"
	"github.com/writ-fm/go/internal/news"
)

type fakeLLM struct {
	responses []string
	errs      []error
	calls     int
	prompts   []string
}

func (f *fakeLLM) Generate(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	f.prompts = append(f.prompts, prompt)
	idx := f.calls
	f.calls++
	if idx < len(f.errs) && f.errs[idx] != nil {
		return "", f.errs[idx]
	}
	if idx < len(f.responses) {
		return f.responses[idx], nil
	}
	return "", nil
}

type fakeRenderer struct {
	singleCalls []renderSingleCall
	multiCalls  []renderMultiCall
	duration    float64
	durationErr error
	renderErr   error
}

type fakeHeadlineProvider struct {
	items []news.Headline
	err   error
	calls int
}

func (f *fakeHeadlineProvider) FetchHeadlines(ctx context.Context) ([]news.Headline, error) {
	_ = ctx
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	dst := make([]news.Headline, len(f.items))
	copy(dst, f.items)
	return dst, nil
}

type renderSingleCall struct {
	Script string
	Voice  string
	Path   string
	Mode   PerformanceMode
}

type renderMultiCall struct {
	Script string
	Voices map[string]string
	Path   string
	Mode   PerformanceMode
}

func (f *fakeRenderer) RenderSingle(ctx context.Context, script, voice, outputPath string, mode PerformanceMode) error {
	_ = ctx
	if f.renderErr != nil {
		return f.renderErr
	}
	f.singleCalls = append(f.singleCalls, renderSingleCall{Script: script, Voice: voice, Path: outputPath, Mode: mode})
	return os.WriteFile(outputPath, []byte(script), 0o644)
}

func (f *fakeRenderer) RenderMulti(ctx context.Context, script string, voices map[string]string, outputPath string, mode PerformanceMode) error {
	_ = ctx
	if f.renderErr != nil {
		return f.renderErr
	}
	f.multiCalls = append(f.multiCalls, renderMultiCall{Script: script, Voices: cloneStringMap(voices), Path: outputPath, Mode: mode})
	return os.WriteFile(outputPath, []byte(script), 0o644)
}

func (f *fakeRenderer) Duration(ctx context.Context, path string) (float64, error) {
	_ = ctx
	_ = path
	if f.durationErr != nil {
		return 0, f.durationErr
	}
	return f.duration, nil
}

func TestGeneratorGenerateWritesMetadataAndAudio(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{
		responses: []string{strings.Repeat("夜", 2200)},
	}
	renderer := &fakeRenderer{duration: 123.5}
	talkDir := filepath.Join(t.TempDir(), "talk")
	scriptsDir := filepath.Join(t.TempDir(), "scripts")
	g := New(fake, renderer, "kokoro", talkDir, scriptsDir)
	fakeNow := time.Date(2026, 4, 14, 22, 5, 6, 0, time.Local)
	g.now = func() time.Time { return fakeNow }
	g.idGen = func() string { return "fixedid" }
	g.promptBuilder = NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time { return fakeNow }).BuildHostPrompt,
		func(n int) int { return 0 },
	)

	result, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:          "midnight_signal",
		ShowName:        "Midnight Signal",
		ShowDescription: "Late night transmissions.",
		HostID:          "liminal_operator",
		TopicFocus:      "philosophy",
		SegmentType:     "deep_dive",
		Topic:           "The archaeology of memory",
		Voices:          map[string]string{"host": "am_michael"},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.WordCount < 2200 {
		t.Fatalf("WordCount = %d, want >= 2200", result.WordCount)
	}
	if result.Duration != 123.5 {
		t.Fatalf("Duration = %v, want 123.5", result.Duration)
	}
	if len(renderer.singleCalls) != 1 {
		t.Fatalf("single render calls = %d, want 1", len(renderer.singleCalls))
	}
	if renderer.singleCalls[0].Mode != PerformanceModeConstrained {
		t.Fatalf("mode = %q, want constrained", renderer.singleCalls[0].Mode)
	}

	data, err := os.ReadFile(result.MetadataPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var meta ScriptMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if meta.Status != "audio_rendered" {
		t.Fatalf("Status = %q, want %q", meta.Status, "audio_rendered")
	}
	if meta.DurationSeconds == nil || *meta.DurationSeconds != 123.5 {
		t.Fatalf("DurationSeconds = %#v, want 123.5", meta.DurationSeconds)
	}
	if meta.AudioPath != result.AudioPath {
		t.Fatalf("AudioPath = %q, want %q", meta.AudioPath, result.AudioPath)
	}
	if !strings.Contains(filepath.Base(result.AudioPath), "deep_dive_the_archaeology_of_memory_20260414_220506_fixedid.wav") {
		t.Fatalf("unexpected audio path %q", result.AudioPath)
	}
	if filepath.Base(result.MetadataPath) != "talk_deep_dive_20260414_220506_fixedid.json" {
		t.Fatalf("unexpected metadata path %q", result.MetadataPath)
	}
}

func TestGeneratorGenerateUsesMultiVoiceRenderer(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{
		responses: []string{strings.Repeat("HOST: hello there traveler. GUEST: reply with memory and signal. ", 300)},
	}
	renderer := &fakeRenderer{duration: 88}
	g := New(fake, renderer, "microsoft", t.TempDir(), t.TempDir())
	fakeNow := time.Date(2026, 4, 14, 22, 5, 6, 0, time.Local)
	g.now = func() time.Time { return fakeNow }
	g.idGen = func() string { return "multiid" }
	g.promptBuilder = NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time { return fakeNow }).BuildHostPrompt,
		func(n int) int { return 0 },
	)

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:          "midnight_signal",
		ShowName:        "Midnight Signal",
		ShowDescription: "Late night transmissions.",
		HostID:          "dr_resonance",
		TopicFocus:      "music_history",
		SegmentType:     "interview",
		Topic:           "Pirate radio and memory",
		PerformanceMode: PerformanceModeExpressive,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(renderer.multiCalls) != 1 {
		t.Fatalf("multi render calls = %d, want 1", len(renderer.multiCalls))
	}
	if renderer.multiCalls[0].Voices["host"] != "zh_yunxi" {
		t.Fatalf("host voice = %q, want zh_yunxi", renderer.multiCalls[0].Voices["host"])
	}
	if renderer.multiCalls[0].Voices["guest"] != "zh_xiaoxiao" {
		t.Fatalf("guest voice = %q, want zh_xiaoxiao", renderer.multiCalls[0].Voices["guest"])
	}
	if renderer.multiCalls[0].Mode != PerformanceModeExpressive {
		t.Fatalf("mode = %q, want expressive", renderer.multiCalls[0].Mode)
	}
}

func TestGeneratorGenerateRetriesShortScript(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{
		responses: []string{
			"too short",
			strings.Repeat("夜", 2200),
		},
	}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir())
	fakeNow := time.Date(2026, 4, 14, 22, 5, 6, 0, time.Local)
	g.now = func() time.Time { return fakeNow }
	g.idGen = func() string { return "retryid" }
	g.promptBuilder = NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time { return fakeNow }).BuildHostPrompt,
		func(n int) int { return 0 },
	)

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:          "midnight_signal",
		ShowName:        "Midnight Signal",
		ShowDescription: "Late night transmissions.",
		HostID:          "liminal_operator",
		TopicFocus:      "philosophy",
		SegmentType:     "deep_dive",
		Topic:           "The archaeology of memory",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if fake.calls != 2 {
		t.Fatalf("calls = %d, want 2", fake.calls)
	}
	if len(fake.prompts) < 2 || !strings.Contains(fake.prompts[1], "修订要求：") {
		t.Fatalf("retry prompt missing revision instructions:\n%s", strings.Join(fake.prompts, "\n---\n"))
	}
}

func TestGeneratorGenerateFailsAfterShortRetries(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{responses: []string{"short", "still short", "tiny"}}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir())
	fakeNow := time.Date(2026, 4, 14, 22, 5, 6, 0, time.Local)
	g.now = func() time.Time { return fakeNow }
	g.idGen = func() string { return "failid" }
	g.promptBuilder = NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time { return fakeNow }).BuildHostPrompt,
		func(n int) int { return 0 },
	)

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:          "midnight_signal",
		ShowName:        "Midnight Signal",
		ShowDescription: "Late night transmissions.",
		HostID:          "liminal_operator",
		TopicFocus:      "philosophy",
		SegmentType:     "deep_dive",
		Topic:           "The archaeology of memory",
	})
	if !errors.Is(err, ErrScriptTooShort) {
		t.Fatalf("Generate() error = %v, want ErrScriptTooShort", err)
	}
	if fake.calls != 3 {
		t.Fatalf("calls = %d, want 3", fake.calls)
	}
}

func TestGeneratorGenerateAcceptsRelaxedThresholdOnThirdAttempt(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{
		responses: []string{
			strings.Repeat("夜", 1000),
			strings.Repeat("夜", 1200),
			strings.Repeat("夜", 1450),
		},
	}
	renderer := &fakeRenderer{duration: 10}
	g := New(fake, renderer, "kokoro", t.TempDir(), t.TempDir())
	fakeNow := time.Date(2026, 4, 14, 22, 5, 6, 0, time.Local)
	g.now = func() time.Time { return fakeNow }
	g.idGen = func() string { return "relaxedid" }
	g.promptBuilder = NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time { return fakeNow }).BuildHostPrompt,
		func(n int) int { return 0 },
	)

	result, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:          "midnight_signal",
		ShowName:        "Midnight Signal",
		ShowDescription: "Late night transmissions.",
		HostID:          "liminal_operator",
		TopicFocus:      "philosophy",
		SegmentType:     "deep_dive",
		Topic:           "The archaeology of memory",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if fake.calls != 3 {
		t.Fatalf("calls = %d, want 3", fake.calls)
	}
	if result.WordCount != 1450 {
		t.Fatalf("WordCount = %d, want 1450", result.WordCount)
	}
}

func TestGeneratorGenerateInjectsNewsHeadlines(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{
		responses: []string{strings.Repeat("晨", 2200)},
	}
	headlines := &fakeHeadlineProvider{
		items: []news.Headline{
			{Title: "第一条标题", Source: "BBC 中文"},
			{Title: "第二条标题", Source: "NPR"},
		},
	}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithHeadlineProvider(headlines))
	fakeNow := time.Date(2026, 4, 14, 22, 5, 6, 0, time.Local)
	g.now = func() time.Time { return fakeNow }
	g.idGen = func() string { return "newsid" }
	g.promptBuilder = NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time { return fakeNow }).BuildHostPrompt,
		func(n int) int { return 0 },
	)

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:          "signal_report",
		ShowName:        "Signal Report",
		ShowDescription: "News through a late-night lens.",
		HostID:          "signal",
		TopicFocus:      "current_events",
		SegmentType:     "news_analysis",
		Topic:           "本周新闻",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if headlines.calls != 1 {
		t.Fatalf("headline calls = %d, want 1", headlines.calls)
	}
	if len(fake.prompts) != 1 || !strings.Contains(fake.prompts[0], "- [BBC 中文] 第一条标题") {
		t.Fatalf("prompt missing injected headlines:\n%s", strings.Join(fake.prompts, "\n---\n"))
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()

	if got := slugify("The archaeology of memory!!!", 30); got != "the_archaeology_of_memory" {
		t.Fatalf("slugify() = %q", got)
	}
	if got := slugify("   ", 30); got != "segment" {
		t.Fatalf("slugify(blank) = %q, want segment", got)
	}
}

func TestGeneratorAllocatePathsUsesUniqueIDsForSameSecond(t *testing.T) {
	t.Parallel()

	g := New(&fakeLLM{}, &fakeRenderer{}, "kokoro", filepath.Join(t.TempDir(), "talk"), filepath.Join(t.TempDir(), "scripts"))
	fakeNow := time.Date(2026, 4, 14, 22, 5, 6, 0, time.Local)
	g.now = func() time.Time { return fakeNow }

	ids := []string{"one", "two"}
	g.idGen = func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	}

	req := GenerateRequest{
		ShowID:      "midnight_signal",
		ShowName:    "Midnight Signal",
		HostID:      "liminal_operator",
		SegmentType: "deep_dive",
		Topic:       "The archaeology of memory",
	}

	audioPath1, metadataPath1, err := g.allocatePaths(req)
	if err != nil {
		t.Fatalf("allocatePaths() first call error = %v", err)
	}
	audioPath2, metadataPath2, err := g.allocatePaths(req)
	if err != nil {
		t.Fatalf("allocatePaths() second call error = %v", err)
	}

	if audioPath1 == audioPath2 {
		t.Fatalf("audio paths should differ, got %q", audioPath1)
	}
	if metadataPath1 == metadataPath2 {
		t.Fatalf("metadata paths should differ, got %q", metadataPath1)
	}
}
