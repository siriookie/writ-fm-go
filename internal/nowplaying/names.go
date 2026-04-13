package nowplaying

import (
	"path/filepath"
	"regexp"
	"strings"
)

// segmentTypeOrder defines segment type keys in priority order.
// Rules:
//   - "listener_response" is first: must match before shorter "news" which is a
//     substring of many longer type names.
//   - "music_history" is before "story": "history" contains the substring "story",
//     so the more-specific key must be checked first.
//   - "news_analysis" is before "news" for the same reason.
//   - "late_night" is before "night" (not a type, but kept for safety ordering).
var segmentTypeOrder = []string{
	"listener_response",
	"deep_dive",
	"news_analysis",
	"interview",
	"panel",
	"music_history", // must precede "story" ("history" ⊃ "story")
	"story",
	"listener_mailbag",
	"music_essay",
	"station_id",
	"show_intro",
	"show_outro",
	// Legacy types
	"long_talk",
	"monologue",
	"late_night",
	"dedication",
	"weather",
	"news",
	"poetry",
}

// segmentFriendlyName maps segment type keys to listener-facing display names.
var segmentFriendlyName = map[string]string{
	"listener_response": "Listener Mail",
	"deep_dive":         "Deep Dive",
	"news_analysis":     "Signal Report",
	"interview":         "The Interview",
	"panel":             "Crosswire",
	"story":             "Story Hour",
	"listener_mailbag":  "Listener Hours",
	"music_essay":       "Sonic Essay",
	"station_id":        "WRIT-FM",
	"show_intro":        "Show Opening",
	"show_outro":        "Show Closing",
	// Legacy
	"long_talk":     "The Operator Speaks",
	"music_history": "Sonic Archaeology",
	"late_night":    "Late Night Transmission",
	"monologue":     "Midnight Musings",
	"dedication":    "For the Night Owls",
	"weather":       "Conditions Unknown",
	"news":          "Signals from Elsewhere",
	"poetry":        "Verse from the Void",
}

// musicNoisePatterns is the ordered list of regexes applied to music filenames
// to strip YouTube-style garbage suffixes and segment markers.
var musicNoisePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\s*\(Official[^)]*\)`),
	regexp.MustCompile(`(?i)\s*\[Official[^]]*\]`),
	regexp.MustCompile(`(?i)\s*\(Full Album[^)]*\)`),
	regexp.MustCompile(`(?i)\s*\[Full Album[^]]*\]`),
	regexp.MustCompile(`(?i)\s*\(HD\)`),
	regexp.MustCompile(`(?i)\s*\[HD\]`),
	regexp.MustCompile(`(?i)\s*\(Audio\)`),
	regexp.MustCompile(`(?i)\s*\[Audio\]`),
	regexp.MustCompile(`(?i)\s*\(Lyrics\)`),
	regexp.MustCompile(`(?i)\s*\[Lyrics\]`),
	regexp.MustCompile(`(?i)\s*\(Visualizer\)`),
	regexp.MustCompile(`\s*\|.*$`),
	regexp.MustCompile(`\s*⧹.*$`), // U+29F9 BIG REVERSE SOLIDUS
	regexp.MustCompile(`_seg\d+_\d+$`),
}

// ExtractSegmentType returns the segment type key embedded in filename
// (e.g. "deep_dive", "listener_response"). Returns "talk" when no known type
// is found. Matching is case-insensitive and uses the filename only (not the
// directory path).
func ExtractSegmentType(filename string) string {
	lower := strings.ToLower(filepath.Base(filename))
	for _, key := range segmentTypeOrder {
		if strings.Contains(lower, key) {
			return key
		}
	}
	return "talk"
}

// CleanName returns a listener-facing display name for an audio file.
//
// For speech segments (isSpeech = true) it maps the segment type key embedded
// in the filename to a friendly label (e.g. "deep_dive" → "Deep Dive").
// Filenames that don't match any known type return "Transmission".
//
// For music files (isSpeech = false) it strips the file extension and then
// removes common YouTube-style noise suffixes (e.g. "(Official Video)", "[HD]",
// "| channel name", "_seg1_44100").
//
// filename should be the base filename including extension; directory components
// are ignored.
func CleanName(filename string, isSpeech bool) string {
	base := filepath.Base(filename)
	stem := strings.TrimSuffix(base, filepath.Ext(base))

	if isSpeech {
		lower := strings.ToLower(stem)
		for _, key := range segmentTypeOrder {
			if strings.Contains(lower, key) {
				return segmentFriendlyName[key]
			}
		}
		return "Transmission"
	}

	name := stem
	for _, re := range musicNoisePatterns {
		name = re.ReplaceAllString(name, "")
	}
	return strings.TrimSpace(name)
}
