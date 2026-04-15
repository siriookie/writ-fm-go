package news

import "testing"

func TestFormatHeadlines(t *testing.T) {
	t.Parallel()

	got := FormatHeadlines([]Headline{
		{Title: "第一条", Source: "BBC 中文"},
		{Title: "第二条", Source: ""},
	}, 8)

	want := "- [BBC 中文] 第一条\n- [来源] 第二条"
	if got != want {
		t.Fatalf("FormatHeadlines() = %q, want %q", got, want)
	}
}
