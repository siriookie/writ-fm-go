package stats

import (
	"testing"

	"github.com/writ-fm/go/internal/nowplaying"
)

func TestTrackerPublish_IncrementsOnTrackChange(t *testing.T) {
	tracker := NewTracker()

	_ = tracker.Publish(nowplaying.Track{Name: "Track A"})
	_ = tracker.Publish(nowplaying.Track{Name: "Track A"})
	_ = tracker.Publish(nowplaying.Track{Name: "Track B"})

	snapshot := tracker.Snapshot()
	if snapshot.TracksPlayed != 2 {
		t.Fatalf("TracksPlayed = %d, want 2", snapshot.TracksPlayed)
	}
}

func TestTrackerRecordListeners_IgnoresNonPositiveValues(t *testing.T) {
	tracker := NewTracker()

	tracker.RecordListeners(0)
	tracker.RecordListeners(-1)
	tracker.RecordListeners(3)

	snapshot := tracker.Snapshot()
	if snapshot.TotalListenersServed != 3 {
		t.Fatalf("TotalListenersServed = %d, want 3", snapshot.TotalListenersServed)
	}
}
