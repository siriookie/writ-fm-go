package store

import (
	"os"
	"path/filepath"
	"testing"
)

// ---- helpers -------------------------------------------------------------------

// makeFakeWAV creates an empty file with a .wav extension in dir.
// The caller only needs the filename to exist — content doesn't matter for store tests.
func makeFakeWAV(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeShowDir(t *testing.T, base, showID string) string {
	t.Helper()
	dir := filepath.Join(base, showID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// ---- segmentType (unexported, tested from same package) ------------------------

func TestSegmentType_KnownTypes(t *testing.T) {
	cases := []struct {
		filename string
		want     string
	}{
		{"listener_response_20240315_160215.wav", "listener_response"},
		{"deep_dive_why_vinyl_matters_20240315_143022.wav", "deep_dive"},
		{"news_analysis_headlines_decoded_20240315_150500.wav", "news_analysis"},
		{"listener_mailbag_letters_20240315_152000.wav", "listener_mailbag"},
		{"station_id_20240315_144530.wav", "station_id"},
		{"show_intro_20240315_140000.wav", "show_intro"},
		{"show_outro_20240315_235900.wav", "show_outro"},
		{"music_essay_jazz_roots_20240315_170000.wav", "music_essay"},
	}
	for _, tc := range cases {
		got := segmentType(tc.filename)
		if got != tc.want {
			t.Errorf("segmentType(%q) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

func TestSegmentType_UnknownPrefix(t *testing.T) {
	if got := segmentType("something_else_20240315.wav"); got != "" {
		t.Errorf("expected empty string for unknown type, got %q", got)
	}
}

func TestSegmentType_EmptyString(t *testing.T) {
	if got := segmentType(""); got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
}

// ---- TalkStore.List ------------------------------------------------------------

func TestTalkStore_List_EmptyDir(t *testing.T) {
	base := t.TempDir()
	makeShowDir(t, base, "test_show")

	store := NewTalkStore(base)
	segs, err := store.List("test_show")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segs) != 0 {
		t.Errorf("expected 0 segments, got %d", len(segs))
	}
}

func TestTalkStore_List_ShowNotFound_ReturnsEmpty(t *testing.T) {
	// A show with no generated segments yet — directory may not exist.
	// Store must not return an error; the main loop handles the empty case.
	store := NewTalkStore(t.TempDir())
	segs, err := store.List("nonexistent_show")
	if err != nil {
		t.Fatalf("expected nil error for missing show dir, got %v", err)
	}
	if segs != nil {
		t.Errorf("expected nil slice for missing show, got %v", segs)
	}
}

func TestTalkStore_List_AllFilesIncluded(t *testing.T) {
	base := t.TempDir()
	dir := makeShowDir(t, base, "midnight_signal")
	makeFakeWAV(t, dir, "deep_dive_topic_20240315_143022.wav")
	makeFakeWAV(t, dir, "news_analysis_headlines_20240315_150500.wav")
	makeFakeWAV(t, dir, "station_id_20240315_144530.wav")

	segs, err := NewTalkStore(base).List("midnight_signal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segs) != 3 {
		t.Errorf("expected 3 segments, got %d", len(segs))
	}
}

func TestTalkStore_List_ListenerResponseComesFirst(t *testing.T) {
	base := t.TempDir()
	dir := makeShowDir(t, base, "show")
	makeFakeWAV(t, dir, "deep_dive_topic_20240315_143022.wav")
	makeFakeWAV(t, dir, "listener_response_20240315_160215.wav")
	makeFakeWAV(t, dir, "news_analysis_something_20240315_150500.wav")
	makeFakeWAV(t, dir, "listener_response_20240316_090000.wav")

	segs, err := NewTalkStore(base).List("show")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segs) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(segs))
	}

	// First two must be listener_response.
	for i := 0; i < 2; i++ {
		if segs[i].Type != "listener_response" {
			t.Errorf("segs[%d].Type = %q, want listener_response", i, segs[i].Type)
		}
	}
	// Last two must not be listener_response.
	for i := 2; i < 4; i++ {
		if segs[i].Type == "listener_response" {
			t.Errorf("segs[%d].Type = listener_response but should be in non-priority group", i)
		}
	}
}

func TestTalkStore_List_SegmentTypePopulated(t *testing.T) {
	base := t.TempDir()
	dir := makeShowDir(t, base, "show")
	makeFakeWAV(t, dir, "deep_dive_vinyl_history_20240315_143022.wav")
	makeFakeWAV(t, dir, "listener_response_20240315_160215.wav")

	segs, err := NewTalkStore(base).List("show")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byType := make(map[string]bool)
	for _, s := range segs {
		byType[s.Type] = true
	}
	if !byType["listener_response"] {
		t.Error("expected listener_response type in results")
	}
	if !byType["deep_dive"] {
		t.Error("expected deep_dive type in results")
	}
}

func TestTalkStore_List_PathIsAbsolute(t *testing.T) {
	base := t.TempDir()
	dir := makeShowDir(t, base, "show")
	makeFakeWAV(t, dir, "deep_dive_topic_20240315_143022.wav")

	segs, err := NewTalkStore(base).List("show")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segs) == 0 {
		t.Fatal("expected at least one segment")
	}
	if !filepath.IsAbs(segs[0].Path) {
		t.Errorf("path %q is not absolute", segs[0].Path)
	}
}

func TestTalkStore_List_NonWAVFilesIgnored(t *testing.T) {
	base := t.TempDir()
	dir := makeShowDir(t, base, "show")
	makeFakeWAV(t, dir, "deep_dive_topic_20240315_143022.wav")

	// These should be ignored.
	os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	segs, err := NewTalkStore(base).List("show")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segs) != 1 {
		t.Errorf("expected 1 segment (WAV only), got %d", len(segs))
	}
}

func TestTalkStore_List_OnlyListenerResponse(t *testing.T) {
	base := t.TempDir()
	dir := makeShowDir(t, base, "show")
	makeFakeWAV(t, dir, "listener_response_20240315_160215.wav")
	makeFakeWAV(t, dir, "listener_response_20240316_090000.wav")

	segs, err := NewTalkStore(base).List("show")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segs) != 2 {
		t.Errorf("expected 2 segments, got %d", len(segs))
	}
	for _, s := range segs {
		if s.Type != "listener_response" {
			t.Errorf("unexpected type %q", s.Type)
		}
	}
}

func TestTalkStore_List_UnknownTypeIncluded(t *testing.T) {
	// Files whose names don't match any known segment type are still returned.
	// The store doesn't filter by type — the caller decides what to do.
	base := t.TempDir()
	dir := makeShowDir(t, base, "show")
	makeFakeWAV(t, dir, "unknown_segment_20240315.wav")

	segs, err := NewTalkStore(base).List("show")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(segs) != 1 {
		t.Errorf("expected 1 segment, got %d", len(segs))
	}
	if segs[0].Type != "" {
		t.Errorf("expected empty type for unknown segment, got %q", segs[0].Type)
	}
}
