// Package nowplaying writes the current on-air state to a JSON file atomically.
package nowplaying

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Track is the now-playing state written to disk as JSON.
type Track struct {
	ShowID    string    `json:"show_id"`
	ShowName  string    `json:"show_name"`
	Type      string    `json:"type"` // "talk" or "bumper"
	File      string    `json:"file"` // basename of the audio file
	UpdatedAt time.Time `json:"updated_at"`
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
