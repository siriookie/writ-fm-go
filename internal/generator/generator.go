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
)

var (
	// ErrScriptTooShort is returned when the generated script fails the quality gate.
	ErrScriptTooShort = errors.New("generator: script too short")
	slugNoiseRE       = regexp.MustCompile(`[^a-z0-9]+`)
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
	Headlines       string
	GuestName       string
	GuestContext    string
	Voices          map[string]string
}

// ScriptMetadata is the JSON sidecar persisted for generated scripts.
type ScriptMetadata struct {
	Type            string            `json:"type"`
	ShowID          string            `json:"show_id"`
	ShowName        string            `json:"show_name"`
	Host            string            `json:"host"`
	Topic           string            `json:"topic"`
	Script          string            `json:"script"`
	WordCount       int               `json:"word_count"`
	DurationSeconds *float64          `json:"duration_seconds"`
	Voices          map[string]string `json:"voices,omitempty"`
	GeneratedAt     time.Time         `json:"generated_at"`
	AudioPath       string            `json:"audio_path"`
	Status          string            `json:"status"`
}

// Result is the output of a successful generation run.
type Result struct {
	Prompt       string
	Script       string
	Topic        string
	AudioPath    string
	MetadataPath string
	WordCount    int
	Duration     float64
}

// LLMClient is the subset of llm.Client used by the generator core.
type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// AudioRenderer is the subset of renderer behavior used by generator orchestration.
type AudioRenderer interface {
	RenderSingle(ctx context.Context, script, voice, outputPath string) error
	RenderMulti(ctx context.Context, script string, voices map[string]string, outputPath string) error
	Duration(ctx context.Context, path string) (float64, error)
}

// Generator orchestrates prompt building, LLM calls, quality gates, rendering, and metadata persistence.
type Generator struct {
	llm             LLMClient
	renderer        AudioRenderer
	ttsBackend      string
	talkSegmentsDir string
	scriptsDir      string
	promptBuilder   *PromptBuilder
	topicPicker     func(string) string
	idGen           func() string
	now             func() time.Time
}

// New creates a generator core with explicit dependencies.
func New(client LLMClient, renderer AudioRenderer, ttsBackend, talkSegmentsDir, scriptsDir string) *Generator {
	return &Generator{
		llm:             client,
		renderer:        renderer,
		ttsBackend:      strings.TrimSpace(ttsBackend),
		talkSegmentsDir: talkSegmentsDir,
		scriptsDir:      scriptsDir,
		promptBuilder:   NewPromptBuilder(),
		topicPicker:     SelectTopic,
		idGen:           defaultID,
		now:             time.Now,
	}
}

// Generate creates a script, renders audio, and persists metadata.
func (g *Generator) Generate(ctx context.Context, req GenerateRequest) (*Result, error) {
	if req.Topic == "" {
		req.Topic = g.topicPicker(req.TopicFocus)
	}

	prompt, err := g.promptBuilder.Build(BuildRequest{
		HostID:          req.HostID,
		SegmentType:     req.SegmentType,
		Topic:           req.Topic,
		ShowName:        req.ShowName,
		ShowDescription: req.ShowDescription,
		TopicFocus:      req.TopicFocus,
		Headlines:       req.Headlines,
		GuestName:       req.GuestName,
		GuestContext:    req.GuestContext,
	})
	if err != nil {
		return nil, err
	}

	script, wordCount, err := g.generateScript(ctx, req.SegmentType, prompt)
	if err != nil {
		return nil, err
	}

	voices, err := g.resolveVoices(req)
	if err != nil {
		return nil, err
	}

	audioPath, metadataPath, err := g.allocatePaths(req)
	if err != nil {
		return nil, err
	}

	duration, err := g.renderAudio(ctx, req.SegmentType, script, voices, audioPath)
	if err != nil {
		return nil, err
	}

	if err := g.writeMetadata(req, script, wordCount, duration, voices, audioPath, metadataPath); err != nil {
		return nil, err
	}

	return &Result{
		Prompt:       prompt,
		Script:       script,
		Topic:        req.Topic,
		AudioPath:    audioPath,
		MetadataPath: metadataPath,
		WordCount:    wordCount,
		Duration:     duration,
	}, nil
}

func (g *Generator) generateScript(ctx context.Context, segmentType, prompt string) (string, int, error) {
	target, ok := SegmentLengthTargets[segmentType]
	if !ok {
		target = SegmentLengthTargets["deep_dive"]
	}
	minAcceptable := int(float64(target.Min) * 0.8)

	var lastErr error
	for range 2 {
		script, err := g.llm.Generate(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		wordCount := countTextUnits(script)
		if wordCount < minAcceptable {
			lastErr = fmt.Errorf("%w: got %d text units, need at least %d", ErrScriptTooShort, wordCount, minAcceptable)
			continue
		}
		return script, wordCount, nil
	}
	if lastErr == nil {
		lastErr = llm.ErrEmptyResponse
	}
	return "", 0, lastErr
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

func (g *Generator) writeMetadata(req GenerateRequest, script string, wordCount int, duration float64, voices map[string]string, audioPath, metadataPath string) error {
	now := g.now()
	meta := ScriptMetadata{
		Type:            req.SegmentType,
		ShowID:          req.ShowID,
		ShowName:        req.ShowName,
		Host:            req.HostID,
		Topic:           req.Topic,
		Script:          script,
		WordCount:       wordCount,
		DurationSeconds: floatPtr(duration),
		Voices:          cloneStringMap(voices),
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

func (g *Generator) renderAudio(ctx context.Context, segmentType, script string, voices map[string]string, audioPath string) (float64, error) {
	if g.renderer == nil {
		return 0, fmt.Errorf("generator: renderer is required")
	}

	var err error
	if isMultiVoiceSegment(segmentType) {
		err = g.renderer.RenderMulti(ctx, script, voices, audioPath)
	} else {
		err = g.renderer.RenderSingle(ctx, script, voices["host"], audioPath)
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
