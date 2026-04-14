package generator

import "testing"

func TestSelectTopic(t *testing.T) {
	t.Parallel()

	got := selectTopicWithRand("philosophy", func(n int) int { return 0 })
	if got != TopicPools["philosophy"][0] {
		t.Fatalf("SelectTopic() = %q, want %q", got, TopicPools["philosophy"][0])
	}
}

func TestSelectTopic_UnknownFocusFallsBack(t *testing.T) {
	t.Parallel()

	if got := selectTopicWithRand("missing", func(n int) int { return 0 }); got == "" {
		t.Fatal("SelectTopic() fallback returned empty string")
	}
}
