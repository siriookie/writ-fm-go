package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/writ-fm/go/internal/generator/persona"
)

func TestBuildGenerationPromptGoldenConstrained(t *testing.T) {
	t.Parallel()

	builder := NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time {
			return time.Date(2026, 4, 14, 22, 5, 0, 0, time.Local)
		}).BuildHostPrompt,
		nil,
	)

	prompt, err := builder.Build(BuildRequest{
		HostID:          "liminal_operator",
		SegmentType:     "deep_dive",
		Topic:           "记忆考古学，一首歌如何把被埋住的过去重新挖出来",
		ShowName:        "午夜信号",
		ShowDescription: "深夜里的低照度广播。",
		TopicFocus:      "philosophy",
		PerformanceMode: PerformanceModeConstrained,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	assertGolden(t, filepath.Join("testdata", "deep_dive_prompt.golden"), prompt)
}

func TestBuildGenerationPromptGoldenExpressive(t *testing.T) {
	t.Parallel()

	builder := NewPromptBuilderWithDeps(
		persona.NewBuilderWithClock(func() time.Time {
			return time.Date(2026, 4, 14, 22, 5, 0, 0, time.Local)
		}).BuildHostPrompt,
		nil,
	)

	prompt, err := builder.Build(BuildRequest{
		HostID:          "nyx",
		SegmentType:     "story",
		Topic:           "凌晨三点，一封没有寄出的信",
		ShowName:        "夜潮备忘录",
		ShowDescription: "夜晚与梦之间的慢速故事。",
		TopicFocus:      "memory",
		PerformanceMode: PerformanceModeExpressive,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	assertGolden(t, filepath.Join("testdata", "story_prompt_expressive.golden"), prompt)
}

func TestBuildGenerationPromptNewsAnalysisUsesHeadlines(t *testing.T) {
	t.Parallel()

	prompt, err := NewPromptBuilder().Build(BuildRequest{
		HostID:          "signal",
		SegmentType:     "news_analysis",
		Topic:           "这周的标题没有告诉你的那部分现实",
		Headlines:       "1. [新华社] 一条示例标题\n   正文摘要：这是一段新闻摘要。",
		PerformanceMode: PerformanceModeConstrained,
	})
	if err != nil {
		t.Fatalf("BuildGenerationPrompt() error = %v", err)
	}
	for _, want := range []string{"一条示例标题", "新闻分析口播", "基本故事说清楚", "直接写出来的", "谨慎推断", "硬性长度要求", "输出会直接进入 TTS 播放"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildGenerationPromptInterviewInjectsGuest(t *testing.T) {
	t.Parallel()

	builder := NewPromptBuilderWithDeps(nil, func(n int) int { return 0 })

	prompt, err := builder.Build(BuildRequest{
		HostID:          "dr_resonance",
		SegmentType:     "interview",
		Topic:           "海盗电台与城市记忆",
		PerformanceMode: PerformanceModeConstrained,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, want := range []string{InterviewGuests[0].Name, InterviewGuests[0].Context, "HOST_A:", "HOST_B:", "HOST_A 固定代表主持人", "HOST_B 固定代表被采访人"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildGenerationPromptIncludesRetryInstruction(t *testing.T) {
	t.Parallel()

	prompt, err := NewPromptBuilder().Build(BuildRequest{
		HostID:           "signal",
		SegmentType:      "deep_dive",
		Topic:            "测试主题",
		PerformanceMode:  PerformanceModeConstrained,
		RetryInstruction: "请至少写到 1760 字，不要提前收束。",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, want := range []string{"修订要求：", "请至少写到 1760 字"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildGenerationPromptForbidsMarkdownOutput(t *testing.T) {
	t.Parallel()

	prompt, err := NewPromptBuilder().Build(BuildRequest{
		HostID:          "signal",
		SegmentType:     "deep_dive",
		Topic:           "测试主题",
		PerformanceMode: PerformanceModeConstrained,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, want := range []string{"不要使用 Markdown", "只允许输出最终要播出的纯文本中文口播内容"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildOutlinePromptIncludesStructureContract(t *testing.T) {
	t.Parallel()

	prompt, err := NewPromptBuilder().BuildOutlinePrompt(BuildRequest{
		HostID:          "signal",
		SegmentType:     "news_analysis",
		Topic:           "media frames",
		ShowName:        "Signal Report",
		Headlines:       "1. [Source] Headline\n   正文摘要：summary",
		PerformanceMode: PerformanceModeConstrained,
	})
	if err != nil {
		t.Fatalf("BuildOutlinePrompt() error = %v", err)
	}
	for _, want := range []string{"结构导演", "只允许输出一个合法 JSON 对象", `"emotional_curve"`, `"segments"`, "media frames", "Headline", "正文摘要", "transition 不是“节目转场句”", "不要写“接下来我们", "必须写成中文"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("outline prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildOutlinePromptIncludesSourceMaterials(t *testing.T) {
	t.Parallel()

	prompt, err := NewPromptBuilder().BuildOutlinePrompt(BuildRequest{
		HostID:          "liminal_operator",
		SegmentType:     "story",
		Topic:           "旧影院最后一晚",
		ShowName:        "Midnight Signal",
		SourceMaterials: "# 故事大纲\n人物：旧影院放映员\n事件：城市拆迁前的最后一场放映",
		PerformanceMode: PerformanceModeConstrained,
	})
	if err != nil {
		t.Fatalf("BuildOutlinePrompt() error = %v", err)
	}
	for _, want := range []string{"本期 source materials", "旧影院放映员", "每个 segment 的 goal 和 key_points 都要明确对应 source materials"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("outline prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestStoryLengthTargetSupportsThirtyMinuteMidnightEpisode(t *testing.T) {
	target := SegmentLengthTargets["story"]
	if target.Min < 6000 || target.Max < 7600 {
		t.Fatalf("story target = %+v, want thirty-minute range", target)
	}
	minSegments, maxSegments := outlineSegmentCountRange("story")
	if minSegments != 6 || maxSegments != 9 {
		t.Fatalf("story outline segments = %d-%d, want 6-9", minSegments, maxSegments)
	}
}

func TestBuildSegmentScriptPromptIncludesOutlineAndSpeakerContract(t *testing.T) {
	t.Parallel()

	outline := mustTestOutline(t, "interview")
	prompt, err := NewPromptBuilder().BuildSegmentScriptPrompt(BuildRequest{
		HostID:          "dr_resonance",
		SegmentType:     "interview",
		Topic:           "radio memory",
		ShowName:        "Midnight Signal",
		PerformanceMode: PerformanceModeConstrained,
	}, outline, outline.Segments[0], "", false)
	if err != nil {
		t.Fatalf("BuildSegmentScriptPrompt() error = %v", err)
	}
	for _, want := range []string{"完整节目 outline", "当前段落要求", "speaker label", "HOST:", "GUEST:", "必须使用中文口播正文", "不要输出英文段落"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("segment prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildSegmentScriptPromptIncludesSourceMaterials(t *testing.T) {
	t.Parallel()

	outline := mustTestOutline(t, "story")
	prompt, err := NewPromptBuilder().BuildSegmentScriptPrompt(BuildRequest{
		HostID:          "nyx",
		SegmentType:     "story",
		Topic:           "旧影院最后一晚",
		ShowName:        "The Night Garden",
		SourceMaterials: "人物：旧影院放映员\n细节：银幕背后有一封没寄出的信",
		PerformanceMode: PerformanceModeExpressive,
	}, outline, outline.Segments[0], "", false)
	if err != nil {
		t.Fatalf("BuildSegmentScriptPrompt() error = %v", err)
	}
	for _, want := range []string{"本期 source materials", "旧影院放映员", "银幕背后有一封没寄出的信"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("segment prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildSegmentScriptPromptUsesHostPersonaWithoutSecondIdentity(t *testing.T) {
	t.Parallel()

	outline := mustTestOutline(t, "news_analysis")
	prompt, err := NewPromptBuilder().BuildSegmentScriptPrompt(BuildRequest{
		HostID:          "signal",
		SegmentType:     "news_analysis",
		Topic:           "RSS briefing",
		ShowName:        "Signal Report",
		PerformanceMode: PerformanceModeExpressive,
	}, outline, outline.Segments[0], "", false)
	if err != nil {
		t.Fatalf("BuildSegmentScriptPrompt() error = %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(prompt), "你是 信号") {
		t.Fatalf("segment prompt should start with host persona, got:\n%s", prompt)
	}
	for _, want := range []string{"本段写作任务", "段落：第 1 / 3 段", "当前段落要求"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("segment prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"你是播客分段编剧", "<host_style_reference>", "不是你的身份指令"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("segment prompt should not contain second identity marker %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSegmentScriptPromptCarriesTranscriptAndFinalState(t *testing.T) {
	t.Parallel()

	outline := mustTestOutline(t, "news_analysis")
	prompt, err := NewPromptBuilder().BuildSegmentScriptPrompt(BuildRequest{
		HostID:          "signal",
		SegmentType:     "news_analysis",
		Topic:           "RSS briefing",
		ShowName:        "Signal Report",
		PerformanceMode: PerformanceModeExpressive,
	}, outline, outline.Segments[2], "第一段已经建立新闻框架。\n\n第二段已经指出结构压力。", true)
	if err != nil {
		t.Fatalf("BuildSegmentScriptPrompt() error = %v", err)
	}
	for _, want := range []string{"此前已播 transcript", "第二段已经指出结构压力", "这是最后一段", "自然收束"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("segment prompt missing %q:\n%s", want, prompt)
		}
	}
}

func mustTestOutline(t *testing.T, segmentType string) *Outline {
	t.Helper()

	speakers := `["HOST"]`
	extraSegment := ""
	if segmentType == "interview" {
		speakers = `["HOST","GUEST"]`
		extraSegment = `,
			{"index":4,"title":"Return","goal":"Return to the larger theme","key_points":["return"],"target_length":500,"emotion":"warm","pacing":"slow","speakers":` + speakers + `,"transition":"end"}`
	}
	if segmentType == "story" {
		extraSegment = `,
			{"index":4,"title":"Turn","goal":"Deepen the event","key_points":["turn"],"target_length":900,"emotion":"curious","pacing":"measured","speakers":` + speakers + `,"transition":"detail"},
			{"index":5,"title":"Echo","goal":"Bring back the earlier image","key_points":["echo"],"target_length":900,"emotion":"warm","pacing":"slow","speakers":` + speakers + `,"transition":"image"},
			{"index":6,"title":"Return","goal":"Close with resonance","key_points":["return"],"target_length":900,"emotion":"resolved","pacing":"slow","speakers":` + speakers + `,"transition":"end"}`
	}
	outline, err := parseOutline(`{
		"title":"Signal map",
		"topic":"RSS briefing",
		"segment_type":"`+segmentType+`",
		"overall_goal":"Explain the hidden structure.",
		"emotional_curve":"calm to conclusive",
		"segments":[
			{"index":1,"title":"Open","goal":"Set the frame","key_points":["frame"],"target_length":400,"emotion":"calm","pacing":"measured","speakers":`+speakers+`,"transition":"next"},
			{"index":2,"title":"Middle","goal":"Name the pressure","key_points":["pressure"],"target_length":500,"emotion":"focused","pacing":"steady","speakers":`+speakers+`,"transition":"next"},
			{"index":3,"title":"Close","goal":"Land the argument","key_points":["judgment"],"target_length":500,"emotion":"resolved","pacing":"slow","speakers":`+speakers+`,"transition":"end"}`+extraSegment+`
		]
	}`, segmentType)
	if err != nil {
		t.Fatalf("parseOutline() error = %v", err)
	}
	return outline
}

func assertGolden(t *testing.T, path string, got string) {
	t.Helper()

	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	normalize := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
	}

	want := normalize(string(wantBytes))
	got = normalize(got)
	if got != want {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}
