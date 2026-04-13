package nowplaying

import "sync"

// State holds the current on-air track in memory behind a read-write mutex.
// It is the single source of truth shared between the streamer goroutine (writer)
// and the API server goroutines (readers).
//
// Create with NewState, update with Update, read with Get.
// All methods are safe for concurrent use.
type State struct {
	mu      sync.RWMutex
	current Track
	path    string // disk path for JSON sync; empty = disk write disabled
}

// NewState returns a State that writes the current track to path on every
// Update call. Pass an empty string to disable disk persistence.
func NewState(path string) *State {
	return &State{path: path}
}

// Update atomically replaces the in-memory track and, if a path was provided
// to NewState, writes it to disk via Write. The caller's Track value is copied
// into the State; subsequent mutations of t do not affect the stored value.
func (s *State) Update(t Track) error {
	s.mu.Lock()
	s.current = t
	s.mu.Unlock()

	if s.path != "" {
		return Write(s.path, t)
	}
	return nil
}

// Get returns a snapshot of the current track. The returned value is a copy;
// mutating it does not affect the State.
func (s *State) Get() Track {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}
