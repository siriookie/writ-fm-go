// Package store provides read-only access to pre-generated audio segment files.
package store

import (
	"encoding/json"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// audioExts is the set of file extensions recognised as bumper audio files.
var audioExts = map[string]bool{
	".flac": true,
	".mp3":  true,
	".wav":  true,
}

// bumperMeta holds the optional JSON sidecar fields for a bumper track.
// The sidecar file has the same base name as the audio file with a .json extension.
type bumperMeta struct {
	Caption     string `json:"caption"`
	DisplayName string `json:"display_name"`
}

// BumperTrack is a pre-generated AI music bumper ready to be decoded and piped
// into the encoder between talk segments.
type BumperTrack struct {
	Path        string  // absolute path to the audio file
	Duration    float64 // seconds; from ffprobe, or 90.0 if probe fails
	Caption     string  // music generation prompt (from JSON sidecar)
	DisplayName string  // short display name, e.g. "Night Drift" (from JSON sidecar)
}

// BumperStore picks bumper tracks from output/music_bumpers/{showID}/.
type BumperStore struct {
	baseDir string // path to output/music_bumpers
}

// NewBumperStore creates a BumperStore that reads from baseDir/{showID}/.
func NewBumperStore(baseDir string) *BumperStore {
	return &BumperStore{baseDir: baseDir}
}

// Pick selects a random bumper track for showID, excluding the track at
// excludePath (pass an empty string to skip exclusion).
//
// Returns nil, nil when:
//   - the show directory does not exist yet
//   - the directory contains no audio files
//   - all candidates are excluded (only one bumper and it was just played)
//
// Duration is obtained via ffprobe; if ffprobe is unavailable or fails,
// Duration falls back to 90.0 seconds — enough for the decoder to apply
// the correct fade-out timing.
func (s *BumperStore) Pick(showID string, excludePath string) (*BumperTrack, error) {
	dir := filepath.Join(s.baseDir, showID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Build the candidate list, applying the exclusion filter.
	var candidates []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !audioExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		abs := filepath.Join(dir, e.Name())
		if abs == excludePath {
			continue
		}
		candidates = append(candidates, abs)
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	path := candidates[rand.Intn(len(candidates))]

	meta := readMeta(path)
	dur := probeDuration(path)

	return &BumperTrack{
		Path:        path,
		Duration:    dur,
		Caption:     meta.Caption,
		DisplayName: meta.DisplayName,
	}, nil
}

// readMeta reads the JSON sidecar for path (same base, .json extension).
// Returns an empty bumperMeta if the file is missing or malformed — metadata
// is optional; its absence must never prevent playback.
func readMeta(path string) bumperMeta {
	jsonPath := path[:len(path)-len(filepath.Ext(path))] + ".json"
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return bumperMeta{}
	}
	var m bumperMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return bumperMeta{}
	}
	return m
}

// probeDuration returns the duration of the audio file in seconds using
// ffprobe. Falls back to 90.0 if ffprobe is not installed, the file is
// unreadable, or the output cannot be parsed.
//
// 90 seconds is a safe default: long enough for the encoder to emit a full
// segment, short enough not to cause excessive fade-out miscalculation.
func probeDuration(path string) float64 {
	out, err := exec.Command(
		"ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 90.0
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || d <= 0 {
		return 90.0
	}
	return d
}
