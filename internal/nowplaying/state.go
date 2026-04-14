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
	sinks   []Sink
}

// NewState returns a State that writes the current track to path on every
// Update call. Pass an empty string to disable disk persistence.
func NewState(path string) *State {
	if path == "" {
		return NewStateWithSinks()
	}
	return NewStateWithSinks(NewJSONSink(path))
}

// NewStateWithSinks returns a State that fans out each update to every sink.
// Use it when state changes need to be published to multiple outputs such as
// JSON files, metrics trackers, or websocket broadcasters.
func NewStateWithSinks(sinks ...Sink) *State {
	return &State{sinks: append([]Sink(nil), sinks...)}
}

// Update atomically replaces the in-memory track and, if a path was provided
// to NewState, writes it to disk via Write. The caller's Track value is copied
// into the State; subsequent mutations of t do not affect the stored value.
func (s *State) Update(t Track) error {
	s.mu.Lock()
	s.current = t
	s.mu.Unlock()

	for _, sink := range s.sinks {
		if err := sink.Publish(t); err != nil {
			return err
		}
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

// Publish lets State satisfy the same sink-oriented interface used by the
// playout service and auxiliary publishers.
func (s *State) Publish(t Track) error {
	return s.Update(t)
}
