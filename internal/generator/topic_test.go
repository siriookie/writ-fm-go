package generator

import (
	"strings"
	"testing"
)

func TestDeriveNewsAnalysisTopicUsesRSSHeadlines(t *testing.T) {
	t.Parallel()

	topic := deriveNewsAnalysisTopic(strings.Join([]string{
		"1. [BBC] 白宫记者晚宴枪击案：31岁男子被捕、特勤人员中弹",
		"   正文摘要：现场秩序混乱，调查仍在进行。",
		"",
		"2. [Reuters] BYD grows without US market",
		"   正文摘要：A report on global EV competition.",
	}, "\n"))

	for _, want := range []string{"今日 RSS 材料", "白宫记者晚宴枪击案", "BYD grows without US market"} {
		if !strings.Contains(topic, want) {
			t.Fatalf("derived topic missing %q: %s", want, topic)
		}
	}
}

func TestDeriveNewsAnalysisTopicFallsBackToDynamicInstruction(t *testing.T) {
	t.Parallel()

	topic := deriveNewsAnalysisTopic("")
	if !strings.Contains(topic, "根据今日 RSS 材料") || !strings.Contains(topic, "自行判断") {
		t.Fatalf("derived fallback topic should ask model to judge from RSS, got %q", topic)
	}
}
