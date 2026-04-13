package musicgen

import (
	"math/rand"
	"testing"
)

func TestPickCaption_ReturnsEntryForStyle(t *testing.T) {
	for _, style := range []string{"ambient", "jazz", "downtempo", "soul", "indie"} {
		entry := pickCaption(style, rand.New(rand.NewSource(0)))
		if entry.Caption == "" {
			t.Errorf("style %q: got empty caption", style)
		}
	}
}

func TestPickCaption_UnknownStyleFallsBackToAmbient(t *testing.T) {
	entry := pickCaption("unknown_style", rand.New(rand.NewSource(0)))
	if entry.Caption == "" {
		t.Error("unknown style: expected fallback caption, got empty")
	}
}

func TestPickCaption_DisplayNameDerived(t *testing.T) {
	entry := pickCaption("ambient", rand.New(rand.NewSource(0)))
	if entry.DisplayName == "" {
		t.Error("expected non-empty display name")
	}
}

func TestPickCaption_InstrumentalEntries(t *testing.T) {
	// All entries for styles that only have instrumental should be instrumental.
	for i := 0; i < 20; i++ {
		entry := pickCaption("ambient", rand.New(rand.NewSource(int64(i))))
		if !entry.Instrumental {
			t.Errorf("ambient entry not instrumental: caption=%q", entry.Caption)
		}
	}
}

func TestCaptionPool_AllStylesNonEmpty(t *testing.T) {
	for style, pool := range captionPools {
		if len(pool) == 0 {
			t.Errorf("style %q has empty caption pool", style)
		}
		for i, e := range pool {
			if e.Caption == "" {
				t.Errorf("style %q entry[%d] has empty caption", style, i)
			}
			if e.DisplayName == "" {
				t.Errorf("style %q entry[%d] has empty display_name", style, i)
			}
		}
	}
}
