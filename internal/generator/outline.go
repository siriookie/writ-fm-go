package generator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	GenerationModeSingleShot   = "single_shot"
	GenerationModeOutlineFirst = "outline_first"

	OutlineModeAuto  = "auto"
	OutlineModeOff   = "off"
	OutlineModeForce = "force"
)

var jsonFenceRE = regexp.MustCompile("(?s)^```(?:json)?\\s*(.*?)\\s*```$")
var thinkBlockRE = regexp.MustCompile("(?s)<think>.*?</think>")
var forbiddenTransitionPhrases = []string{
	"接下来",
	"接著",
	"接着",
	"下一步",
	"下一段",
	"下一个",
	"後面會",
	"后面会",
	"後續",
	"后续",
	"我們會",
	"我们会",
	"我們將",
	"我们将",
	"我們先停",
	"我们先停",
	"這一點很關鍵",
	"这一点很关键",
	"這也引出",
	"这也引出",
	"引出",
	"轉向",
	"转向",
}

// Outline is the structured plan used by outline-first generation.
type Outline struct {
	Title             string           `json:"title"`
	Topic             string           `json:"topic"`
	SegmentType       string           `json:"segment_type"`
	OverallGoal       string           `json:"overall_goal"`
	EmotionalCurve    string           `json:"emotional_curve"`
	SelectedItemIndex int              `json:"selected_item_index,omitempty"`
	SelectedItemTitle string           `json:"selected_item_title,omitempty"`
	SelectedItemLink  string           `json:"selected_item_link,omitempty"`
	Segments          []OutlineSegment `json:"segments"`
}

// OutlineSegment is one script/TTS unit in an outline-first generation run.
type OutlineSegment struct {
	Index        int      `json:"index"`
	Title        string   `json:"title"`
	Goal         string   `json:"goal"`
	KeyPoints    []string `json:"key_points"`
	TargetLength int      `json:"target_length"`
	Emotion      string   `json:"emotion"`
	Pacing       string   `json:"pacing"`
	Speaker      string   `json:"speaker,omitempty"`
	Speakers     []string `json:"speakers"`
	Transition   string   `json:"transition"`
	Script       string   `json:"script,omitempty"`
	WordCount    int      `json:"word_count,omitempty"`
}

// ScriptPart is a renderable script segment.
type ScriptPart struct {
	Index    int
	Title    string
	Script   string
	Speakers []string
}

func normalizeOutlineMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", OutlineModeAuto:
		return OutlineModeAuto
	case OutlineModeOff:
		return OutlineModeOff
	case OutlineModeForce:
		return OutlineModeForce
	default:
		return OutlineModeAuto
	}
}

func shouldUseOutlineFirst(mode, segmentType string) bool {
	switch normalizeOutlineMode(mode) {
	case OutlineModeOff:
		return false
	case OutlineModeForce:
		return true
	default:
		switch segmentType {
		case "deep_dive", "music_essay", "story", "news_analysis", "interview", "panel":
			return true
		default:
			return false
		}
	}
}

func parseOutline(raw, segmentType string) (*Outline, error) {
	cleaned := cleanJSONResponse(raw)
	if cleaned == "" {
		return nil, fmt.Errorf("generator/outline: empty outline response")
	}
	var outline Outline
	if err := json.Unmarshal([]byte(cleaned), &outline); err != nil {
		return nil, fmt.Errorf("generator/outline: parse outline JSON: %w", err)
	}
	if err := validateOutline(&outline, segmentType); err != nil {
		return nil, err
	}
	return &outline, nil
}

func cleanJSONResponse(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	text = strings.TrimSpace(thinkBlockRE.ReplaceAllString(text, ""))
	if idx := strings.Index(text, "<think>"); idx >= 0 {
		text = strings.TrimSpace(text[:idx])
	}
	if match := jsonFenceRE.FindStringSubmatch(text); len(match) == 2 {
		text = strings.TrimSpace(match[1])
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end >= start {
		text = text[start : end+1]
	}
	return strings.TrimSpace(text)
}

func validateOutline(outline *Outline, segmentType string) error {
	if outline == nil {
		return fmt.Errorf("generator/outline: nil outline")
	}
	if strings.TrimSpace(outline.Title) == "" {
		return fmt.Errorf("generator/outline: title is required")
	}
	if strings.TrimSpace(outline.Topic) == "" {
		return fmt.Errorf("generator/outline: topic is required")
	}
	if strings.TrimSpace(outline.SegmentType) == "" {
		outline.SegmentType = segmentType
	}
	if outline.SegmentType != segmentType {
		return fmt.Errorf("generator/outline: segment_type %q does not match request %q", outline.SegmentType, segmentType)
	}
	if strings.TrimSpace(outline.OverallGoal) == "" {
		return fmt.Errorf("generator/outline: overall_goal is required")
	}
	if strings.TrimSpace(outline.EmotionalCurve) == "" {
		return fmt.Errorf("generator/outline: emotional_curve is required")
	}
	if segmentType == "news_analysis" && outline.SelectedItemIndex < 1 {
		outline.SelectedItemIndex = 1
	}
	if len(outline.Segments) == 0 {
		return fmt.Errorf("generator/outline: segments are required")
	}
	min, max := outlineSegmentCountRange(segmentType)
	if len(outline.Segments) < min || len(outline.Segments) > max {
		return fmt.Errorf("generator/outline: segment count %d outside allowed range %d-%d for %s", len(outline.Segments), min, max, segmentType)
	}
	for i := range outline.Segments {
		segment := &outline.Segments[i]
		if segment.Index == 0 {
			segment.Index = i + 1
		}
		if segment.Index != i+1 {
			return fmt.Errorf("generator/outline: segment %d has index %d", i+1, segment.Index)
		}
		if strings.TrimSpace(segment.Title) == "" {
			return fmt.Errorf("generator/outline: segment %d title is required", segment.Index)
		}
		if strings.TrimSpace(segment.Goal) == "" {
			return fmt.Errorf("generator/outline: segment %d goal is required", segment.Index)
		}
		if len(segment.KeyPoints) == 0 {
			return fmt.Errorf("generator/outline: segment %d key_points are required", segment.Index)
		}
		if segment.TargetLength <= 0 {
			return fmt.Errorf("generator/outline: segment %d target_length must be positive", segment.Index)
		}
		if strings.TrimSpace(segment.Emotion) == "" {
			return fmt.Errorf("generator/outline: segment %d emotion is required", segment.Index)
		}
		if strings.TrimSpace(segment.Pacing) == "" {
			return fmt.Errorf("generator/outline: segment %d pacing is required", segment.Index)
		}
		if err := validateSegmentTransition(*segment); err != nil {
			return err
		}
		segment.Speakers = normalizeSegmentSpeakers(*segment)
		if len(segment.Speakers) == 0 {
			return fmt.Errorf("generator/outline: segment %d speakers are required", segment.Index)
		}
		for _, speaker := range segment.Speakers {
			if !speakerAllowedForSegment(speaker, segmentType) {
				return fmt.Errorf("generator/outline: segment %d speaker %q is not allowed for %s", segment.Index, speaker, segmentType)
			}
		}
	}
	return nil
}

func validateSegmentTransition(segment OutlineSegment) error {
	transition := strings.TrimSpace(segment.Transition)
	if transition == "" {
		return fmt.Errorf("generator/outline: segment %d transition is required", segment.Index)
	}
	for _, phrase := range forbiddenTransitionPhrases {
		if strings.Contains(transition, phrase) {
			return fmt.Errorf("generator/outline: segment %d transition contains forbidden guiding phrase %q", segment.Index, phrase)
		}
	}
	return nil
}

func normalizeSegmentSpeakers(segment OutlineSegment) []string {
	seen := map[string]bool{}
	var speakers []string
	add := func(value string) {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		speakers = append(speakers, value)
	}
	add(segment.Speaker)
	for _, speaker := range segment.Speakers {
		add(speaker)
	}
	return speakers
}

func outlineSegmentCountRange(segmentType string) (int, int) {
	switch segmentType {
	case "interview", "panel":
		return 4, 8
	case "deep_dive", "music_essay":
		return 4, 7
	case "news_analysis":
		return 3, 6
	case "story":
		return 6, 9
	default:
		return 1, 8
	}
}

func speakerAllowedForSegment(speaker, segmentType string) bool {
	switch segmentType {
	case "interview":
		return speaker == "HOST" || speaker == "HOST_A" || speaker == "GUEST" || speaker == "HOST_B"
	case "panel":
		return speaker == "HOST" || speaker == "HOST_A" || speaker == "HOST_B" || speaker == "GUEST"
	default:
		return speaker == "HOST"
	}
}

func cloneOutline(outline *Outline) *Outline {
	if outline == nil {
		return nil
	}
	copied := *outline
	copied.Segments = make([]OutlineSegment, len(outline.Segments))
	copy(copied.Segments, outline.Segments)
	for i := range copied.Segments {
		copied.Segments[i].KeyPoints = append([]string(nil), outline.Segments[i].KeyPoints...)
		copied.Segments[i].Speakers = append([]string(nil), outline.Segments[i].Speakers...)
	}
	return &copied
}
