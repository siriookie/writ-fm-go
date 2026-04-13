package musicgen

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

const (
	bumperMinDuration = 120.0 // seconds
	bumperMaxDuration = 240.0 // seconds
	bumperAudioFormat = "flac"
	bumperModel       = "ace-step"
	bumperInferSteps  = 25
)

// BumperMeta is the sidecar metadata written alongside each generated bumper.
type BumperMeta struct {
	ShowID            string    `json:"show_id"`
	Caption           string    `json:"caption"`
	DisplayName       string    `json:"display_name"`
	Duration          float64   `json:"duration"`
	Instrumental      bool      `json:"instrumental"`
	GeneratedAt       time.Time `json:"generated_at"`
	GenerationSeconds float64   `json:"generation_seconds"`
	AIGenerated       bool      `json:"ai_generated"`
	Model             string    `json:"model"`
	Filename          string    `json:"filename"`
}

// generatorClient is the consumer-side interface for audio generation.
type generatorClient interface {
	Generate(ctx context.Context, req GenerateRequest) ([]byte, error)
}

// BumperGenerator generates music bumpers and writes them to disk.
type BumperGenerator struct {
	client    generatorClient
	outputDir string
	rng       *rand.Rand
}

// NewBumperGenerator creates a BumperGenerator using the given client.
// outputDir is the root bumper directory (show subdirs are created automatically).
func NewBumperGenerator(client *Client, outputDir string) *BumperGenerator {
	return newBumperGenerator(client, outputDir, time.Now().UnixNano())
}

func newBumperGenerator(client generatorClient, outputDir string, seed int64) *BumperGenerator {
	return &BumperGenerator{
		client:    client,
		outputDir: outputDir,
		rng:       rand.New(rand.NewSource(seed)),
	}
}

// Generate creates one bumper for the given show and bumper style.
// It writes the audio file and a sidecar .json metadata file, then returns the metadata.
func (g *BumperGenerator) Generate(ctx context.Context, showID, bumperStyle string) (*BumperMeta, error) {
	entry := pickCaption(bumperStyle, g.rng)
	duration := bumperMinDuration + g.rng.Float64()*(bumperMaxDuration-bumperMinDuration)
	guidanceScale := 4.0 + g.rng.Float64()*6.0 // 4.0–10.0

	req := GenerateRequest{
		Caption:        entry.Caption,
		Duration:       duration,
		AudioFormat:    bumperAudioFormat,
		Seed:           -1,
		Instrumental:   entry.Instrumental,
		GuidanceScale:  guidanceScale,
		InferenceSteps: bumperInferSteps,
		Thinking:       false,   // requires LLM; set ENABLE_LM=1 on server to re-enable
		TimeSignature:  "4/4",   // avoid ACE-Step None crash in metadata_utils.py
	}
	if !entry.Instrumental {
		req.Lyrics = entry.Lyrics
	}

	start := time.Now()
	audio, err := g.client.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("musicgen: generate bumper for %s: %w", showID, err)
	}
	genSecs := time.Since(start).Seconds()

	now := time.Now()
	filename := fmt.Sprintf("%s_bumper_%s_%09d.%s", showID, now.Format("20060102_150405"), now.Nanosecond(), bumperAudioFormat)
	showDir := filepath.Join(g.outputDir, showID)
	if err := os.MkdirAll(showDir, 0o755); err != nil {
		return nil, fmt.Errorf("musicgen: create show dir: %w", err)
	}

	audioPath := filepath.Join(showDir, filename)
	if err := os.WriteFile(audioPath, audio, 0o644); err != nil {
		return nil, fmt.Errorf("musicgen: write audio file: %w", err)
	}

	meta := &BumperMeta{
		ShowID:            showID,
		Caption:           entry.Caption,
		DisplayName:       entry.DisplayName,
		Duration:          duration,
		Instrumental:      entry.Instrumental,
		GeneratedAt:       now,
		GenerationSeconds: genSecs,
		AIGenerated:       true,
		Model:             bumperModel,
		Filename:          filename,
	}

	jsonPath := filepath.Join(showDir, filename[:len(filename)-len(filepath.Ext(filename))]+".json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("musicgen: marshal metadata: %w", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("musicgen: write metadata: %w", err)
	}

	return meta, nil
}
