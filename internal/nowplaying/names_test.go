package nowplaying

import "testing"

// ---------------------------------------------------------------------------
// ExtractSegmentType
// ---------------------------------------------------------------------------

func TestExtractSegmentType_KnownTypes(t *testing.T) {
	cases := []struct {
		filename string
		want     string
	}{
		{"listener_response_abc123.wav", "listener_response"},
		{"deep_dive_philosophy_20260413.wav", "deep_dive"},
		{"news_analysis_001.wav", "news_analysis"},
		{"interview_guest_xyz.wav", "interview"},
		{"panel_roundtable.wav", "panel"},
		{"story_night_20260101.wav", "story"},
		{"listener_mailbag_april.wav", "listener_mailbag"},
		{"music_essay_001.wav", "music_essay"},
		{"station_id_jingle.wav", "station_id"},
		{"show_intro_midnight.wav", "show_intro"},
		{"show_outro_midnight.wav", "show_outro"},
		// legacy types
		{"long_talk_20260413.wav", "long_talk"},
		{"monologue_late_night.wav", "monologue"},
		{"late_night_transmission.wav", "late_night"},
		{"music_history_essay.wav", "music_history"},
		{"dedication_for_listeners.wav", "dedication"},
		{"weather_conditions.wav", "weather"},
		{"news_brief.wav", "news"},
		{"poetry_void.wav", "poetry"},
	}
	for _, tc := range cases {
		got := ExtractSegmentType(tc.filename)
		if got != tc.want {
			t.Errorf("ExtractSegmentType(%q) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

func TestExtractSegmentType_ListenerResponsePriority(t *testing.T) {
	// "listener_response" contains "news" as substring but must match the longer key first.
	// (In practice filenames won't mix types, but the priority order must be stable.)
	got := ExtractSegmentType("listener_response_news_special.wav")
	if got != "listener_response" {
		t.Errorf("ExtractSegmentType = %q, want listener_response", got)
	}
}

func TestExtractSegmentType_UnknownFallsBackToTalk(t *testing.T) {
	cases := []string{
		"M500002Kbxm51Te11A.mp3",
		"random_audio_file.wav",
		"",
	}
	for _, name := range cases {
		got := ExtractSegmentType(name)
		if got != "talk" {
			t.Errorf("ExtractSegmentType(%q) = %q, want talk", name, got)
		}
	}
}

func TestExtractSegmentType_CaseInsensitive(t *testing.T) {
	got := ExtractSegmentType("DEEP_DIVE_TOPIC.WAV")
	if got != "deep_dive" {
		t.Errorf("ExtractSegmentType (uppercase) = %q, want deep_dive", got)
	}
}

// ---------------------------------------------------------------------------
// CleanName
// ---------------------------------------------------------------------------

func TestCleanName_SpeechKnownType(t *testing.T) {
	cases := []struct {
		filename string
		want     string
	}{
		{"listener_response_abc.wav", "Listener Mail"},
		{"deep_dive_philosophy.wav", "Deep Dive"},
		{"news_analysis_001.wav", "Signal Report"},
		{"interview_guest.wav", "The Interview"},
		{"panel_roundtable.wav", "Crosswire"},
		{"story_night.wav", "Story Hour"},
		{"listener_mailbag_april.wav", "Listener Hours"},
		{"music_essay_001.wav", "Sonic Essay"},
		{"station_id_jingle.wav", "WRIT-FM"},
		{"show_intro_midnight.wav", "Show Opening"},
		{"show_outro_midnight.wav", "Show Closing"},
		// legacy
		{"long_talk_20260413.wav", "The Operator Speaks"},
		{"music_history_essay.wav", "Sonic Archaeology"},
		{"late_night_transmission.wav", "Late Night Transmission"},
		{"monologue_late_night.wav", "Midnight Musings"},
		{"dedication_for_listeners.wav", "For the Night Owls"},
		{"weather_conditions.wav", "Conditions Unknown"},
		{"news_brief.wav", "Signals from Elsewhere"},
		{"poetry_void.wav", "Verse from the Void"},
	}
	for _, tc := range cases {
		got := CleanName(tc.filename, true)
		if got != tc.want {
			t.Errorf("CleanName(%q, speech) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

func TestCleanName_SpeechUnknownFallsBackToTransmission(t *testing.T) {
	got := CleanName("M500002Kbxm51Te11A.mp3", true)
	if got != "Transmission" {
		t.Errorf("CleanName unknown speech = %q, want Transmission", got)
	}
}

func TestCleanName_MusicStripsOfficialSuffix(t *testing.T) {
	cases := []struct {
		filename string
		want     string
	}{
		{"Oasis - Don't Look Back In Anger (Official Video).flac", "Oasis - Don't Look Back In Anger"},
		{"Radiohead - Creep [Official Audio].mp3", "Radiohead - Creep"},
		{"Pink Floyd - Comfortably Numb (HD).wav", "Pink Floyd - Comfortably Numb"},
		{"Some Song (Audio).flac", "Some Song"},
		{"Track (Lyrics).mp3", "Track"},
		{"Song [HD].wav", "Song"},
		{"Video (Visualizer).mp3", "Video"},
		{"Song (Full Album Version).flac", "Song"},
		{"Track [Full Album Remaster].mp3", "Track"},
	}
	for _, tc := range cases {
		got := CleanName(tc.filename, false)
		if got != tc.want {
			t.Errorf("CleanName(%q, music) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

func TestCleanName_MusicStripsSegSuffix(t *testing.T) {
	got := CleanName("Oasis - Don't Look Back In Anger (Remastered)_seg1_44100.flac", false)
	// _seg\d+_\d+ stripped
	if got != "Oasis - Don't Look Back In Anger (Remastered)" {
		t.Errorf("CleanName seg suffix = %q", got)
	}
}

func TestCleanName_MusicStripsAfterPipe(t *testing.T) {
	got := CleanName("Track Name | Official Music Video.mp3", false)
	if got != "Track Name" {
		t.Errorf("CleanName pipe strip = %q, want 'Track Name'", got)
	}
}

func TestCleanName_MusicNoGarbageSuffix(t *testing.T) {
	// Clean title should pass through unchanged.
	got := CleanName("Massive Attack - Teardrop.flac", false)
	if got != "Massive Attack - Teardrop" {
		t.Errorf("CleanName clean title = %q, want unchanged", got)
	}
}

func TestCleanName_UsesFileStemNotPath(t *testing.T) {
	// Caller passes just the filename (basename); extension must be stripped.
	got := CleanName("Some Track (Official Video).mp3", false)
	if got != "Some Track" {
		t.Errorf("CleanName stem = %q", got)
	}
}

func TestCleanName_SpeechCaseInsensitive(t *testing.T) {
	got := CleanName("DEEP_DIVE_TOPIC.WAV", true)
	if got != "Deep Dive" {
		t.Errorf("CleanName speech case-insensitive = %q, want Deep Dive", got)
	}
}
