package persona

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
	"time"
)

//go:embed templates/*.tmpl
var promptTemplateFS embed.FS

var hostPromptTemplate = template.Must(template.ParseFS(promptTemplateFS, "templates/*.tmpl"))

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

type hostPromptData struct {
	HostName         string
	StationName      string
	Identity         string
	VoiceStyle       string
	Philosophy       string
	AntiPatterns     string
	PerformanceNotes string
	HasShowContext   bool
	ShowName         string
	ShowDescription  string
	TopicFocus       string
	SegmentType      string
	Date             string
	CurrentTime      string
	PeriodLabel      string
	Mood             string
	OperatorState    string
}

// TimePeriodMoods mirrors the Python persona time-aware behavior.
var TimePeriodMoods = map[string]PeriodMood{
	"late_night": {
		Mood:          "最深的夜里，失眠的人、夜班的人、独自清醒的人都还在。氛围应当缓慢、亲密，带一点沉思。",
		OperatorState: "说话更轻，更靠近耳边，更像在陪伴。容易自然延伸到记忆、时间和夜晚本身的纹理。",
		SegmentTypes:  []string{"deep_dive", "story", "listener_mailbag"},
	},
	"early_morning": {
		Mood:          "天将亮未亮，处于过渡地带，像咖啡、微光和仍未散去的安静。",
		OperatorState: "要轻柔地迎接新一天，同时照顾那些根本还没睡的人。",
		SegmentTypes:  []string{"station_id", "show_intro", "deep_dive"},
	},
	"morning": {
		Mood:          "白天已经站稳脚跟，空气里有更多流动和亮度，但仍然必须保留 WRIT-FM 的质感。",
		OperatorState: "更在场一些，但绝不鸡血。亮起来，但不能失去深度。",
		SegmentTypes:  []string{"music_essay", "deep_dive", "station_id"},
	},
	"early_afternoon": {
		Mood:          "午后开始松开，适合长一点的谈话，适合慢慢把一个念头展开。",
		OperatorState: "思考速度可以放慢，允许延展、回望和反复咂摸。",
		SegmentTypes:  []string{"deep_dive", "music_essay", "story"},
	},
	"afternoon": {
		Mood:          "临近傍晚，动能开始回升，但不能丢掉安稳的底色。",
		OperatorState: "承认世界正在变快，但频道本身要保持稳。",
		SegmentTypes:  []string{"panel", "news_analysis", "music_essay"},
	},
	"evening": {
		Mood:          "日落、通勤、松弛、回家，以及夜晚即将真正展开前的过渡。",
		OperatorState: "像在把人慢慢接回屋里，也像在为更深的夜做准备。",
		SegmentTypes:  []string{"deep_dive", "interview", "story"},
	},
	"night": {
		Mood:          "夜已经完全落下，频道终于进入自己的主场。",
		OperatorState: "这是 WRIT-FM 最舒展的时段，可以更长、更深、更耐心。",
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
	data := hostPromptData{
		HostName:         host.Name,
		StationName:      StationName,
		Identity:         host.Identity,
		VoiceStyle:       host.VoiceStyle,
		Philosophy:       host.Philosophy,
		AntiPatterns:     host.AntiPatterns,
		PerformanceNotes: host.PerformanceNotes,
		HasShowContext:   showCtx != nil,
		Date:             formatChineseDate(now),
		CurrentTime:      opCtx.CurrentTime,
		PeriodLabel:      localizePeriod(opCtx.Period),
		Mood:             opCtx.Mood,
		OperatorState:    opCtx.OperatorState,
	}
	if showCtx != nil {
		data.ShowName = defaultString(showCtx.ShowName, StationName)
		data.ShowDescription = showCtx.ShowDescription
		data.TopicFocus = showCtx.TopicFocus
		data.SegmentType = showCtx.SegmentType
	}

	var buf bytes.Buffer
	if err := hostPromptTemplate.ExecuteTemplate(&buf, "host_prompt.tmpl", data); err != nil {
		return "", fmt.Errorf("generator/persona: render host prompt: %w", err)
	}
	return buf.String(), nil
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

func formatChineseDate(now time.Time) string {
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	return fmt.Sprintf("%04d年%02d月%02d日 %s", now.Year(), now.Month(), now.Day(), weekdays[now.Weekday()])
}

func localizePeriod(period string) string {
	switch period {
	case "late_night":
		return "深夜"
	case "early_morning":
		return "清晨"
	case "morning":
		return "上午"
	case "early_afternoon":
		return "午后"
	case "afternoon":
		return "傍晚前"
	case "evening":
		return "夜晚前段"
	case "night":
		return "夜晚"
	default:
		return period
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
