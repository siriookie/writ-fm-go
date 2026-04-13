package nowplaying

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestState_GetOnFreshStateReturnsZeroTrack(t *testing.T) {
	s := NewState("")
	got := s.Get()
	if got.ShowID != "" || got.Type != "" {
		t.Errorf("fresh State.Get() = %+v, want zero Track", got)
	}
}

func TestState_UpdateThenGetReturnsLatest(t *testing.T) {
	s := NewState("")
	track := Track{
		ShowID:      "midnight_signal",
		ShowName:    "Midnight Signal",
		Type:        "talk",
		Name:        "Deep Dive",
		Host:        "signal_host",
		SegmentType: "deep_dive",
		Listeners:   2,
		UpdatedAt:   time.Date(2026, 4, 13, 1, 0, 0, 0, time.UTC),
	}

	if err := s.Update(track); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := s.Get()
	if got.ShowID != track.ShowID {
		t.Errorf("ShowID = %q, want %q", got.ShowID, track.ShowID)
	}
	if got.ShowName != track.ShowName {
		t.Errorf("ShowName = %q, want %q", got.ShowName, track.ShowName)
	}
	if got.Type != track.Type {
		t.Errorf("Type = %q, want %q", got.Type, track.Type)
	}
	if got.Name != track.Name {
		t.Errorf("Name = %q, want %q", got.Name, track.Name)
	}
}

func TestState_SecondUpdateOverwritesFirst(t *testing.T) {
	s := NewState("")
	_ = s.Update(Track{ShowID: "show_a", Type: "talk", UpdatedAt: time.Now()})
	_ = s.Update(Track{ShowID: "show_b", Type: "bumper", UpdatedAt: time.Now()})

	got := s.Get()
	if got.ShowID != "show_b" {
		t.Errorf("ShowID = %q after second Update, want show_b", got.ShowID)
	}
}

func TestState_UpdateWritesToDiskWhenPathSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "now-playing.json")
	s := NewState(path)

	track := Track{ShowID: "the_loop", ShowName: "The Loop", Type: "bumper", Name: "Night Drift", UpdatedAt: time.Now()}
	if err := s.Update(track); err != nil {
		t.Fatalf("Update: %v", err)
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
		t.Errorf("disk ShowID = %q, want %q", got.ShowID, track.ShowID)
	}
}

func TestState_UpdateNoPathDoesNotCreateFile(t *testing.T) {
	dir := t.TempDir()
	s := NewState("") // no path
	_ = s.Update(Track{ShowID: "x", UpdatedAt: time.Now()})

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected no files in dir, got %v", entries)
	}
}

func TestState_ConcurrentUpdateAndGet(t *testing.T) {
	s := NewState("")
	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for range workers {
		go func() {
			defer wg.Done()
			_ = s.Update(Track{ShowID: "concurrent", Type: "talk", UpdatedAt: time.Now()})
		}()
		go func() {
			defer wg.Done()
			_ = s.Get()
		}()
	}
	wg.Wait()
	// Race detector catches data races; no assertion needed beyond completion.
}

func TestState_GetReturnsCopyNotPointer(t *testing.T) {
	s := NewState("")
	_ = s.Update(Track{ShowID: "original", UpdatedAt: time.Now()})

	got := s.Get()
	got.ShowID = "mutated" // modifying the copy should not affect State
	if got.ShowID != "mutated" || s.Get().ShowID != "original" {
		t.Errorf("State internal track was mutated via Get() return value")
	}
}
