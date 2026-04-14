package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gen "github.com/writ-fm/go/internal/generator"
)

type fakeGenerateService struct {
	requests []gen.GenerateRequest
	err      error
}

func (f *fakeGenerateService) Generate(ctx context.Context, req gen.GenerateRequest) (*gen.Result, error) {
	_ = ctx
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_%d.wav", req.ShowID, len(f.requests)))
	return &gen.Result{
		AudioPath: audioPath,
		WordCount: 1600,
		Duration:  120,
	}, nil
}

func TestRunGenerate_UsesExplicitShow(t *testing.T) {
	cfg := config{
		SchedulePath:    filepath.Join("..", "..", "config", "schedule.yaml"),
		TalkSegmentsDir: t.TempDir(),
		ScriptsDir:      t.TempDir(),
	}

	fake := &fakeGenerateService{}
	origBuild := buildGeneratorFn
	buildGeneratorFn = func(cfg config) (generateService, error) { return fake, nil }
	defer func() { buildGeneratorFn = origBuild }()

	if err := runGenerate(context.Background(), cfg, "midnight_signal", "deep_dive", "The archaeology of memory", 2); err != nil {
		t.Fatalf("runGenerate() error = %v", err)
	}
	if len(fake.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(fake.requests))
	}
	if fake.requests[0].ShowID != "midnight_signal" {
		t.Fatalf("ShowID = %q", fake.requests[0].ShowID)
	}
	if fake.requests[0].SegmentType != "deep_dive" {
		t.Fatalf("SegmentType = %q", fake.requests[0].SegmentType)
	}
	if fake.requests[0].Topic != "The archaeology of memory" {
		t.Fatalf("Topic = %q", fake.requests[0].Topic)
	}
}

func TestRunGenerate_FallsBackToCurrentShow(t *testing.T) {
	cfg := config{
		SchedulePath:    filepath.Join("..", "..", "config", "schedule.yaml"),
		TalkSegmentsDir: t.TempDir(),
		ScriptsDir:      t.TempDir(),
	}

	fake := &fakeGenerateService{}
	origBuild := buildGeneratorFn
	origNow := nowFunc
	buildGeneratorFn = func(cfg config) (generateService, error) { return fake, nil }
	nowFunc = func() time.Time { return time.Date(2026, 4, 14, 1, 0, 0, 0, time.Local) }
	defer func() {
		buildGeneratorFn = origBuild
		nowFunc = origNow
	}()

	if err := runGenerate(context.Background(), cfg, "", "story", "", 1); err != nil {
		t.Fatalf("runGenerate() error = %v", err)
	}
	if len(fake.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(fake.requests))
	}
	if fake.requests[0].ShowID != "midnight_signal" {
		t.Fatalf("ShowID = %q, want midnight_signal", fake.requests[0].ShowID)
	}
}

func TestRunGenerateAll_FillsShortfall(t *testing.T) {
	dir := t.TempDir()
	showDir := filepath.Join(dir, "midnight_signal")
	if err := os.MkdirAll(showDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for i := range 2 {
		if err := os.WriteFile(filepath.Join(showDir, fmt.Sprintf("s%d.wav", i)), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	cfg := config{
		SchedulePath:    filepath.Join("..", "..", "config", "schedule.yaml"),
		TalkSegmentsDir: dir,
		ScriptsDir:      t.TempDir(),
	}

	fake := &fakeGenerateService{}
	origBuild := buildGeneratorFn
	buildGeneratorFn = func(cfg config) (generateService, error) { return fake, nil }
	defer func() { buildGeneratorFn = origBuild }()

	if err := runGenerateAll(context.Background(), cfg, 3, "show_intro"); err != nil {
		t.Fatalf("runGenerateAll() error = %v", err)
	}
	if len(fake.requests) == 0 {
		t.Fatal("expected at least one generated request")
	}
	if fake.requests[0].SegmentType != "show_intro" {
		t.Fatalf("SegmentType = %q, want show_intro", fake.requests[0].SegmentType)
	}
}

func TestRunStatus_PrintsSegmentCounts(t *testing.T) {
	dir := t.TempDir()
	showDir := filepath.Join(dir, "midnight_signal")
	if err := os.MkdirAll(showDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for i := range 3 {
		if err := os.WriteFile(filepath.Join(showDir, fmt.Sprintf("s%d.wav", i)), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	cfg := config{
		SchedulePath:    filepath.Join("..", "..", "config", "schedule.yaml"),
		TalkSegmentsDir: dir,
	}

	var buf strings.Builder
	if err := runStatus(cfg, &buf); err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "midnight_signal") {
		t.Fatalf("status output missing show: %s", out)
	}
	if !strings.Contains(out, "3") {
		t.Fatalf("status output missing count: %s", out)
	}
}

func TestRunListTypes(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	if err := runListTypes(&buf); err != nil {
		t.Fatalf("runListTypes() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "deep_dive") || !strings.Contains(out, "show_outro") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunListTopics(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	if err := runListTopics(&buf, "philosophy"); err != nil {
		t.Fatalf("runListTopics() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[philosophy]") || !strings.Contains(out, "The archaeology of memory") {
		t.Fatalf("unexpected output: %s", out)
	}
}
