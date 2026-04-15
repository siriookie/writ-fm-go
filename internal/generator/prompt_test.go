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
		Topic:           "这周的标题党没有告诉你的那部分现实",
		Headlines:       "- [新华社] 一条示例标题",
		PerformanceMode: PerformanceModeConstrained,
	})
	if err != nil {
		t.Fatalf("BuildGenerationPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "一条示例标题") {
		t.Fatalf("prompt missing formatted headlines:\n%s", prompt)
	}
	if !strings.Contains(prompt, "硬性长度要求") {
		t.Fatalf("prompt missing length instructions:\n%s", prompt)
	}
	if !strings.Contains(prompt, "输出会直接进入 TTS 播放") {
		t.Fatalf("prompt missing TTS output contract:\n%s", prompt)
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
	if !strings.Contains(prompt, InterviewGuests[0].Name) {
		t.Fatalf("prompt missing guest name:\n%s", prompt)
	}
	if !strings.Contains(prompt, InterviewGuests[0].Context) {
		t.Fatalf("prompt missing guest context:\n%s", prompt)
	}
	if !strings.Contains(prompt, "主持人：") || !strings.Contains(prompt, "嘉宾：") {
		t.Fatalf("prompt missing Chinese dialogue markers:\n%s", prompt)
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
	if !strings.Contains(prompt, "修订要求：") {
		t.Fatalf("prompt missing revision section:\n%s", prompt)
	}
	if !strings.Contains(prompt, "请至少写到 1760 字") {
		t.Fatalf("prompt missing retry instruction:\n%s", prompt)
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
	if !strings.Contains(prompt, "不要使用 Markdown") {
		t.Fatalf("prompt missing markdown prohibition:\n%s", prompt)
	}
	if !strings.Contains(prompt, "只允许输出最终要播出的纯文本中文口播内容") {
		t.Fatalf("prompt missing plain-text contract:\n%s", prompt)
	}
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
