// Package scheduler contains the core scheduling logic for WRIT-FM.
// It depends only on the domain package (no external libraries).
package scheduler

import (
	"fmt"
	"time"

	"github.com/writ-fm/go/internal/domain"
)

// ScheduleError is returned for invalid schedule config or failed resolution.
type ScheduleError struct{ msg string }

func (e *ScheduleError) Error() string { return e.msg }

func schedErr(format string, args ...interface{}) error {
	return &ScheduleError{msg: fmt.Sprintf(format, args...)}
}

// validSegmentTypes is now defined in the domain package.

// StationSchedule holds the full parsed schedule ready for resolution.
type StationSchedule struct {
	Shows     map[string]*domain.Show
	Base      []*domain.ScheduleBlock
	Overrides []*domain.ScheduleBlock
}

// blockMatches reports whether the block covers the given time.
//
// Weekday mapping (matches Python's datetime.weekday()):  Mon=0 … Sun=6.
// Go's time.Weekday uses Sun=0, so we shift: (go_weekday + 6) % 7.
func blockMatches(block *domain.ScheduleBlock, now time.Time) bool {
	minute := now.Hour()*60 + now.Minute()
	day := (int(now.Weekday()) + 6) % 7 // Mon=0 … Sun=6

	if block.Days == nil {
		// Base block: day-agnostic.
		if block.EndMinute > block.StartMinute {
			return minute >= block.StartMinute && minute < block.EndMinute
		}
		// Cross-midnight: e.g. 22:00 – 02:00
		return minute >= block.StartMinute || minute < block.EndMinute
	}

	// Override block: day-aware.
	_, dayOK := block.Days[day]

	if block.EndMinute > block.StartMinute {
		// Normal (non-cross-midnight) override.
		return dayOK && minute >= block.StartMinute && minute < block.EndMinute
	}

	// Cross-midnight override: the block belongs to the start-day and
	// continues into the next calendar day.
	prevDay := (day - 1 + 7) % 7
	_, prevDayOK := block.Days[prevDay]
	return (dayOK && minute >= block.StartMinute) ||
		(prevDayOK && minute < block.EndMinute)
}

// expandMinutes splits a time range into at most two [start, end) pairs,
// handling the cross-midnight case (end < start).
func expandMinutes(start, end int) ([][2]int, error) {
	if start == end {
		return nil, schedErr("schedule block start and end cannot be the same")
	}
	if start < 0 || start >= 1440 || end < 0 || end > 1440 {
		return nil, schedErr("schedule block times out of range")
	}
	if end > start {
		return [][2]int{{start, end}}, nil
	}
	// Cross-midnight: split into two ranges.
	return [][2]int{{start, 1440}, {0, end}}, nil
}

// Validate checks:
//   - base covers all 1440 minutes with no gaps or overlaps
//   - all show references (base + overrides) point to existing shows
//   - all segment types are valid
func (s *StationSchedule) Validate() error {
	if len(s.Base) == 0 {
		return schedErr("schedule.base is empty")
	}

	coverage := make([]int, 1440)
	for _, block := range s.Base {
		ranges, err := expandMinutes(block.StartMinute, block.EndMinute)
		if err != nil {
			return err
		}
		for _, r := range ranges {
			for m := r[0]; m < r[1]; m++ {
				coverage[m]++
			}
		}
	}
	for m, c := range coverage {
		if c == 0 {
			return schedErr("schedule.base does not cover the full day (first gap at %02d:%02d)", m/60, m%60)
		}
		if c > 1 {
			return schedErr("schedule.base overlaps itself (first overlap at %02d:%02d)", m/60, m%60)
		}
	}

	for _, block := range append(s.Base, s.Overrides...) {
		if _, ok := s.Shows[block.ShowID]; !ok {
			return schedErr("schedule references unknown show: %q", block.ShowID)
		}
	}

	for _, show := range s.Shows {
		for _, st := range show.SegmentTypes {
			if !domain.ValidSegmentTypes[st] {
				return schedErr("show %s: unknown segment type %q. valid: %v", show.ShowID, st, sortedKeys(domain.ValidSegmentTypes))
			}
		}
	}

	return nil
}

// Resolve returns the ResolvedShow active at the given time.
// Overrides are checked first; base is the fallback.
func (s *StationSchedule) Resolve(now time.Time) (*domain.ResolvedShow, error) {
	for _, block := range s.Overrides {
		if blockMatches(block, now) {
			return toResolved(s.Shows[block.ShowID]), nil
		}
	}
	for _, block := range s.Base {
		if blockMatches(block, now) {
			return toResolved(s.Shows[block.ShowID]), nil
		}
	}
	return nil, schedErr("no matching schedule block for current time (base clock may be invalid)")
}

// toResolved converts a Show to a ResolvedShow with independent copies of slices/maps.
func toResolved(show *domain.Show) *domain.ResolvedShow {
	segs := make([]string, len(show.SegmentTypes))
	copy(segs, show.SegmentTypes)

	voices := make(map[string]string, len(show.Voices))
	for k, v := range show.Voices {
		voices[k] = v
	}

	return &domain.ResolvedShow{
		ShowID:       show.ShowID,
		Name:         show.Name,
		Description:  show.Description,
		Host:         show.Host,
		TopicFocus:   show.TopicFocus,
		SegmentTypes: segs,
		BumperStyle:  show.BumperStyle,
		Voices:       voices,
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
