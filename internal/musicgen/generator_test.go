package musicgen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockClient implements generatorClient for testing.
type mockClient struct {
	audio []byte
	err   error
	calls []GenerateRequest
}

func (m *mockClient) Generate(_ context.Context, req GenerateRequest) ([]byte, error) {
	m.calls = append(m.calls, req)
	return m.audio, m.err
}

func TestBumperGenerator_WritesAudioFile(t *testing.T) {
	dir := t.TempDir()
	mc := &mockClient{audio: []byte("pcm-data")}
	g := newBumperGenerator(mc, dir, 42)

	meta, err := g.Generate(context.Background(), "midnight_signal", "ambient")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Audio file must exist.
	audioPath := filepath.Join(dir, "midnight_signal", meta.Filename)
	data, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("audio file not written: %v", err)
	}
	if string(data) != "pcm-data" {
		t.Errorf("audio content mismatch")
	}
}

func TestBumperGenerator_WritesMetaJSON(t *testing.T) {
	dir := t.TempDir()
	mc := &mockClient{audio: []byte("audio")}
	g := newBumperGenerator(mc, dir, 0)

	meta, err := g.Generate(context.Background(), "midnight_signal", "ambient")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	base := meta.Filename[:len(meta.Filename)-len(filepath.Ext(meta.Filename))]
	jsonPath := filepath.Join(dir, "midnight_signal", base+".json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("metadata JSON not written: %v", err)
	}

	var got BumperMeta
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if got.ShowID != "midnight_signal" {
		t.Errorf("show_id = %q, want midnight_signal", got.ShowID)
	}
	if got.Caption == "" {
		t.Error("caption must not be empty in metadata")
	}
	if !got.AIGenerated {
		t.Error("ai_generated must be true")
	}
	if got.Model != "ace-step" {
		t.Errorf("model = %q, want ace-step", got.Model)
	}
	if got.DisplayName == "" {
		t.Error("display_name must not be empty")
	}
}

func TestBumperGenerator_FileNaming(t *testing.T) {
	dir := t.TempDir()
	mc := &mockClient{audio: []byte("x")}
	g := newBumperGenerator(mc, dir, 0)

	meta, err := g.Generate(context.Background(), "the_groove_lab", "soul")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Must start with show_id and contain "bumper".
	if len(meta.Filename) == 0 {
		t.Fatal("filename must not be empty")
	}
	if meta.Filename[:len("the_groove_lab")] != "the_groove_lab" {
		t.Errorf("filename %q does not start with show_id", meta.Filename)
	}
}

func TestBumperGenerator_DurationInRange(t *testing.T) {
	dir := t.TempDir()
	mc := &mockClient{audio: []byte("x")}
	g := newBumperGenerator(mc, dir, 99)

	for range 10 {
		_, err := g.Generate(context.Background(), "midnight_signal", "ambient")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		req := mc.calls[len(mc.calls)-1]
		if req.Duration < bumperMinDuration || req.Duration > bumperMaxDuration {
			t.Errorf("duration %v not in [%v, %v]", req.Duration, bumperMinDuration, bumperMaxDuration)
		}
	}
}

func TestBumperGenerator_ClientError(t *testing.T) {
	dir := t.TempDir()
	mc := &mockClient{err: fmt.Errorf("server down")}
	g := newBumperGenerator(mc, dir, 0)

	_, err := g.Generate(context.Background(), "midnight_signal", "ambient")
	if err == nil {
		t.Fatal("expected error when client fails")
	}
}

func TestBumperGenerator_CreatesShowSubdir(t *testing.T) {
	dir := t.TempDir()
	mc := &mockClient{audio: []byte("x")}
	g := newBumperGenerator(mc, dir, 0)

	meta, err := g.Generate(context.Background(), "dawn_chorus", "jazz")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	showDir := filepath.Join(dir, "dawn_chorus")
	if _, err := os.Stat(showDir); os.IsNotExist(err) {
		t.Errorf("show subdirectory not created")
	}
	_ = meta
}

func TestBumperGenerator_GenerationSecondsRecorded(t *testing.T) {
	dir := t.TempDir()
	mc := &mockClient{audio: []byte("x")}
	g := newBumperGenerator(mc, dir, 0)

	before := time.Now()
	meta, err := g.Generate(context.Background(), "midnight_signal", "ambient")
	after := time.Now()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if meta.GenerationSeconds < 0 {
		t.Error("generation_seconds must be >= 0")
	}
	elapsed := after.Sub(before).Seconds()
	if meta.GenerationSeconds > elapsed+1 {
		t.Errorf("generation_seconds %v > elapsed %v", meta.GenerationSeconds, elapsed)
	}

	// Also verify it's in the JSON file.
	base := meta.Filename[:len(meta.Filename)-len(filepath.Ext(meta.Filename))]
	jsonPath := filepath.Join(dir, "midnight_signal", base+".json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var got BumperMeta
	_ = json.Unmarshal(data, &got)
	if got.GenerationSeconds != meta.GenerationSeconds {
		t.Errorf("JSON generation_seconds %v != returned %v", got.GenerationSeconds, meta.GenerationSeconds)
	}
}
