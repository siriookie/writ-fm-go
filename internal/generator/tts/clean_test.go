package tts

import "testing"

func TestCleanText(t *testing.T) {
	t.Parallel()

	input := "  你好\x00 world\t\n😀  [test]  "
	got := CleanText(input)
	want := "你好 world [test]"
	if got != want {
		t.Fatalf("CleanText() = %q, want %q", got, want)
	}
}

func TestCleanTextWithLimit(t *testing.T) {
	t.Parallel()

	got := CleanTextWithLimit("abcdef", 4)
	if got != "abcd" {
		t.Fatalf("CleanTextWithLimit() = %q, want abcd", got)
	}
}

func TestCleanTextEmpty(t *testing.T) {
	t.Parallel()

	if got := CleanText(" \t\n "); got != "" {
		t.Fatalf("CleanText(empty) = %q, want empty", got)
	}
}
