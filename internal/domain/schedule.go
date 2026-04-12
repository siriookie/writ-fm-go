// Package domain contains the core types for WRIT-FM scheduling.
// This package has zero external dependencies.
package domain

// Show is a show definition loaded from schedule.yaml.
type Show struct {
	ShowID       string
	Name         string
	Description  string
	Host         string
	TopicFocus   string
	SegmentTypes []string
	BumperStyle  string
	Voices       map[string]string
}

// ScheduleBlock is a time slot that maps to a show.
// Days is nil for base blocks (applies every day).
// Days is a set of weekday indices (Mon=0 … Sun=6) for override blocks.
type ScheduleBlock struct {
	StartMinute int
	EndMinute   int
	ShowID      string
	Days        map[int]struct{} // nil = every day
}

// ResolvedShow is the result of resolving the schedule at a given point in time.
type ResolvedShow struct {
	ShowID       string
	Name         string
	Description  string
	Host         string
	TopicFocus   string
	SegmentTypes []string
	BumperStyle  string
	Voices       map[string]string
}

// ValidSegmentTypes is the closed set of segment types the system recognises.
// Validation belongs here because it is domain knowledge, not infrastructure.
var ValidSegmentTypes = map[string]bool{
	"deep_dive":         true,
	"news_analysis":     true,
	"interview":         true,
	"panel":             true,
	"story":             true,
	"listener_mailbag":  true,
	"listener_response": true,
	"music_essay":       true,
	"station_id":        true,
	"show_intro":        true,
	"show_outro":        true,
}
