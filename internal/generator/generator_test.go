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
	partCalls   []renderPartsCall
	duration    float64
	durationErr error
	renderErr   error
}

type fakeHeadlineProvider struct {
	items        []news.Headline
	articleByURL map[string]news.Headline
	err          error
	calls        int
	articleCalls int
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

func (f *fakeHeadlineProvider) FetchArticle(ctx context.Context, item news.Headline) (news.Headline, error) {
	_ = ctx
	f.articleCalls++
	if f.articleByURL == nil {
		return item, nil
	}
	if enriched, ok := f.articleByURL[item.Link]; ok {
		if enriched.Title == "" {
			enriched.Title = item.Title
		}
		if enriched.Source == "" {
			enriched.Source = item.Source
		}
		if enriched.Summary == "" {
			enriched.Summary = item.Summary
		}
		if enriched.Link == "" {
			enriched.Link = item.Link
		}
		if enriched.Published == "" {
			enriched.Published = item.Published
		}
		return enriched, nil
	}
	return item, nil
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

type renderPartsCall struct {
	Parts  []ScriptPart
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

func (f *fakeRenderer) RenderParts(ctx context.Context, parts []ScriptPart, voices map[string]string, outputPath string, mode PerformanceMode) error {
	_ = ctx
	if f.renderErr != nil {
		return f.renderErr
	}
	copied := make([]ScriptPart, len(parts))
	copy(copied, parts)
	f.partCalls = append(f.partCalls, renderPartsCall{Parts: copied, Voices: cloneStringMap(voices), Path: outputPath, Mode: mode})
	var builder strings.Builder
	for i, part := range parts {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(part.Script)
	}
	return os.WriteFile(outputPath, []byte(builder.String()), 0o644)
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
		responses: []string{strings.Repeat("\u591c", 2200)},
	}
	renderer := &fakeRenderer{duration: 123.5}
	talkDir := filepath.Join(t.TempDir(), "talk")
	scriptsDir := filepath.Join(t.TempDir(), "scripts")
	g := New(fake, renderer, "kokoro", talkDir, scriptsDir, WithOutlineMode(OutlineModeOff))
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
		responses: []string{strings.Repeat("HOST: 你好，今晚我们从一段旧录音讲起。GUEST: 我记得那个声音，也记得它背后的城市。", 300)},
	}
	renderer := &fakeRenderer{duration: 88}
	g := New(fake, renderer, "microsoft", t.TempDir(), t.TempDir(), WithOutlineMode(OutlineModeOff))
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
			strings.Repeat("\u591c", 2200),
		},
	}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithOutlineMode(OutlineModeOff))
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
	if len(fake.prompts) < 2 || !strings.Contains(fake.prompts[1], "1760") {
		t.Fatalf("retry prompt missing revision instructions:\n%s", strings.Join(fake.prompts, "\n---\n"))
	}
}

func TestGeneratorGenerateFailsAfterShortRetries(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{responses: []string{"short", "still short", "tiny"}}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithOutlineMode(OutlineModeOff))
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
			strings.Repeat("\u591c", 1000),
			strings.Repeat("\u591c", 1200),
			strings.Repeat("\u591c", 1450),
		},
	}
	renderer := &fakeRenderer{duration: 10}
	g := New(fake, renderer, "kokoro", t.TempDir(), t.TempDir(), WithOutlineMode(OutlineModeOff))
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
		responses: []string{strings.Repeat("\u591c", 2200)},
	}
	headlines := &fakeHeadlineProvider{
		items: []news.Headline{
			{Title: "first headline", Source: "BBC Chinese"},
			{Title: "second headline", Source: "NPR"},
		},
	}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithHeadlineProvider(headlines), WithOutlineMode(OutlineModeOff))
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
		Topic:           "weekly news",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if headlines.calls != 1 {
		t.Fatalf("headline calls = %d, want 1", headlines.calls)
	}
	if len(fake.prompts) != 1 || !strings.Contains(fake.prompts[0], "1. [BBC Chinese] first headline") {
		t.Fatalf("prompt missing injected headlines:\n%s", strings.Join(fake.prompts, "\n---\n"))
	}
}

func TestGeneratorGenerateDerivesNewsTopicFromRSSWhenTopicIsEmpty(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{
		responses: []string{strings.Repeat("\u591c", 2200)},
	}
	headlines := &fakeHeadlineProvider{
		items: []news.Headline{
			{Title: "white house dinner shooting arrest", Source: "BBC"},
			{Title: "BYD grows without US market", Source: "BBC"},
		},
	}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithHeadlineProvider(headlines), WithOutlineMode(OutlineModeOff))
	g.topicPicker = func(string) string {
		return "static current events topic"
	}

	result, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:          "signal_report",
		ShowName:        "Signal Report",
		ShowDescription: "News through a late-night lens.",
		HostID:          "signal",
		TopicFocus:      "current_events",
		SegmentType:     "news_analysis",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !strings.Contains(result.Topic, "white house dinner shooting arrest") {
		t.Fatalf("topic should be derived from RSS, got %q", result.Topic)
	}
	if strings.Contains(result.Topic, "static current events topic") {
		t.Fatalf("topic should not use static current_events pool, got %q", result.Topic)
	}
	if len(fake.prompts) != 1 || !strings.Contains(fake.prompts[0], result.Topic) {
		t.Fatalf("prompt missing derived topic %q:\n%s", result.Topic, strings.Join(fake.prompts, "\n---\n"))
	}
}

func TestGeneratorGenerateDerivesTopicFromSourceMaterials(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{responses: []string{strings.Repeat("\u591c", 2200)}}
	renderer := &fakeRenderer{duration: 10}
	g := New(fake, renderer, "kokoro", t.TempDir(), t.TempDir(), WithOutlineMode(OutlineModeOff))
	g.topicPicker = func(string) string {
		return "generic fallback topic"
	}

	result, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:          "midnight_signal",
		ShowName:        "Midnight Signal",
		ShowDescription: "Story after midnight.",
		HostID:          "liminal_operator",
		TopicFocus:      "philosophy",
		SegmentType:     "deep_dive",
		SourceMaterials: "# 旧影院最后一晚\n人物：旧影院放映员\n事件：城市拆迁前的最后一场放映",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Topic != "旧影院最后一晚" {
		t.Fatalf("Topic = %q, want source-derived topic", result.Topic)
	}
	if !strings.Contains(fake.prompts[0], "旧影院放映员") {
		t.Fatalf("prompt missing source materials:\n%s", fake.prompts[0])
	}
	data, err := os.ReadFile(result.MetadataPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var meta ScriptMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !strings.Contains(meta.SourceMaterials, "城市拆迁前的最后一场放映") {
		t.Fatalf("metadata missing source materials: %#v", meta.SourceMaterials)
	}
}

func TestGeneratorGenerateRequiresNewsHeadlinesForNewsAnalysis(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithOutlineMode(OutlineModeOff))

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:      "signal_report",
		ShowName:    "Signal Report",
		HostID:      "signal",
		SegmentType: "news_analysis",
		Topic:       "Information vacuum",
	})
	if err == nil || !strings.Contains(err.Error(), "requires RSS materials") {
		t.Fatalf("Generate() error = %v, want missing RSS materials error", err)
	}
	if fake.calls != 0 {
		t.Fatalf("LLM calls = %d, want 0", fake.calls)
	}
}

func TestGeneratorGenerateReturnsHeadlineFetchErrorForNewsAnalysis(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{}
	headlines := &fakeHeadlineProvider{err: errors.New("rss offline")}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithHeadlineProvider(headlines), WithOutlineMode(OutlineModeOff))

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:      "signal_report",
		ShowName:    "Signal Report",
		HostID:      "signal",
		SegmentType: "news_analysis",
		Topic:       "Information vacuum",
	})
	if err == nil || !strings.Contains(err.Error(), "fetch RSS materials") {
		t.Fatalf("Generate() error = %v, want RSS fetch error", err)
	}
	if headlines.calls != 1 {
		t.Fatalf("headline calls = %d, want 1", headlines.calls)
	}
	if fake.calls != 0 {
		t.Fatalf("LLM calls = %d, want 0", fake.calls)
	}
}

func TestGeneratorGenerateReturnsEmptyHeadlineErrorForNewsAnalysis(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{}
	headlines := &fakeHeadlineProvider{}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithHeadlineProvider(headlines), WithOutlineMode(OutlineModeOff))

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:      "signal_report",
		ShowName:    "Signal Report",
		HostID:      "signal",
		SegmentType: "news_analysis",
		Topic:       "Information vacuum",
	})
	if err == nil || !strings.Contains(err.Error(), "no RSS materials") {
		t.Fatalf("Generate() error = %v, want empty RSS materials error", err)
	}
	if fake.calls != 0 {
		t.Fatalf("LLM calls = %d, want 0", fake.calls)
	}
}

func TestGeneratorGenerateIncludesNewsSummariesInOutlinePrompt(t *testing.T) {
	t.Parallel()

	outlineJSON := `{
		"title":"Information vacuum",
		"topic":"Information vacuum",
		"segment_type":"news_analysis",
		"overall_goal":"Analyze the structure behind a breaking news cycle.",
		"emotional_curve":"calm to focused",
		"selected_item_index":2,
		"selected_item_title":"Selected investigation",
		"selected_item_link":"https://example.com/selected",
		"segments":[
			{"index":1,"title":"Fact boundary","goal":"Separate facts from inference","key_points":["confirmed facts"],"target_length":500,"emotion":"calm","pacing":"measured","speakers":["HOST"],"transition":"next"},
			{"index":2,"title":"Narrative pressure","goal":"Explain political narrative pressure","key_points":["narrative pressure"],"target_length":500,"emotion":"focused","pacing":"steady","speakers":["HOST"],"transition":"next"},
			{"index":3,"title":"Close","goal":"Land the judgment","key_points":["judgment"],"target_length":500,"emotion":"resolved","pacing":"slow","speakers":["HOST"],"transition":"end"}
		]
	}`
	longSegment := strings.Repeat("这是一段中文新闻分析，围绕事实、责任和现场信息继续展开。", 40)
	fake := &fakeLLM{
		responses: []string{outlineJSON, longSegment, longSegment, longSegment},
	}
	headlines := &fakeHeadlineProvider{
		items: []news.Headline{
			{
				Title:     "Breaking event under investigation",
				Source:    "BBC",
				Summary:   "Officials confirmed one arrest while investigators are still separating motive from speculation.",
				Content:   "This is the fuller article body with witness detail, agency response, and legal process context that should be visible during segment writing.",
				Comments:  "Reader comments questioned the security perimeter and official timeline.",
				Link:      "https://example.com/news",
				Published: "2026-04-27",
			},
		},
	}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithHeadlineProvider(headlines))

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:      "signal_report",
		ShowName:    "Signal Report",
		HostID:      "signal",
		SegmentType: "news_analysis",
		Topic:       "Information vacuum",
		Voices:      map[string]string{"host": "am_onyx"},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(fake.prompts) == 0 {
		t.Fatal("no prompts captured")
	}
	if !strings.Contains(fake.prompts[0], "RSS") {
		t.Fatalf("outline prompt missing RSS material section:\n%s", fake.prompts[0])
	}
	if !strings.Contains(fake.prompts[0], "Officials confirmed one arrest") {
		t.Fatalf("outline prompt missing RSS summary:\n%s", fake.prompts[0])
	}
}

func TestGeneratorGeneratePassesFullNewsMaterialsToSegmentScripts(t *testing.T) {
	t.Parallel()

	outlineJSON := `{
		"title":"Information vacuum",
		"topic":"Information vacuum",
		"segment_type":"news_analysis",
		"overall_goal":"Analyze the structure behind a breaking news cycle.",
		"emotional_curve":"calm to focused",
		"selected_item_index":2,
		"selected_item_title":"Selected investigation",
		"selected_item_link":"https://example.com/selected",
		"segments":[
			{"index":1,"title":"Fact boundary","goal":"Separate facts from inference","key_points":["confirmed facts"],"target_length":500,"emotion":"calm","pacing":"measured","speakers":["HOST"],"transition":"next"},
			{"index":2,"title":"Narrative pressure","goal":"Explain political narrative pressure","key_points":["narrative pressure"],"target_length":500,"emotion":"focused","pacing":"steady","speakers":["HOST"],"transition":"next"},
			{"index":3,"title":"Close","goal":"Land the judgment","key_points":["judgment"],"target_length":500,"emotion":"resolved","pacing":"slow","speakers":["HOST"],"transition":"end"}
		]
	}`
	longSegment := strings.Repeat("这是一段中文新闻分析，围绕事实、责任和现场信息继续展开。", 40)
	fake := &fakeLLM{
		responses: []string{outlineJSON, longSegment, longSegment, longSegment},
	}
	headlines := &fakeHeadlineProvider{
		items: []news.Headline{
			{
				Title:   "Background item",
				Source:  "BBC",
				Summary: "This should stay in the outline-only background material.",
				Link:    "https://example.com/background",
			},
			{
				Title:   "Selected investigation",
				Source:  "BBC",
				Summary: "RSS summary for the selected item.",
				Link:    "https://example.com/selected",
			},
		},
		articleByURL: map[string]news.Headline{
			"https://example.com/selected": {
				Content:  "This is the selected article body fetched from the linked page with witness detail, agency response, and legal process context.",
				Comments: "Reader comments questioned the selected article timeline.",
			},
		},
	}
	g := New(fake, &fakeRenderer{duration: 10}, "kokoro", t.TempDir(), t.TempDir(), WithHeadlineProvider(headlines))

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:      "signal_report",
		ShowName:    "Signal Report",
		HostID:      "signal",
		SegmentType: "news_analysis",
		Topic:       "Information vacuum",
		Voices:      map[string]string{"host": "am_onyx"},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(fake.prompts) < 2 {
		t.Fatalf("expected segment script prompts, got %d prompt(s)", len(fake.prompts))
	}
	if headlines.articleCalls != 1 {
		t.Fatalf("article fetch calls = %d, want 1", headlines.articleCalls)
	}
	for _, want := range []string{"source_materials", "selected article body fetched from the linked page", "Reader comments questioned the selected article timeline"} {
		if !strings.Contains(fake.prompts[1], want) {
			t.Fatalf("segment script prompt missing %q:\n%s", want, fake.prompts[1])
		}
	}
	if strings.Contains(fake.prompts[1], "This should stay in the outline-only background material") {
		t.Fatalf("segment script prompt should only include selected article material:\n%s", fake.prompts[1])
	}
}

func TestGeneratorGenerateUsesOutlineFirstForLongSegments(t *testing.T) {
	t.Parallel()

	outlineJSON := `{
		"title":"Memory map",
		"topic":"The archaeology of memory",
		"segment_type":"deep_dive",
		"overall_goal":"Build a layered argument.",
		"emotional_curve":"calm to reflective",
		"segments":[
			{"index":1,"title":"Door","goal":"Open the topic","key_points":["image"],"target_length":2,"emotion":"calm","pacing":"slow","speakers":["HOST"],"transition":"next"},
			{"index":2,"title":"Archive","goal":"Add context","key_points":["history"],"target_length":2,"emotion":"curious","pacing":"measured","speakers":["HOST"],"transition":"next"},
			{"index":3,"title":"Signal","goal":"Make the claim","key_points":["claim"],"target_length":2,"emotion":"focused","pacing":"steady","speakers":["HOST"],"transition":"next"},
			{"index":4,"title":"Return","goal":"Close with resonance","key_points":["close"],"target_length":2,"emotion":"warm","pacing":"slow","speakers":["HOST"],"transition":"end"}
		]
	}`
	longSegment := strings.Repeat("这是一段中文记忆讲述，继续展开声音、时间和被保存下来的细节。", 35)
	fake := &fakeLLM{
		responses: []string{outlineJSON, "这是一段简短的中文开场。", longSegment, longSegment, longSegment},
	}
	renderer := &fakeRenderer{duration: 42}
	g := New(fake, renderer, "kokoro", t.TempDir(), t.TempDir())
	fakeNow := time.Date(2026, 4, 14, 22, 5, 6, 0, time.Local)
	g.now = func() time.Time { return fakeNow }
	g.idGen = func() string { return "outlineid" }
	g.promptBuilder = NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time { return fakeNow }).BuildHostPrompt,
		func(n int) int { return 0 },
	)

	result, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:      "midnight_signal",
		ShowName:    "Midnight Signal",
		HostID:      "liminal_operator",
		SegmentType: "deep_dive",
		Topic:       "The archaeology of memory",
		Voices:      map[string]string{"host": "am_michael"},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if fake.calls != 5 {
		t.Fatalf("LLM calls = %d, want 5", fake.calls)
	}
	if !strings.Contains(fake.prompts[2], "这是一段简短的中文开场") {
		t.Fatalf("second segment prompt missing first segment transcript:\n%s", fake.prompts[2])
	}
	if !strings.Contains(fake.prompts[4], "Return") {
		t.Fatalf("final segment prompt missing final instruction:\n%s", fake.prompts[4])
	}
	if len(renderer.partCalls) != 1 {
		t.Fatalf("RenderParts calls = %d, want 1", len(renderer.partCalls))
	}
	if len(renderer.partCalls[0].Parts) != 4 {
		t.Fatalf("rendered parts = %d, want 4", len(renderer.partCalls[0].Parts))
	}
	if renderer.partCalls[0].Parts[2].Script != longSegment {
		t.Fatalf("third script = %q", renderer.partCalls[0].Parts[2].Script)
	}

	data, err := os.ReadFile(result.MetadataPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var meta ScriptMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if meta.GenerationMode != GenerationModeOutlineFirst {
		t.Fatalf("GenerationMode = %q, want outline_first", meta.GenerationMode)
	}
	if meta.Outline == nil || len(meta.Segments) != 4 {
		t.Fatalf("metadata outline/segments missing: %#v %#v", meta.Outline, meta.Segments)
	}
	if meta.Segments[0].Script != "这是一段简短的中文开场。" {
		t.Fatalf("first metadata script = %q", meta.Segments[0].Script)
	}
}

func TestGeneratorGenerateAutoKeepsShortSegmentsSingleShot(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{responses: []string{"WRIT FM，午夜信号回来。今晚的声音慢一点，也更靠近你。"}}
	renderer := &fakeRenderer{duration: 3}
	g := New(fake, renderer, "kokoro", t.TempDir(), t.TempDir())

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:      "midnight_signal",
		ShowName:    "Midnight Signal",
		HostID:      "liminal_operator",
		SegmentType: "station_id",
		Topic:       "Station ID",
		Voices:      map[string]string{"host": "am_michael"},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(renderer.singleCalls) != 1 {
		t.Fatalf("single calls = %d, want 1", len(renderer.singleCalls))
	}
	if len(renderer.partCalls) != 0 {
		t.Fatalf("part calls = %d, want 0", len(renderer.partCalls))
	}
}

func TestGeneratorGenerateReturnsOutlineErrorWhenOutlineIsNotJSON(t *testing.T) {
	t.Parallel()

	fake := &fakeLLM{
		responses: []string{
			"闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偞鐗犻、鏇氱秴闁搞儺鍓﹂弫鍐煥閺囨浜鹃梺姹囧€楅崑鎾舵崲濠靛洨绡€闁稿本绮岄·鈧梻浣侯焾椤戞垹鎹㈠┑瀣摕鐎广儱顦导鐘绘煕閺囥劌澧繛鍜冪秮濮婂宕掑顑跨敖闂佹悶鍔岄悥鐓庮嚕婵犳碍鏅插璺猴功椤撴椽姊洪幐搴ｇ畵婵☆偒鍘艰灋闁告洦鍨遍埛鎴︽煕濞戞﹫鍔熼柟鍐插暣閺屻劑寮村Ο鍝勫Е婵犵绱曢弫璇茬暦閻旂⒈鏁嶆慨姗嗗墮閺併倝姊绘笟鈧褔鈥﹂崼銉ョ？闁哄被鍎辩粻姘箾閹寸偟鎳呯紒鐘荤畺閹鈽夊▎妯煎姺闂佺粯绮忓畷鐢垫閹烘鍤戦柤鍝ユ暩閵嗗﹤鈹戦垾鍐茬骇闁告梹鐟╅悰顕€骞掗幊铏閸┾偓妞ゆ帒鍊绘稉宥夋煛鐏炶鍔滈柣鎾寸懃闇夐柣鎾虫捣閹界姵绻涢幊宄板缁犻箖鏌涢銈呮灁缂佺姵顭囩槐鎺撴綇閵婏箑闉嶉梺鐟板槻閹冲繒绮嬮幒鏂哄亾閿濆簼绨奸柟铏懅缁辨捇宕掑▎鎰偘濡炪倖娉﹂崶褏鏌堥梺瑙勵問閸犳牠宕瑰┑鍥╃闁糕剝锚婵洨绱掗崜浣镐粶闁宠鍨块幃鈺呭蓟閵夘喒鍋撳Δ鍛厵妞ゆ棁妫勯悘瀛樻叏婵犲懏顏犻柛鏍ㄧ墵瀵挳鎮欏ù瀣珶濠?JSON",
			"婵犵數濮烽弫鍛婃叏閻戣棄鏋侀柟闂寸绾剧粯绻涢幋娆忕労闁轰礁顑嗛妵鍕箻鐠虹儤鐎鹃梺鍛婄懃缁绘﹢骞冨Δ鍛棃婵炴垶鐟﹂崰鎰版⒑濞茶骞楅柟鐟版喘瀵鈽夐姀鈺傛櫇闂佹寧绻傚Λ娑⑺囬妷褏纾藉ù锝嗗絻娴滅偓绻濋姀锝嗙【闁哄牜鍓熷畷浼村箛閺夎法顔愰柡澶婄墕婢т粙骞冩總鍛婄厽妞ゆ挾鍠撻幊鍐┿亜椤撯剝纭堕柟鐟板缁楃喖顢涘鍐ㄧ稻闂傚倷绀侀幉锟犳嚌妤ｅ喚鏁勫鑸靛姇閽冪喐绻涢幋娆忕仼闁绘帗妞介弻娑㈠箛椤掆偓閳锋棃鏌?JSON",
			"闂傚倸鍊搁崐鎼佸磹妞嬪海鐭嗗〒姘ｅ亾妤犵偞鐗犻、鏇氱秴闁搞儺鍓﹂弫鍐煥閺囨浜鹃梺姹囧€楅崑鎾舵崲濠靛洨绡€闁稿本渚楀Λ锟犳⒑閻熸澘鏆遍柡鍫墰閹广垹鈹戦崼婵囩€冲┑鈽嗗灠閹碱偊鍩涙径鎰仭婵犲﹤瀚欢鏌ユ煕閻斿憡灏﹂柣娑卞枟瀵板嫰骞囬鐔哥彨闂備礁鎲″ú锕傚储妤ｅ啯鍋傞柟鎯ь嚟缁♀偓闂侀潧楠忕徊鍓ф兜閻愵兙浜滄い鎾楀啫鈷嬮悗瑙勬磸閸庢娊鍩€椤掑﹦绉甸柛鐘愁殜閹繝寮撮姀锛勫帗闂佸疇妗ㄧ粈渚€寮搁妶澶嬬厽?JSON",
		},
	}
	renderer := &fakeRenderer{duration: 12}
	g := New(fake, renderer, "kokoro", t.TempDir(), t.TempDir())
	g.idGen = func() string { return "fallbackid" }

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:      "signal_report",
		ShowName:    "Signal Report",
		HostID:      "signal",
		SegmentType: "news_analysis",
		Topic:       "RSS style briefing",
		Headlines:   "1. [RSS] Known material\n   濠电姷鏁告慨鐑藉极閸涘﹥鍙忛柣銏犲閺佸﹪鏌″搴″箹缂佹劖顨嗘穱濠囧Χ閸涱厽娈查悗瑙勬礃閻擄繝寮婚悢鍏肩劷闁挎洍鍋撻柡瀣〒缁辨帡鐓幓鎺嗗亾閺囷紕浜欓梻浣瑰缁诲倿骞婃惔銊ユ辈婵炲棙鎸婚悡鏇㈡煙鐎电孝闁宠鐗撻弻锛勪沪閸撗勫垱濡ょ姷鍋涘ú顓炍涢崘銊㈡婵炲棙鍨抽柇顖滅磽閸屾艾鈧兘鎳楅懜鍨弿闁绘垼妫勭壕鍧楁煏閸繃顥犻柍鐟扮Т閳规垿鎮╅崣澶屻偐闂佽桨绀佺粔鐢垫崲濞戙垹绠ｆ繝闈涙閸╃偤姊洪崨濠冪叆闁兼椿鍨堕崺鐐哄箣閿旇棄鈧嘲螞閻楀牏绠撴繛鍫熺箘缁辨捇宕掑姣欙紕绱掗懜浣冨妞ゆ洩绲剧换婵嗩潩椤戔斁鏅犻弻鏇熷緞閸繄浠惧銈庡亜缁夋挳鍩為幋锔藉€烽柛娆忣樈濡繝姊洪幐搴″摵闁?known source item for this test.",
		Voices:      map[string]string{"host": "am_onyx"},
	})
	if err == nil || !strings.Contains(err.Error(), "outline generation failed") {
		t.Fatalf("Generate() error = %v, want outline generation failure", err)
	}
	if fake.calls != 3 {
		t.Fatalf("LLM calls = %d, want 3", fake.calls)
	}
	if len(renderer.singleCalls) != 0 {
		t.Fatalf("single calls = %d, want no fallback single-shot render", len(renderer.singleCalls))
	}
	if len(renderer.partCalls) != 0 {
		t.Fatalf("part calls = %d, want 0 after outline failure", len(renderer.partCalls))
	}
}

func TestGeneratorGenerateReturnsOutlineErrorWhenOutlineSegmentScriptIsTooShort(t *testing.T) {
	t.Parallel()

	outlineJSON := `{
		"title":"Briefing map",
		"topic":"RSS style briefing",
		"segment_type":"news_analysis",
		"overall_goal":"Explain the news structure.",
		"emotional_curve":"calm to sharp",
		"segments":[
			{"index":1,"title":"Open","goal":"Set the frame","key_points":["frame"],"target_length":100,"emotion":"calm","pacing":"measured","speakers":["HOST"],"transition":"next"},
			{"index":2,"title":"Context","goal":"Add background","key_points":["context"],"target_length":100,"emotion":"focused","pacing":"steady","speakers":["HOST"],"transition":"next"},
			{"index":3,"title":"Stakes","goal":"Name the stakes","key_points":["stakes"],"target_length":100,"emotion":"tense","pacing":"firm","speakers":["HOST"],"transition":"end"}
		]
	}`
	fake := &fakeLLM{
		responses: []string{
			outlineJSON,
			"too short",
			"still short",
			"tiny",
		},
	}
	renderer := &fakeRenderer{duration: 12}
	g := New(fake, renderer, "kokoro", t.TempDir(), t.TempDir())
	g.idGen = func() string { return "segmentfallbackid" }

	_, err := g.Generate(context.Background(), GenerateRequest{
		ShowID:      "signal_report",
		ShowName:    "Signal Report",
		HostID:      "signal",
		SegmentType: "news_analysis",
		Topic:       "RSS style briefing",
		Headlines:   "1. [RSS] Known material\n   濠电姷鏁告慨鐑藉极閸涘﹥鍙忛柣銏犲閺佸﹪鏌″搴″箹缂佹劖顨嗘穱濠囧Χ閸涱厽娈查悗瑙勬礃閻擄繝寮婚悢鍏肩劷闁挎洍鍋撻柡瀣〒缁辨帡鐓幓鎺嗗亾閺囷紕浜欓梻浣瑰缁诲倿骞婃惔銊ユ辈婵炲棙鎸婚悡鏇㈡煙鐎电孝闁宠鐗撻弻锛勪沪閸撗勫垱濡ょ姷鍋涘ú顓炍涢崘銊㈡婵炲棙鍨抽柇顖滅磽閸屾艾鈧兘鎳楅懜鍨弿闁绘垼妫勭壕鍧楁煏閸繃顥犻柍鐟扮Т閳规垿鎮╅崣澶屻偐闂佽桨绀佺粔鐢垫崲濞戙垹绠ｆ繝闈涙閸╃偤姊洪崨濠冪叆闁兼椿鍨堕崺鐐哄箣閿旇棄鈧嘲螞閻楀牏绠撴繛鍫熺箘缁辨捇宕掑姣欙紕绱掗懜浣冨妞ゆ洩绲剧换婵嗩潩椤戔斁鏅犻弻鏇熷緞閸繄浠惧銈庡亜缁夋挳鍩為幋锔藉€烽柛娆忣樈濡繝姊洪幐搴″摵闁?known source item for this test.",
		Voices:      map[string]string{"host": "am_onyx"},
	})
	if err == nil || !strings.Contains(err.Error(), "outline generation failed") {
		t.Fatalf("Generate() error = %v, want outline generation failure", err)
	}
	if fake.calls != 4 {
		t.Fatalf("LLM calls = %d, want 4", fake.calls)
	}
	if len(renderer.singleCalls) != 0 {
		t.Fatalf("single calls = %d, want no fallback single-shot render", len(renderer.singleCalls))
	}
	if len(renderer.partCalls) != 0 {
		t.Fatalf("part calls = %d, want 0 after outline failure", len(renderer.partCalls))
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
