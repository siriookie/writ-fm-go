package scheduler

import (
	"testing"
	"time"

	"github.com/writ-fm/go/internal/domain"
)

// ---- helpers ----------------------------------------------------------------

func makeTime(weekday time.Weekday, hour, min int) time.Time {
	// Find the next occurrence of the given weekday from a fixed epoch Monday.
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local) // 2024-01-01 is Monday
	daysAhead := (int(weekday) - int(base.Weekday()) + 7) % 7
	d := base.AddDate(0, 0, daysAhead)
	return time.Date(d.Year(), d.Month(), d.Day(), hour, min, 0, 0, time.Local)
}

func daySet(days ...int) map[int]struct{} {
	m := make(map[int]struct{}, len(days))
	for _, d := range days {
		m[d] = struct{}{}
	}
	return m
}

func simpleShow(id string) *domain.Show {
	return &domain.Show{
		ShowID:       id,
		Name:         id,
		Description:  id,
		Host:         "liminal_operator",
		SegmentTypes: []string{"deep_dive"},
		BumperStyle:  "ambient",
		Voices:       map[string]string{"host": "am_michael"},
	}
}

func fullDayStation() *StationSchedule {
	// Build a station schedule covering 24 hours with two shows.
	return &StationSchedule{
		Shows: map[string]*domain.Show{
			"night": simpleShow("night"),
			"day":   simpleShow("day"),
		},
		Base: []*domain.ScheduleBlock{
			{StartMinute: 0, EndMinute: 720, ShowID: "night"},   // 00:00 – 12:00
			{StartMinute: 720, EndMinute: 1440, ShowID: "day"},  // 12:00 – 00:00 (midnight)
		},
		Overrides: nil,
	}
}

// ---- blockMatches -----------------------------------------------------------

func TestBlockMatches_BaseNormalSlot(t *testing.T) {
	block := &domain.ScheduleBlock{StartMinute: 0, EndMinute: 720, ShowID: "night"}

	tests := []struct {
		name    string
		t       time.Time
		want    bool
	}{
		{"at start", makeTime(time.Monday, 0, 0), true},
		{"mid block", makeTime(time.Monday, 6, 0), true},
		{"one before end", makeTime(time.Monday, 11, 59), true},
		{"at end (exclusive)", makeTime(time.Monday, 12, 0), false},
		{"after block", makeTime(time.Monday, 14, 0), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := blockMatches(block, tc.t); got != tc.want {
				t.Errorf("blockMatches(%v) = %v, want %v", tc.t.Format("15:04"), got, tc.want)
			}
		})
	}
}

func TestBlockMatches_BaseCrossMidnight(t *testing.T) {
	// 22:00 – 02:00 (cross-midnight base block, day-agnostic)
	block := &domain.ScheduleBlock{StartMinute: 22 * 60, EndMinute: 2 * 60, ShowID: "late"}

	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"at 22:00", makeTime(time.Monday, 22, 0), true},
		{"at 23:30", makeTime(time.Monday, 23, 30), true},
		{"at 00:00", makeTime(time.Tuesday, 0, 0), true},
		{"at 01:59", makeTime(time.Tuesday, 1, 59), true},
		{"at 02:00 (exclusive)", makeTime(time.Tuesday, 2, 0), false},
		{"at 12:00", makeTime(time.Monday, 12, 0), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := blockMatches(block, tc.t); got != tc.want {
				t.Errorf("blockMatches(%v) = %v, want %v", tc.t.Format("15:04"), got, tc.want)
			}
		})
	}
}

func TestBlockMatches_OverrideMatchesDay(t *testing.T) {
	// Sunday-only 18:00 – 20:00 override
	block := &domain.ScheduleBlock{
		StartMinute: 18 * 60,
		EndMinute:   20 * 60,
		ShowID:      "listener_hours",
		Days:        daySet(6), // Sunday
	}

	if !blockMatches(block, makeTime(time.Sunday, 19, 0)) {
		t.Error("expected match on Sunday 19:00")
	}
	if blockMatches(block, makeTime(time.Saturday, 19, 0)) {
		t.Error("expected no match on Saturday 19:00")
	}
	if blockMatches(block, makeTime(time.Sunday, 20, 0)) {
		t.Error("expected no match at end time (exclusive)")
	}
}

func TestBlockMatches_OverrideCrossMidnight(t *testing.T) {
	// Friday 23:00 – 02:00 cross-midnight override
	block := &domain.ScheduleBlock{
		StartMinute: 23 * 60,
		EndMinute:   2 * 60,
		ShowID:      "special",
		Days:        daySet(4), // Friday
	}

	// Friday night (same day)
	if !blockMatches(block, makeTime(time.Friday, 23, 30)) {
		t.Error("expected match Friday 23:30")
	}
	// Saturday early morning (next day, previous day was Friday)
	if !blockMatches(block, makeTime(time.Saturday, 1, 0)) {
		t.Error("expected match Saturday 01:00 (cross-midnight from Friday)")
	}
	// Saturday after end
	if blockMatches(block, makeTime(time.Saturday, 2, 0)) {
		t.Error("expected no match Saturday 02:00 (exclusive end)")
	}
	// Thursday – should not match
	if blockMatches(block, makeTime(time.Thursday, 23, 30)) {
		t.Error("expected no match Thursday 23:30")
	}
}

// ---- Validate ---------------------------------------------------------------

func TestValidate_OK(t *testing.T) {
	s := fullDayStation()
	if err := s.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_EmptyBase(t *testing.T) {
	s := &StationSchedule{
		Shows: map[string]*domain.Show{"a": simpleShow("a")},
		Base:  nil,
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for empty base")
	}
}

func TestValidate_GapInBase(t *testing.T) {
	s := &StationSchedule{
		Shows: map[string]*domain.Show{"a": simpleShow("a")},
		Base: []*domain.ScheduleBlock{
			{StartMinute: 0, EndMinute: 720, ShowID: "a"},
			// gap: 720 – 1080 missing
			{StartMinute: 1080, EndMinute: 1440, ShowID: "a"},
		},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for gap in base")
	}
}

func TestValidate_OverlapInBase(t *testing.T) {
	s := &StationSchedule{
		Shows: map[string]*domain.Show{"a": simpleShow("a")},
		Base: []*domain.ScheduleBlock{
			{StartMinute: 0, EndMinute: 800, ShowID: "a"},
			{StartMinute: 720, EndMinute: 1440, ShowID: "a"},
		},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for overlap in base")
	}
}

func TestValidate_UnknownShowRef(t *testing.T) {
	s := &StationSchedule{
		Shows: map[string]*domain.Show{"a": simpleShow("a")},
		Base: []*domain.ScheduleBlock{
			{StartMinute: 0, EndMinute: 1440, ShowID: "unknown"},
		},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for unknown show reference")
	}
}

func TestValidate_InvalidSegmentType(t *testing.T) {
	show := simpleShow("a")
	show.SegmentTypes = []string{"not_a_real_type"}
	s := &StationSchedule{
		Shows: map[string]*domain.Show{"a": show},
		Base: []*domain.ScheduleBlock{
			{StartMinute: 0, EndMinute: 1440, ShowID: "a"},
		},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for invalid segment type")
	}
}

// ---- Resolve ----------------------------------------------------------------

func TestResolve_BaseSlot(t *testing.T) {
	s := fullDayStation()
	resolved, err := s.Resolve(makeTime(time.Monday, 6, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.ShowID != "night" {
		t.Errorf("got show %q, want %q", resolved.ShowID, "night")
	}
}

func TestResolve_SecondBaseSlot(t *testing.T) {
	s := fullDayStation()
	resolved, err := s.Resolve(makeTime(time.Monday, 14, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.ShowID != "day" {
		t.Errorf("got show %q, want %q", resolved.ShowID, "day")
	}
}

func TestResolve_OverrideTakesPriority(t *testing.T) {
	s := fullDayStation()
	// Sunday 14:00 would normally be "day", but override says "night".
	s.Overrides = []*domain.ScheduleBlock{
		{StartMinute: 720, EndMinute: 1440, ShowID: "night", Days: daySet(6)},
	}

	resolved, err := s.Resolve(makeTime(time.Sunday, 14, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.ShowID != "night" {
		t.Errorf("override not applied: got %q, want %q", resolved.ShowID, "night")
	}
}

func TestResolve_OverrideDoesNotApplyOtherDay(t *testing.T) {
	s := fullDayStation()
	s.Overrides = []*domain.ScheduleBlock{
		{StartMinute: 720, EndMinute: 1440, ShowID: "night", Days: daySet(6)}, // Sunday only
	}

	// Monday at 14:00 should still use base ("day").
	resolved, err := s.Resolve(makeTime(time.Monday, 14, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.ShowID != "day" {
		t.Errorf("override wrongly applied: got %q, want %q", resolved.ShowID, "day")
	}
}

func TestResolve_ResolvedShowFieldsPopulated(t *testing.T) {
	s := fullDayStation()
	resolved, err := s.Resolve(makeTime(time.Monday, 1, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Host == "" {
		t.Error("resolved.Host is empty")
	}
	if len(resolved.SegmentTypes) == 0 {
		t.Error("resolved.SegmentTypes is empty")
	}
	if resolved.Voices == nil {
		t.Error("resolved.Voices is nil")
	}
}

func TestResolve_MutatingReturnedSliceDoesNotAffectSchedule(t *testing.T) {
	s := fullDayStation()
	r1, _ := s.Resolve(makeTime(time.Monday, 1, 0))
	r1.SegmentTypes[0] = "mutated"

	r2, _ := s.Resolve(makeTime(time.Monday, 1, 0))
	if r2.SegmentTypes[0] == "mutated" {
		t.Error("resolve returned shared slice; mutation leaked")
	}
}

// ---- ScheduleError ---------------------------------------------------------

func TestScheduleError_Error(t *testing.T) {
	err := schedErr("test %s", "message")
	if err.Error() != "test message" {
		t.Errorf("unexpected error string: %q", err.Error())
	}
}

// ---- expandMinutes edge cases ----------------------------------------------

func TestExpandMinutes_SameStartEnd(t *testing.T) {
	_, err := expandMinutes(60, 60)
	if err == nil {
		t.Fatal("expected error for same start/end")
	}
}

func TestExpandMinutes_OutOfRange(t *testing.T) {
	_, err := expandMinutes(0, 1441)
	if err == nil {
		t.Fatal("expected error for out-of-range end")
	}
}
