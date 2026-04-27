package generator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/writ-fm/go/internal/generator/llm"
	"github.com/writ-fm/go/internal/generator/persona"
	"github.com/writ-fm/go/internal/news"
)

var (
	// ErrScriptTooShort is returned when the generated script fails the quality gate.
	ErrScriptTooShort        = errors.New("generator: script too short")
	errOutlineGenerationFail = errors.New("generator: outline generation failed")
	slugNoiseRE              = regexp.MustCompile(`[^a-z0-9]+`)
)

// GenerateRequest is the input for one script generation job.
type GenerateRequest struct {
	ShowID          string
	ShowName        string
	ShowDescription string
	HostID          string
	TopicFocus      string
	SegmentType     string
	Topic           string
	SourceMaterials string
	Headlines       string
	NewsMaterials   string
	NewsItems       []news.Headline
	GuestName       string
	GuestContext    string
	Voices          map[string]string
	PerformanceMode PerformanceMode
}

// ScriptMetadata is the JSON sidecar persisted for generated scripts.
type ScriptMetadata struct {
	Type            string            `json:"type"`
	ShowID          string            `json:"show_id"`
	ShowName        string            `json:"show_name"`
	Host            string            `json:"host"`
	Topic           string            `json:"topic"`
	SourceMaterials string            `json:"source_materials,omitempty"`
	Script          string            `json:"script"`
	WordCount       int               `json:"word_count"`
	DurationSeconds *float64          `json:"duration_seconds"`
	Voices          map[string]string `json:"voices,omitempty"`
	GenerationMode  string            `json:"generation_mode"`
	Outline         *Outline          `json:"outline,omitempty"`
	Segments        []OutlineSegment  `json:"segments,omitempty"`
	GeneratedAt     time.Time         `json:"generated_at"`
	AudioPath       string            `json:"audio_path"`
	Status          string            `json:"status"`
}

// Result is the output of a successful generation run.
type Result struct {
	Prompt          string
	Script          string
	Topic           string
	SourceMaterials string
	AudioPath       string
	MetadataPath    string
	WordCount       int
	Duration        float64
}

// LLMClient is the subset of llm.Client used by the generator core.
type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// AudioRenderer is the subset of renderer behavior used by generator orchestration.
type AudioRenderer interface {
	RenderSingle(ctx context.Context, script, voice, outputPath string, mode PerformanceMode) error
	RenderMulti(ctx context.Context, script string, voices map[string]string, outputPath string, mode PerformanceMode) error
	RenderParts(ctx context.Context, parts []ScriptPart, voices map[string]string, outputPath string, mode PerformanceMode) error
	Duration(ctx context.Context, path string) (float64, error)
}

// HeadlineProvider fetches current news headlines for prompt injection.
type HeadlineProvider interface {
	FetchHeadlines(ctx context.Context) ([]news.Headline, error)
}

type ArticleFetcher interface {
	FetchArticle(ctx context.Context, item news.Headline) (news.Headline, error)
}

// Generator orchestrates prompt building, LLM calls, quality gates, rendering, and metadata persistence.
type Generator struct {
	llm             LLMClient
	renderer        AudioRenderer
	ttsBackend      string
	talkSegmentsDir string
	scriptsDir      string
	promptBuilder   *PromptBuilder
	outlineMode     string
	topicPicker     func(string) string
	idGen           func() string
	now             func() time.Time
	headlines       HeadlineProvider
}

// Option customizes Generator construction.
type Option func(*Generator)

// WithHeadlineProvider injects the optional news provider used by news_analysis.
func WithHeadlineProvider(provider HeadlineProvider) Option {
	return func(g *Generator) {
		g.headlines = provider
	}
}

// New creates a generator core with explicit dependencies.
func New(client LLMClient, renderer AudioRenderer, ttsBackend, talkSegmentsDir, scriptsDir string, opts ...Option) *Generator {
	g := &Generator{
		llm:             client,
		renderer:        renderer,
		ttsBackend:      strings.TrimSpace(ttsBackend),
		talkSegmentsDir: talkSegmentsDir,
		scriptsDir:      scriptsDir,
		promptBuilder:   NewPromptBuilder(),
		outlineMode:     OutlineModeAuto,
		topicPicker:     SelectTopic,
		idGen:           defaultID,
		now:             time.Now,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// WithOutlineMode configures outline-first generation.
func WithOutlineMode(mode string) Option {
	return func(g *Generator) {
		g.outlineMode = normalizeOutlineMode(mode)
	}
}

// Generate creates a script, renders audio, and persists metadata.
func (g *Generator) Generate(ctx context.Context, req GenerateRequest) (*Result, error) {
	if strings.TrimSpace(req.Topic) == "" && req.SegmentType != "news_analysis" {
		if strings.TrimSpace(req.SourceMaterials) != "" {
			req.Topic = deriveTopicFromSourceMaterials(req.SourceMaterials)
		} else {
			req.Topic = g.topicPicker(req.TopicFocus)
		}
	}
	if err := g.injectHeadlines(ctx, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Topic) == "" {
		req.Topic = g.pickTopic(req)
	}

	promptReq := BuildRequest{
		HostID:          req.HostID,
		SegmentType:     req.SegmentType,
		Topic:           req.Topic,
		SourceMaterials: req.SourceMaterials,
		ShowName:        req.ShowName,
		ShowDescription: req.ShowDescription,
		TopicFocus:      req.TopicFocus,
		Headlines:       req.Headlines,
		NewsMaterials:   req.NewsMaterials,
		GuestName:       req.GuestName,
		GuestContext:    req.GuestContext,
		PerformanceMode: req.PerformanceMode,
	}

	voices, err := g.resolveVoices(req)
	if err != nil {
		return nil, err
	}

	audioPath, metadataPath, err := g.allocatePaths(req)
	if err != nil {
		return nil, err
	}

	if shouldUseOutlineFirst(g.outlineMode, req.SegmentType) {
		result, err := g.generateWithOutline(ctx, req, promptReq, voices, audioPath, metadataPath)
		if err == nil {
			return result, nil
		}
		return nil, err
	}

	prompt, script, wordCount, err := g.generateScript(ctx, req.SegmentType, promptReq)
	if err != nil {
		return nil, err
	}
	duration, err := g.renderAudio(ctx, req.SegmentType, script, voices, audioPath, req.PerformanceMode)
	if err != nil {
		return nil, err
	}

	if err := g.writeMetadata(req, script, wordCount, duration, voices, audioPath, metadataPath, GenerationModeSingleShot, nil, nil); err != nil {
		return nil, err
	}

	return &Result{
		Prompt:          prompt,
		Script:          script,
		Topic:           req.Topic,
		SourceMaterials: req.SourceMaterials,
		AudioPath:       audioPath,
		MetadataPath:    metadataPath,
		WordCount:       wordCount,
		Duration:        duration,
	}, nil
}

func (g *Generator) pickTopic(req GenerateRequest) string {
	if req.SegmentType == "news_analysis" && strings.TrimSpace(req.Headlines) != "" {
		return deriveNewsAnalysisTopic(req.Headlines)
	}
	return g.topicPicker(req.TopicFocus)
}

func (g *Generator) generateWithOutline(ctx context.Context, req GenerateRequest, promptReq BuildRequest, voices map[string]string, audioPath, metadataPath string) (*Result, error) {
	outlinePrompt, outline, err := g.generateOutline(ctx, req.SegmentType, promptReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errOutlineGenerationFail, err)
	}
	if req.SegmentType == "news_analysis" {
		promptReq.NewsMaterials = g.materialsForSelectedNews(ctx, req, outline)
	}

	segments, scripts, err := g.generateSegmentScripts(ctx, promptReq, outline)
	if err != nil {
		return nil, fmt.Errorf("%w: segment script generation: %v", errOutlineGenerationFail, err)
	}
	script := strings.Join(scripts, "\n\n")
	wordCount := countTextUnits(script)
	if err := validateFinalScriptLength(req.SegmentType, script); err != nil {
		return nil, fmt.Errorf("%w: final outline script: %v", errOutlineGenerationFail, err)
	}

	parts := make([]ScriptPart, 0, len(segments))
	for _, segment := range segments {
		parts = append(parts, ScriptPart{
			Index:    segment.Index,
			Title:    segment.Title,
			Script:   segment.Script,
			Speakers: append([]string(nil), segment.Speakers...),
		})
	}
	duration, err := g.renderScriptParts(ctx, parts, voices, audioPath, req.PerformanceMode)
	if err != nil {
		return nil, err
	}

	metaOutline := cloneOutline(outline)
	metaOutline.Segments = segments
	if err := g.writeMetadata(req, script, wordCount, duration, voices, audioPath, metadataPath, GenerationModeOutlineFirst, metaOutline, segments); err != nil {
		return nil, err
	}

	return &Result{
		Prompt:          outlinePrompt,
		Script:          script,
		Topic:           req.Topic,
		SourceMaterials: req.SourceMaterials,
		AudioPath:       audioPath,
		MetadataPath:    metadataPath,
		WordCount:       wordCount,
		Duration:        duration,
	}, nil
}

func (g *Generator) generateOutline(ctx context.Context, segmentType string, buildReq BuildRequest) (string, *Outline, error) {
	var lastErr error
	lastPrompt := ""
	for attempt := 0; attempt < defaultMaxAttempts; attempt++ {
		prompt, err := g.promptBuilder.BuildOutlinePrompt(buildReq)
		if err != nil {
			return "", nil, err
		}
		lastPrompt = prompt
		raw, err := g.llm.Generate(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		outline, err := parseOutline(raw, segmentType)
		if err != nil {
			lastErr = err
			continue
		}
		return lastPrompt, outline, nil
	}
	if lastErr == nil {
		lastErr = llm.ErrEmptyResponse
	}
	return lastPrompt, nil, lastErr
}

func (g *Generator) generateSegmentScripts(ctx context.Context, buildReq BuildRequest, outline *Outline) ([]OutlineSegment, []string, error) {
	segments := make([]OutlineSegment, len(outline.Segments))
	scripts := make([]string, len(outline.Segments))
	for i, segment := range outline.Segments {
		previousTranscript := strings.Join(scripts[:i], "\n\n")
		isFinal := i == len(outline.Segments)-1
		generated, err := g.generateOneSegmentScript(ctx, buildReq, outline, segment, previousTranscript, isFinal)
		if err != nil {
			return nil, nil, err
		}
		segment.Script = generated
		segment.WordCount = countTextUnits(generated)
		segments[i] = segment
		scripts[i] = generated
	}
	return segments, scripts, nil
}

func (g *Generator) generateOneSegmentScript(ctx context.Context, buildReq BuildRequest, outline *Outline, segment OutlineSegment, previousTranscript string, isFinal bool) (string, error) {
	var lastErr error
	for attempt := 0; attempt < defaultMaxAttempts; attempt++ {
		req := buildReq
		if attempt > 0 {
			req.RetryInstruction = fmt.Sprintf("上一轮没有生成可用正文。请只重写当前 outline 段落的最终中文口播正文，目标约 %d 个中文字或等价文本单位。即使 source materials 是英文，也必须改写成自然中文讲述，不要输出英文段落或英文 transcript。", segment.TargetLength)
		}
		prompt, err := g.promptBuilder.BuildSegmentScriptPrompt(req, outline, segment, previousTranscript, isFinal)
		if err != nil {
			return "", err
		}
		script, err := g.llm.Generate(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		if strings.TrimSpace(script) == "" || countTextUnits(script) == 0 {
			lastErr = fmt.Errorf("segment %d returned empty script", segment.Index)
			continue
		}
		if err := validateChineseScript(script); err != nil {
			lastErr = fmt.Errorf("segment %d must be Chinese: %w", segment.Index, err)
			continue
		}
		return script, nil
	}
	if lastErr == nil {
		lastErr = llm.ErrEmptyResponse
	}
	return "", lastErr
}

func validateFinalScriptLength(segmentType, script string) error {
	target, ok := SegmentLengthTargets[segmentType]
	if !ok {
		target = SegmentLengthTargets["deep_dive"]
	}
	gate := newQualityGate(target)
	units := countTextUnits(script)
	if !gate.accepted(units, gate.maxAttempts-1) {
		return fmt.Errorf("%w: got %d text units, need at least %d, sample=%q", ErrScriptTooShort, units, gate.minimumForAttempt(gate.maxAttempts-1), responseSnippet(script))
	}
	return nil
}

func (g *Generator) injectHeadlines(ctx context.Context, req *GenerateRequest) error {
	if req == nil || req.SegmentType != "news_analysis" || strings.TrimSpace(req.Headlines) != "" {
		return nil
	}
	if g.headlines == nil {
		return fmt.Errorf("generator: news_analysis requires RSS materials but no headline provider is configured")
	}

	headlines, err := g.headlines.FetchHeadlines(ctx)
	if err != nil {
		return fmt.Errorf("generator: fetch RSS materials for news_analysis: %w", err)
	}
	formatted := strings.TrimSpace(news.FormatHeadlines(headlines, 8))
	if formatted == "" {
		return fmt.Errorf("generator: no RSS materials available for news_analysis")
	}
	req.Headlines = formatted
	req.NewsItems = cloneNewsHeadlines(headlines)
	req.NewsMaterials = strings.TrimSpace(news.FormatDetailedMaterials(headlines[:1], 1, 1800))
	return nil
}

func (g *Generator) materialsForSelectedNews(ctx context.Context, req GenerateRequest, outline *Outline) string {
	if outline == nil || len(req.NewsItems) == 0 {
		return strings.TrimSpace(req.NewsMaterials)
	}
	index := outline.SelectedItemIndex
	if index < 1 || index > len(req.NewsItems) {
		index = 1
	}
	selected := req.NewsItems[index-1]
	if fetcher, ok := g.headlines.(ArticleFetcher); ok {
		if enriched, err := fetcher.FetchArticle(ctx, selected); err == nil {
			selected = enriched
		}
	}
	return strings.TrimSpace(news.FormatDetailedMaterials([]news.Headline{selected}, 1, 3600))
}

func (g *Generator) generateScript(ctx context.Context, segmentType string, buildReq BuildRequest) (string, string, int, error) {
	target, ok := SegmentLengthTargets[segmentType]
	if !ok {
		target = SegmentLengthTargets["deep_dive"]
	}
	gate := newQualityGate(target)

	var lastErr error
	lastPrompt := ""
	lastUnits := 0

	for attempt := 0; attempt < gate.maxAttempts; attempt++ {
		if attempt > 0 {
			buildReq.RetryInstruction = gate.retryInstruction(attempt-1, lastUnits)
		}

		prompt, err := g.promptBuilder.Build(buildReq)
		if err != nil {
			return "", "", 0, err
		}
		lastPrompt = prompt

		script, err := g.llm.Generate(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		wordCount := countTextUnits(script)
		lastUnits = wordCount
		if !gate.accepted(wordCount, attempt) {
			lastErr = fmt.Errorf("%w: got %d text units, need at least %d, sample=%q", ErrScriptTooShort, wordCount, gate.minimumForAttempt(attempt), responseSnippet(script))
			continue
		}
		if err := validateChineseScript(script); err != nil {
			lastErr = fmt.Errorf("script must be Chinese: %w", err)
			continue
		}
		return lastPrompt, script, wordCount, nil
	}

	if lastErr == nil {
		lastErr = llm.ErrEmptyResponse
	}
	return lastPrompt, "", 0, lastErr
}

func (g *Generator) allocatePaths(req GenerateRequest) (string, string, error) {
	now := g.now()
	timestamp := now.Format("20060102_150405")
	uniqueID := g.idGen()
	topicSlug := slugify(req.Topic, 30)

	showDir := filepath.Join(g.talkSegmentsDir, req.ShowID)
	if err := os.MkdirAll(showDir, 0o755); err != nil {
		return "", "", fmt.Errorf("generator: create show dir: %w", err)
	}
	if err := os.MkdirAll(g.scriptsDir, 0o755); err != nil {
		return "", "", fmt.Errorf("generator: create scripts dir: %w", err)
	}

	audioPath := filepath.Join(showDir, fmt.Sprintf("%s_%s_%s_%s.wav", req.SegmentType, topicSlug, timestamp, uniqueID))
	metadataPath := filepath.Join(g.scriptsDir, fmt.Sprintf("talk_%s_%s_%s.json", req.SegmentType, timestamp, uniqueID))
	return audioPath, metadataPath, nil
}

func (g *Generator) writeMetadata(req GenerateRequest, script string, wordCount int, duration float64, voices map[string]string, audioPath, metadataPath, generationMode string, outline *Outline, segments []OutlineSegment) error {
	now := g.now()
	meta := ScriptMetadata{
		Type:            req.SegmentType,
		ShowID:          req.ShowID,
		ShowName:        req.ShowName,
		Host:            req.HostID,
		Topic:           req.Topic,
		SourceMaterials: strings.TrimSpace(req.SourceMaterials),
		Script:          script,
		WordCount:       wordCount,
		DurationSeconds: floatPtr(duration),
		Voices:          cloneStringMap(voices),
		GenerationMode:  generationMode,
		Outline:         outline,
		Segments:        cloneOutlineSegments(segments),
		GeneratedAt:     now,
		AudioPath:       audioPath,
		Status:          "audio_rendered",
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("generator: marshal metadata: %w", err)
	}
	if err := os.WriteFile(metadataPath, data, 0o644); err != nil {
		return fmt.Errorf("generator: write metadata: %w", err)
	}
	return nil
}

func (g *Generator) renderAudio(ctx context.Context, segmentType, script string, voices map[string]string, audioPath string, mode PerformanceMode) (float64, error) {
	if g.renderer == nil {
		return 0, fmt.Errorf("generator: renderer is required")
	}

	var err error
	if isMultiVoiceSegment(segmentType) {
		err = g.renderer.RenderMulti(ctx, script, voices, audioPath, NormalizePerformanceMode(mode))
	} else {
		err = g.renderer.RenderSingle(ctx, script, voices["host"], audioPath, NormalizePerformanceMode(mode))
	}
	if err != nil {
		return 0, err
	}
	duration, err := g.renderer.Duration(ctx, audioPath)
	if err != nil {
		return 0, err
	}
	return duration, nil
}

func (g *Generator) renderScriptParts(ctx context.Context, parts []ScriptPart, voices map[string]string, audioPath string, mode PerformanceMode) (float64, error) {
	if g.renderer == nil {
		return 0, fmt.Errorf("generator: renderer is required")
	}
	if err := g.renderer.RenderParts(ctx, parts, voices, audioPath, NormalizePerformanceMode(mode)); err != nil {
		return 0, err
	}
	duration, err := g.renderer.Duration(ctx, audioPath)
	if err != nil {
		return 0, err
	}
	return duration, nil
}

func (g *Generator) resolveVoices(req GenerateRequest) (map[string]string, error) {
	voices := cloneStringMap(req.Voices)
	if voices == nil {
		voices = make(map[string]string, 2)
	}
	if strings.TrimSpace(voices["host"]) == "" {
		hostVoice, err := persona.GetHostVoice(req.HostID, g.voiceBackend())
		if err != nil {
			return nil, err
		}
		voices["host"] = hostVoice
	}
	if isMultiVoiceSegment(req.SegmentType) && strings.TrimSpace(voices["guest"]) == "" {
		guestVoice, err := persona.GetHostVoice("ember", g.voiceBackend())
		if err != nil {
			return nil, err
		}
		voices["guest"] = guestVoice
	}
	return voices, nil
}

func (g *Generator) voiceBackend() string {
	if strings.TrimSpace(g.ttsBackend) == "" {
		return "kokoro"
	}
	return g.ttsBackend
}

func isMultiVoiceSegment(segmentType string) bool {
	switch segmentType {
	case "interview", "panel":
		return true
	default:
		return false
	}
}

func slugify(value string, maxLen int) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = slugNoiseRE.ReplaceAllString(slug, "_")
	slug = strings.Trim(slug, "_")
	if slug == "" {
		slug = "segment"
	}
	if len(slug) > maxLen {
		slug = strings.Trim(slug[:maxLen], "_")
	}
	if slug == "" {
		return "segment"
	}
	return slug
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneNewsHeadlines(src []news.Headline) []news.Headline {
	if len(src) == 0 {
		return nil
	}
	dst := make([]news.Headline, len(src))
	copy(dst, src)
	return dst
}

func cloneOutlineSegments(src []OutlineSegment) []OutlineSegment {
	if len(src) == 0 {
		return nil
	}
	dst := make([]OutlineSegment, len(src))
	copy(dst, src)
	for i := range dst {
		dst[i].KeyPoints = append([]string(nil), src[i].KeyPoints...)
		dst[i].Speakers = append([]string(nil), src[i].Speakers...)
	}
	return dst
}

func responseSnippet(text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	runes := []rune(text)
	if len(runes) > 120 {
		return string(runes[:120]) + "..."
	}
	return text
}

func defaultID() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func floatPtr(v float64) *float64 {
	return &v
}
