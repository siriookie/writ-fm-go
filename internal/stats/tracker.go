package stats

import (
	"sync"

	"github.com/writ-fm/go/internal/nowplaying"
)

// Snapshot is the read-only view exposed to API handlers.
type Snapshot struct {
	TracksPlayed         int
	TotalListenersServed int64
}

// Tracker aggregates playout and listener metrics independently from HTTP read
// traffic so API requests do not mutate business state.
type Tracker struct {
	mu             sync.Mutex
	tracksPlayed   int
	lastTrackName  string
	totalListeners int64
}

// NewTracker returns an empty metrics tracker.
func NewTracker() *Tracker {
	return &Tracker{}
}

// Publish records a track transition. It satisfies nowplaying.Sink so it can
// subscribe directly to state updates.
func (t *Tracker) Publish(track nowplaying.Track) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if track.Name != "" && track.Name != t.lastTrackName {
		t.tracksPlayed++
		t.lastTrackName = track.Name
	}
	return nil
}

// RecordListeners samples the current listener count.
func (t *Tracker) RecordListeners(n int) {
	if n <= 0 {
		return
	}

	t.mu.Lock()
	t.totalListeners += int64(n)
	t.mu.Unlock()
}

// Snapshot returns a point-in-time copy of the aggregated metrics.
func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	return Snapshot{
		TracksPlayed:         t.tracksPlayed,
		TotalListenersServed: t.totalListeners,
	}
}
