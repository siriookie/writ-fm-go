package news

import (
	"strings"
	"testing"
)

func TestFormatHeadlines(t *testing.T) {
	t.Parallel()

	got := FormatHeadlines([]Headline{
		{
			Title:     "第一条",
			Source:    "BBC 中文",
			Summary:   "这是一段包含事实细节的摘要。",
			Link:      "https://example.com/one",
			Published: "Mon, 27 Apr 2026 10:00:00 GMT",
		},
		{Title: "第二条", Source: ""},
	}, 8)

	for _, want := range []string{
		"1. [BBC 中文] 第一条",
		"时间：Mon, 27 Apr 2026 10:00:00 GMT",
		"链接：https://example.com/one",
		"正文摘要：这是一段包含事实细节的摘要。",
		"2. [来源] 第二条",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatHeadlines() missing %q:\n%s", want, got)
		}
	}
}

func TestFormatDetailedMaterialsIncludesBoundedContentAndComments(t *testing.T) {
	t.Parallel()

	got := FormatDetailedMaterials([]Headline{
		{
			Title:     "调查中的新闻事件",
			Source:    "BBC",
			Summary:   "这是一段摘要。",
			Content:   strings.Repeat("完整正文材料", 80),
			Comments:  "读者评论和现场反馈",
			Link:      "https://example.com/full",
			Published: "2026-04-27",
		},
	}, 8, 40)

	for _, want := range []string{
		"1. [BBC] 调查中的新闻事件",
		"摘要：这是一段摘要。",
		"正文材料：",
		"评论/反馈：读者评论和现场反馈",
		"...",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatDetailedMaterials() missing %q:\n%s", want, got)
		}
	}
}
