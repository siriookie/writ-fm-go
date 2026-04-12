package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---- helpers -------------------------------------------------------------------

// makeFakeBumper creates an empty audio file and an optional JSON sidecar.
// Pass meta=nil to omit the sidecar (test missing-JSON path).
func makeFakeBumper(t *testing.T, dir, name string, meta *bumperMeta) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if meta != nil {
		b, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		base := name[:len(name)-len(filepath.Ext(name))]
		if err := os.WriteFile(filepath.Join(dir, base+".json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func makeBumperDir(t *testing.T, base, showID string) string {
	t.Helper()
	dir := filepath.Join(base, showID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// ---- BumperStore.Pick ----------------------------------------------------------

func TestBumperStore_Pick_ShowNotFound_ReturnsNil(t *testing.T) {
	store := NewBumperStore(t.TempDir())
	track, err := store.Pick("nonexistent_show", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track != nil {
		t.Errorf("expected nil for missing show, got %+v", track)
	}
}

func TestBumperStore_Pick_EmptyDir_ReturnsNil(t *testing.T) {
	base := t.TempDir()
	makeBumperDir(t, base, "show")
	track, err := NewBumperStore(base).Pick("show", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track != nil {
		t.Errorf("expected nil for empty dir, got %+v", track)
	}
}

func TestBumperStore_Pick_ReturnsTrack(t *testing.T) {
	base := t.TempDir()
	dir := makeBumperDir(t, base, "show")
	makeFakeBumper(t, dir, "bumper_20240315_143022.flac", nil)

	track, err := NewBumperStore(base).Pick("show", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track == nil {
		t.Fatal("expected a track, got nil")
	}
}

func TestBumperStore_Pick_PathIsAbsolute(t *testing.T) {
	base := t.TempDir()
	dir := makeBumperDir(t, base, "show")
	makeFakeBumper(t, dir, "bumper_20240315_143022.flac", nil)

	track, _ := NewBumperStore(base).Pick("show", "")
	if !filepath.IsAbs(track.Path) {
		t.Errorf("path %q is not absolute", track.Path)
	}
}

func TestBumperStore_Pick_ReadsMetadata(t *testing.T) {
	base := t.TempDir()
	dir := makeBumperDir(t, base, "show")
	makeFakeBumper(t, dir, "bumper_20240315_143022.flac", &bumperMeta{
		Caption:     "ambient electronic soundscape",
		DisplayName: "Night Drift",
	})

	track, err := NewBumperStore(base).Pick("show", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track.Caption != "ambient electronic soundscape" {
		t.Errorf("Caption = %q, want %q", track.Caption, "ambient electronic soundscape")
	}
	if track.DisplayName != "Night Drift" {
		t.Errorf("DisplayName = %q, want %q", track.DisplayName, "Night Drift")
	}
}

func TestBumperStore_Pick_NoJSONSidecar_NotAnError(t *testing.T) {
	// Missing JSON sidecar is OK — Caption/DisplayName are just empty.
	base := t.TempDir()
	dir := makeBumperDir(t, base, "show")
	makeFakeBumper(t, dir, "bumper_20240315_143022.flac", nil) // no JSON

	track, err := NewBumperStore(base).Pick("show", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track == nil {
		t.Fatal("expected track even without JSON sidecar")
	}
	if track.Caption != "" || track.DisplayName != "" {
		t.Errorf("expected empty metadata without JSON, got Caption=%q DisplayName=%q",
			track.Caption, track.DisplayName)
	}
}

func TestBumperStore_Pick_DurationFallback(t *testing.T) {
	// An empty file is not a valid audio file; ffprobe will fail.
	// Duration must fall back to 90.0.
	base := t.TempDir()
	dir := makeBumperDir(t, base, "show")
	makeFakeBumper(t, dir, "bumper_20240315_143022.flac", nil)

	track, err := NewBumperStore(base).Pick("show", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track.Duration != 90.0 {
		t.Errorf("expected fallback duration 90.0, got %v", track.Duration)
	}
}

func TestBumperStore_Pick_ExcludesLastPlayed(t *testing.T) {
	// With two bumpers and one excluded, must always return the other.
	base := t.TempDir()
	dir := makeBumperDir(t, base, "show")
	makeFakeBumper(t, dir, "bumper_a_20240315.flac", nil)
	makeFakeBumper(t, dir, "bumper_b_20240315.mp3", nil)

	excludePath := filepath.Join(dir, "bumper_a_20240315.flac")
	store := NewBumperStore(base)

	for i := 0; i < 10; i++ { // run several times to catch any random failure
		track, err := store.Pick("show", excludePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if track == nil {
			t.Fatal("expected a track, got nil")
		}
		if track.Path == excludePath {
			t.Errorf("excluded track %q was returned", excludePath)
		}
	}
}

func TestBumperStore_Pick_AllExcluded_ReturnsNil(t *testing.T) {
	// Only one bumper and it is the excluded one → nil (no candidates).
	base := t.TempDir()
	dir := makeBumperDir(t, base, "show")
	makeFakeBumper(t, dir, "bumper_20240315.flac", nil)

	excludePath := filepath.Join(dir, "bumper_20240315.flac")
	track, err := NewBumperStore(base).Pick("show", excludePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track != nil {
		t.Errorf("expected nil when all candidates excluded, got %+v", track)
	}
}

func TestBumperStore_Pick_MultipleFormats(t *testing.T) {
	// .flac, .mp3, .wav are all valid; .json and .txt are not.
	base := t.TempDir()
	dir := makeBumperDir(t, base, "show")
	makeFakeBumper(t, dir, "bumper_a.flac", nil)
	makeFakeBumper(t, dir, "bumper_b.mp3", nil)
	makeFakeBumper(t, dir, "bumper_c.wav", nil)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	store := NewBumperStore(base)
	for i := 0; i < 30; i++ {
		track, err := store.Pick("show", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if track == nil {
			t.Fatal("expected a track")
		}
		seen[filepath.Base(track.Path)] = true
	}

	for _, want := range []string{"bumper_a.flac", "bumper_b.mp3", "bumper_c.wav"} {
		if !seen[want] {
			t.Errorf("format %q never selected in 30 picks", want)
		}
	}
}

func TestBumperStore_Pick_NonAudioIgnored(t *testing.T) {
	base := t.TempDir()
	dir := makeBumperDir(t, base, "show")
	// Only non-audio files.
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	track, err := NewBumperStore(base).Pick("show", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track != nil {
		t.Errorf("expected nil when no audio files, got %+v", track)
	}
}
