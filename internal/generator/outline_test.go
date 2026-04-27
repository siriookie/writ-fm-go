package generator

import (
	"strings"
	"testing"
)

func TestParseOutlineAcceptsFencedJSON(t *testing.T) {
	t.Parallel()

	raw := "```json\n" + validDeepDiveOutlineJSON() + "\n```"
	outline, err := parseOutline(raw, "deep_dive")
	if err != nil {
		t.Fatalf("parseOutline() error = %v", err)
	}
	if outline.Title != "Memory map" {
		t.Fatalf("Title = %q", outline.Title)
	}
	if got := outline.Segments[0].Speakers[0]; got != "HOST" {
		t.Fatalf("speaker = %q, want HOST", got)
	}
}

func TestParseOutlineRejectsInvalidSpeaker(t *testing.T) {
	t.Parallel()

	raw := `{
		"title":"Memory map",
		"topic":"Memory",
		"segment_type":"deep_dive",
		"overall_goal":"Explain.",
		"emotional_curve":"calm",
		"segments":[
			{"index":1,"title":"One","goal":"A","key_points":["a"],"target_length":100,"emotion":"calm","pacing":"slow","speakers":["GUEST"],"transition":"next"},
			{"index":2,"title":"Two","goal":"B","key_points":["b"],"target_length":100,"emotion":"calm","pacing":"slow","speakers":["HOST"],"transition":"next"},
			{"index":3,"title":"Three","goal":"C","key_points":["c"],"target_length":100,"emotion":"calm","pacing":"slow","speakers":["HOST"],"transition":"next"},
			{"index":4,"title":"Four","goal":"D","key_points":["d"],"target_length":100,"emotion":"calm","pacing":"slow","speakers":["HOST"],"transition":"end"}
		]
	}`
	_, err := parseOutline(raw, "deep_dive")
	if err == nil {
		t.Fatal("parseOutline() error = nil, want invalid speaker error")
	}
}

func TestParseOutlineRejectsBadSegmentCount(t *testing.T) {
	t.Parallel()

	raw := `{
		"title":"Memory map",
		"topic":"Memory",
		"segment_type":"deep_dive",
		"overall_goal":"Explain.",
		"emotional_curve":"calm",
		"segments":[
			{"index":1,"title":"One","goal":"A","key_points":["a"],"target_length":100,"emotion":"calm","pacing":"slow","speakers":["HOST"],"transition":"end"}
		]
	}`
	_, err := parseOutline(raw, "deep_dive")
	if err == nil {
		t.Fatal("parseOutline() error = nil, want segment count error")
	}
}

func TestParseOutlineRejectsGuidingTransitionPhrases(t *testing.T) {
	t.Parallel()

	raw := strings.Replace(validDeepDiveOutlineJSON(), `"transition":"next"`, `"transition":"接下来我们看官方如何定性这个案件"`, 1)
	_, err := parseOutline(raw, "deep_dive")
	if err == nil {
		t.Fatal("parseOutline() error = nil, want forbidden transition phrase error")
	}
	if !strings.Contains(err.Error(), "transition contains forbidden guiding phrase") {
		t.Fatalf("parseOutline() error = %v", err)
	}
}

func TestShouldUseOutlineFirst(t *testing.T) {
	t.Parallel()

	if !shouldUseOutlineFirst(OutlineModeAuto, "deep_dive") {
		t.Fatal("auto deep_dive should use outline")
	}
	if shouldUseOutlineFirst(OutlineModeAuto, "station_id") {
		t.Fatal("auto station_id should not use outline")
	}
	if !shouldUseOutlineFirst(OutlineModeForce, "station_id") {
		t.Fatal("force station_id should use outline")
	}
	if shouldUseOutlineFirst(OutlineModeOff, "deep_dive") {
		t.Fatal("off deep_dive should not use outline")
	}
}

func validDeepDiveOutlineJSON() string {
	return `{
		"title":"Memory map",
		"topic":"Memory",
		"segment_type":"deep_dive",
		"overall_goal":"Explain.",
		"emotional_curve":"calm to reflective",
		"segments":[
			{"index":1,"title":"One","goal":"A","key_points":["a"],"target_length":100,"emotion":"calm","pacing":"slow","speakers":["HOST"],"transition":"next"},
			{"index":2,"title":"Two","goal":"B","key_points":["b"],"target_length":100,"emotion":"curious","pacing":"measured","speakers":["HOST"],"transition":"next"},
			{"index":3,"title":"Three","goal":"C","key_points":["c"],"target_length":100,"emotion":"focused","pacing":"steady","speakers":["HOST"],"transition":"next"},
			{"index":4,"title":"Four","goal":"D","key_points":["d"],"target_length":100,"emotion":"warm","pacing":"slow","speakers":["HOST"],"transition":"end"}
		]
	}`
}
