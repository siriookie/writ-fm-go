package generator

import (
	"bytes"
	"embed"
	"fmt"
	"math/rand"
	"text/template"

	"github.com/writ-fm/go/internal/generator/persona"
)

//go:embed templates/*.tmpl
var promptTemplateFS embed.FS

var promptTemplates = template.Must(template.ParseFS(promptTemplateFS, "templates/*.tmpl"))

// ScriptLengthTarget defines the acceptable output length range for a segment type.
// For Chinese generation we use approximate character counts instead of word counts.
type ScriptLengthTarget struct {
	Min int
	Max int
}

// WordTarget is kept as a compatibility alias for older callers.
type WordTarget = ScriptLengthTarget

// SegmentLengthTargets mirrors the intended spoken length for a segment type.
var SegmentLengthTargets = map[string]ScriptLengthTarget{
	"deep_dive":        {Min: 2200, Max: 3600},
	"news_analysis":    {Min: 1800, Max: 3000},
	"interview":        {Min: 2600, Max: 4200},
	"panel":            {Min: 2600, Max: 4200},
	"story":            {Min: 1800, Max: 3200},
	"listener_mailbag": {Min: 1800, Max: 2800},
	"music_essay":      {Min: 2200, Max: 3600},
	"station_id":       {Min: 20, Max: 40},
	"show_intro":       {Min: 120, Max: 220},
	"show_outro":       {Min: 90, Max: 180},
}

// SegmentWordTargets is kept as a compatibility alias for older callers.
var SegmentWordTargets = SegmentLengthTargets

type promptEnvelopeData struct {
	BasePrompt              string
	SegmentType             string
	Topic                   string
	TargetMin               int
	TargetMax               int
	LengthInstructions      string
	OutputInstructions      string
	TaskPrompt              string
	PerformanceInstructions string
}

type taskPromptData struct {
	Headlines string
	GuestName string
}

// BuildRequest is the input for prompt generation.
type BuildRequest struct {
	HostID           string
	SegmentType      string
	Topic            string
	ShowName         string
	ShowDescription  string
	TopicFocus       string
	Headlines        string
	GuestName        string
	GuestContext     string
	PerformanceMode  PerformanceMode
	RetryInstruction string
}

// PromptBuilder assembles generation prompts using injected collaborators.
type PromptBuilder struct {
	buildHostPrompt func(string, *persona.ShowContext) (string, error)
	randIntn        func(int) int
}

// NewPromptBuilder returns a prompt builder with production defaults.
func NewPromptBuilder() *PromptBuilder {
	return NewPromptBuilderWithDeps(nil, nil)
}

// NewPromptBuilderWithDeps returns a prompt builder with injected collaborators.
func NewPromptBuilderWithDeps(
	buildHostPrompt func(string, *persona.ShowContext) (string, error),
	randIntn func(int) int,
) *PromptBuilder {
	if buildHostPrompt == nil {
		buildHostPrompt = persona.BuildHostPrompt
	}
	if randIntn == nil {
		randIntn = rand.Intn
	}

	return &PromptBuilder{
		buildHostPrompt: buildHostPrompt,
		randIntn:        randIntn,
	}
}

// BuildGenerationPrompt assembles the full persona + task prompt.
func BuildGenerationPrompt(req BuildRequest) (string, error) {
	return NewPromptBuilder().Build(req)
}

// Build assembles the full persona + task prompt.
func (b *PromptBuilder) Build(req BuildRequest) (string, error) {
	req.PerformanceMode = NormalizePerformanceMode(req.PerformanceMode)

	showCtx := &persona.ShowContext{
		ShowName:        req.ShowName,
		ShowDescription: req.ShowDescription,
		TopicFocus:      req.TopicFocus,
		SegmentType:     req.SegmentType,
	}
	base, err := b.buildHostPrompt(req.HostID, showCtx)
	if err != nil {
		return "", err
	}

	target, ok := SegmentLengthTargets[req.SegmentType]
	if !ok {
		target = SegmentLengthTargets["deep_dive"]
	}

	taskPrompt, topic, err := b.taskPrompt(req)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := promptTemplates.ExecuteTemplate(&buf, "base_prompt.tmpl", promptEnvelopeData{
		BasePrompt:              base,
		SegmentType:             req.SegmentType,
		Topic:                   topic,
		TargetMin:               target.Min,
		TargetMax:               target.Max,
		LengthInstructions:      lengthPromptInstructions(req.SegmentType, target),
		OutputInstructions:      outputPromptInstructions(),
		TaskPrompt:              taskPrompt,
		PerformanceInstructions: performancePromptInstructions(req.PerformanceMode),
	}); err != nil {
		return "", fmt.Errorf("generator: render prompt envelope: %w", err)
	}

	if req.RetryInstruction != "" {
		buf.WriteString("\n\n修订要求：\n")
		buf.WriteString(req.RetryInstruction)
	}

	return buf.String(), nil
}

func (b *PromptBuilder) taskPrompt(req BuildRequest) (string, string, error) {
	switch req.SegmentType {
	case "news_analysis":
		headlines := req.Headlines
		if headlines == "" {
			headlines = "暂无新闻标题，请讨论“新闻”本身是如何塑造现实感的。"
		}
		prompt, err := renderTaskTemplate("segment_news_analysis.tmpl", taskPromptData{
			Headlines: headlines,
		})
		return prompt, req.Topic, err
	case "interview":
		guestName := req.GuestName
		guestContext := req.GuestContext
		if guestName == "" {
			guest := randomInterviewGuestWithRand(b.randIntn)
			guestName = guest.Name
			guestContext = guest.Context
		}
		prompt, err := renderTaskTemplate("segment_interview.tmpl", taskPromptData{
			GuestName: guestName,
		})
		topic := req.Topic
		if guestContext != "" {
			topic = fmt.Sprintf("%s（嘉宾背景：%s）", topic, guestContext)
		}
		return prompt, topic, err
	case "panel":
		prompt, err := renderTaskTemplate("segment_panel.tmpl", nil)
		return prompt, req.Topic, err
	case "story":
		prompt, err := renderTaskTemplate("segment_story.tmpl", nil)
		return prompt, req.Topic, err
	case "listener_mailbag":
		prompt, err := renderTaskTemplate("segment_listener_mailbag.tmpl", nil)
		return prompt, req.Topic, err
	case "music_essay":
		prompt, err := renderTaskTemplate("segment_music_essay.tmpl", nil)
		return prompt, req.Topic, err
	case "station_id":
		prompt, err := renderTaskTemplate("segment_station_id.tmpl", nil)
		return prompt, req.Topic, err
	case "show_intro":
		prompt, err := renderTaskTemplate("segment_show_intro.tmpl", nil)
		return prompt, req.Topic, err
	case "show_outro":
		prompt, err := renderTaskTemplate("segment_show_outro.tmpl", nil)
		return prompt, req.Topic, err
	default:
		prompt, err := renderTaskTemplate("segment_deep_dive.tmpl", nil)
		return prompt, req.Topic, err
	}
}

func renderTaskTemplate(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := promptTemplates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("generator: render task template %s: %w", name, err)
	}
	return buf.String(), nil
}

func lengthPromptInstructions(segmentType string, target ScriptLengthTarget) string {
	minAcceptable := newQualityGate(target).strictMin

	switch segmentType {
	case "deep_dive", "music_essay":
		return fmt.Sprintf("硬性长度要求：正文至少 %d 字，理想范围 %d-%d 字。请写成 6-8 个自然段，每段都要继续推进，不要在前半段过早收束。", minAcceptable, target.Min, target.Max)
	case "news_analysis":
		return fmt.Sprintf("硬性长度要求：正文至少 %d 字，理想范围 %d-%d 字。请写成 5-7 个自然段，至少覆盖事件表层、结构背景、利益关系、隐藏问题和收束判断。", minAcceptable, target.Min, target.Max)
	case "interview", "panel":
		return fmt.Sprintf("硬性长度要求：正文至少 %d 字，理想范围 %d-%d 字。请让对话展开到至少 10-14 轮，不要只做短问短答。", minAcceptable, target.Min, target.Max)
	case "story":
		return fmt.Sprintf("硬性长度要求：正文至少 %d 字，理想范围 %d-%d 字。请写成 5-7 个自然段，必须包含铺垫、细节、转折和回响，不要只写梗概。", minAcceptable, target.Min, target.Max)
	case "listener_mailbag":
		return fmt.Sprintf("硬性长度要求：正文至少 %d 字，理想范围 %d-%d 字。请先展开来信，再回应，再延展出更深一层的讨论。", minAcceptable, target.Min, target.Max)
	default:
		return fmt.Sprintf("硬性长度要求：正文至少 %d 字，理想范围 %d-%d 字。不要把说明或摘要当成成稿。", minAcceptable, target.Min, target.Max)
	}
}

func outputPromptInstructions() string {
	return "输出会直接进入 TTS 播放。只允许输出最终要播出的纯文本中文口播内容，不要使用 Markdown、标题语法、粗体、斜体、代码块、列表符号、链接、括号注释说明或任何排版标记。不要写“以下是内容”这类前言。"
}
