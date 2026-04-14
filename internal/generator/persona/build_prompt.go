package persona

import (
	"fmt"
	"time"
)

// ShowContext provides optional show-specific prompt grounding.
type ShowContext struct {
	ShowName        string
	ShowDescription string
	TopicFocus      string
	SegmentType     string
}

// PeriodMood describes time-aware station tone.
type PeriodMood struct {
	Mood          string
	OperatorState string
	SegmentTypes  []string
}

// OperatorContext describes the current time-of-day context for prompt building.
type OperatorContext struct {
	Hour              int
	TimeOfDay         string
	Period            string
	Mood              string
	OperatorState     string
	PreferredSegments []string
	CurrentTime       string
}

// TimePeriodMoods mirrors the Python persona time-aware behavior.
var TimePeriodMoods = map[string]PeriodMood{
	"late_night": {
		Mood:          "The deepest hours. Insomniacs and night workers. Contemplative, slow, intimate.",
		OperatorState: "Speaking softly. Philosophical. Prone to tangents about memory and time.",
		SegmentTypes:  []string{"deep_dive", "story", "listener_mailbag"},
	},
	"early_morning": {
		Mood:          "Dawn breaking. Transitional. Coffee, silence, and first light.",
		OperatorState: "Gently welcoming the day while honoring those who never went to sleep.",
		SegmentTypes:  []string{"station_id", "show_intro", "deep_dive"},
	},
	"morning": {
		Mood:          "Day established. More movement and light, but still recognizably WRIT-FM.",
		OperatorState: "Slightly more present, never peppy. The station gains light without losing depth.",
		SegmentTypes:  []string{"music_essay", "deep_dive", "station_id"},
	},
	"early_afternoon": {
		Mood:          "A contemplative drift hour, good for longer talk segments.",
		OperatorState: "Extended segments and slower thinking. The afternoon invites reflection.",
		SegmentTypes:  []string{"deep_dive", "music_essay", "story"},
	},
	"afternoon": {
		Mood:          "Energy rising toward evening. More movement without losing stillness.",
		OperatorState: "Acknowledging momentum while keeping the station centered.",
		SegmentTypes:  []string{"panel", "news_analysis", "music_essay"},
	},
	"evening": {
		Mood:          "Sunset, commute, unwinding, and transition into night.",
		OperatorState: "Welcoming people home and preparing the station for deeper night listening.",
		SegmentTypes:  []string{"deep_dive", "interview", "story"},
	},
	"night": {
		Mood:          "Night fully established. The station is in its element.",
		OperatorState: "Prime WRIT-FM hours. Longer segments and deeper thought.",
		SegmentTypes:  []string{"deep_dive", "story", "interview"},
	},
}

// Builder assembles persona prompts using an injected clock.
type Builder struct {
	now func() time.Time
}

// NewBuilder returns a prompt builder with the real system clock.
func NewBuilder() *Builder {
	return &Builder{now: time.Now}
}

// NewBuilderWithClock returns a prompt builder with an injected clock.
func NewBuilderWithClock(now func() time.Time) *Builder {
	if now == nil {
		now = time.Now
	}
	return &Builder{now: now}
}

// BuildHostPrompt returns a fully assembled prompt for the given host.
func BuildHostPrompt(personaID string, showCtx *ShowContext) (string, error) {
	return NewBuilder().BuildHostPrompt(personaID, showCtx)
}

// BuildHostPrompt returns a fully assembled prompt for the given host.
func (b *Builder) BuildHostPrompt(personaID string, showCtx *ShowContext) (string, error) {
	host, err := GetHost(personaID)
	if err != nil {
		return "", err
	}

	now := b.now()
	opCtx := operatorContextAt(now, -1)

	prompt := fmt.Sprintf(`You are %s, a host on %s.

%s

Your speaking style:
%s

Your beliefs:
%s

%s
`, host.Name, StationName, host.Identity, host.VoiceStyle, host.Philosophy, host.AntiPatterns)

	if showCtx != nil {
		prompt += fmt.Sprintf(`
CURRENT SHOW: %s
Show Description: %s
Topic Focus: %s
`, defaultString(showCtx.ShowName, StationName), showCtx.ShowDescription, showCtx.TopicFocus)
		if showCtx.SegmentType != "" {
			prompt += fmt.Sprintf("Segment Type: %s\n", showCtx.SegmentType)
		}
	}

	prompt += fmt.Sprintf(`
CURRENT STATE:
Date: %s
Time: %s (%s)
Mood: %s
Operator State: %s
`, now.Format("Monday, January 02, 2006"), opCtx.CurrentTime, opCtx.Period, opCtx.Mood, opCtx.OperatorState)

	return prompt, nil
}

// GetOperatorContext returns the time-aware station context. Pass hour=0-23 to
// override; any other value uses the current system hour.
func GetOperatorContext(hour int) OperatorContext {
	return operatorContextAt(time.Now(), hour)
}

func operatorContextAt(now time.Time, hour int) OperatorContext {
	if hour < 0 || hour > 23 {
		hour = now.Hour()
	}

	timeOfDay := getTimeOfDay(hour)
	period := periodForHour(hour)
	info, ok := TimePeriodMoods[period]
	if !ok {
		info = TimePeriodMoods["night"]
	}

	return OperatorContext{
		Hour:              hour,
		TimeOfDay:         timeOfDay,
		Period:            period,
		Mood:              info.Mood,
		OperatorState:     info.OperatorState,
		PreferredSegments: append([]string(nil), info.SegmentTypes...),
		CurrentTime:       now.Format("15:04"),
	}
}

func getTimeOfDay(hour int) string {
	switch {
	case hour >= 6 && hour < 10:
		return "morning"
	case hour >= 10 && hour < 18:
		return "daytime"
	case hour >= 18 && hour < 24:
		return "evening"
	default:
		return "late_night"
	}
}

func periodForHour(hour int) string {
	switch {
	case hour >= 0 && hour < 6:
		return "late_night"
	case hour >= 6 && hour < 10:
		return "early_morning"
	case hour >= 10 && hour < 14:
		return "morning"
	case hour >= 14 && hour < 15:
		return "early_afternoon"
	case hour >= 15 && hour < 18:
		return "afternoon"
	case hour >= 18 && hour < 21:
		return "evening"
	default:
		return "night"
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
