// Package store provides read-only access to pre-generated audio segment files.
package store

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/writ-fm/go/internal/domain"
)

// Segment is a single audio file waiting to be played.
type Segment struct {
	Path string // absolute path to the WAV file
	Type string // segment type, e.g. "listener_response", "deep_dive"; empty if unknown
}

// TalkStore scans the talk_segments directory for a given show and returns
// playable segments ordered with listener_response first, then others
// randomly shuffled.
//
// Files are named: {segment_type}_{topic_slug}_{timestamp}.wav
// (listener_response segments omit topic_slug: listener_response_{timestamp}.wav)
//
// The store only reads — callers are responsible for deleting files after
// playback (consume-queue semantics: os.Remove after Pipe returns).
type TalkStore struct {
	baseDir string // path to output/talk_segments
}

// NewTalkStore creates a TalkStore that scans baseDir/{showID}/*.wav.
func NewTalkStore(baseDir string) *TalkStore {
	return &TalkStore{baseDir: baseDir}
}

// List returns all WAV segments for showID, with listener_response segments
// first and all other segments randomly shuffled after them.
//
// If the show directory does not exist (no segments generated yet), List
// returns nil, nil — the caller is expected to wait and retry.
func (s *TalkStore) List(showID string) ([]Segment, error) {
	dir := filepath.Join(s.baseDir, showID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no segments yet; not an error
		}
		return nil, err
	}

	var priority []Segment // listener_response — play these first
	var others []Segment

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".wav") {
			continue
		}
		seg := Segment{
			Path: filepath.Join(dir, e.Name()),
			Type: segmentType(e.Name()),
		}
		if seg.Type == "listener_response" {
			priority = append(priority, seg)
		} else {
			others = append(others, seg)
		}
	}

	// Shuffle non-priority segments so playback order varies each run.
	rand.Shuffle(len(others), func(i, j int) {
		others[i], others[j] = others[j], others[i]
	})

	return append(priority, others...), nil
}

// segmentType extracts the segment type from a WAV filename by matching the
// leading prefix against all known domain segment types.
// Returns an empty string if no known type is found.
func segmentType(filename string) string {
	for t := range domain.ValidSegmentTypes {
		if strings.HasPrefix(filename, t+"_") {
			return t
		}
	}
	return ""
}
