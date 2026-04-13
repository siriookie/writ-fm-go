package nowplaying

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWrite_CreatesFileWithCorrectContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "now-playing.json")

	track := Track{
		ShowID:    "midnight_signal",
		ShowName:  "Midnight Signal",
		Type:      "talk",
		File:      "deep_dive_philosophy_20260413.wav",
		UpdatedAt: time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC),
	}
	if err := Write(path, track); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got Track
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ShowID != track.ShowID {
		t.Errorf("ShowID = %q, want %q", got.ShowID, track.ShowID)
	}
	if got.ShowName != track.ShowName {
		t.Errorf("ShowName = %q, want %q", got.ShowName, track.ShowName)
	}
	if got.Type != track.Type {
		t.Errorf("Type = %q, want %q", got.Type, track.Type)
	}
	if got.File != track.File {
		t.Errorf("File = %q, want %q", got.File, track.File)
	}
}

func TestWrite_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "now-playing.json")

	if err := Write(path, Track{ShowID: "show_a", Type: "talk", UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := Write(path, Track{ShowID: "show_b", Type: "bumper", UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	data, _ := os.ReadFile(path)
	var got Track
	_ = json.Unmarshal(data, &got)
	if got.ShowID != "show_b" {
		t.Errorf("ShowID = %q after overwrite, want show_b", got.ShowID)
	}
}

func TestWrite_NoTempFileLeft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "now-playing.json")

	if err := Write(path, Track{UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWrite_DirMissing_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "now-playing.json")
	err := Write(path, Track{UpdatedAt: time.Now()})
	if err == nil {
		t.Fatal("expected error for missing directory, got nil")
	}
}

func TestWrite_OutputIsValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "now-playing.json")
	if err := Write(path, Track{ShowID: "s", UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}
