// Package nowplaying writes the current on-air state to a JSON file atomically.
package nowplaying

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Track is the now-playing state written to disk as JSON.
//
// JSON field names match the Python api_server.py / stream_gapless.py schema so
// that the same now-playing.json can be consumed by the web frontend, OBS browser
// sources, and any tooling already written against the Python version.
type Track struct {
	// Core identification — present for every track.
	ShowID   string `json:"show_id"`
	ShowName string `json:"show"`
	Type     string `json:"type"` // "talk" | "bumper"

	// Display name shown to listeners.
	// For talk: output of CleanName (friendly segment label).
	// For bumpers: BumperTrack.DisplayName, or "AI Music" as fallback.
	Name string `json:"track"`

	// Talk-only fields; omitted from JSON when empty.
	Host        string `json:"host,omitempty"`         // persona / host id
	SegmentType string `json:"segment_type,omitempty"` // e.g. "deep_dive"

	// Bumper-only fields; omitted from JSON when zero/false.
	AIGenerated bool   `json:"ai_generated,omitempty"`
	Caption     string `json:"caption,omitempty"` // AI generation prompt excerpt

	// Runtime state.
	Listeners int       `json:"listeners"`
	UpdatedAt time.Time `json:"timestamp"`
}

// Write atomically writes t as JSON to path.
//
// It writes to a temp file in the same directory, then renames it into place.
// Readers therefore never observe a partial file.
func Write(path string, t Track) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".now-playing-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful os.Rename

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
