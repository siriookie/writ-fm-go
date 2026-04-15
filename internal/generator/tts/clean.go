package tts

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const DefaultMaxTextRunes = 500

// CleanText normalizes text before it is sent to a TTS backend.
func CleanText(text string) string {
	return CleanTextWithLimit(text, DefaultMaxTextRunes)
}

// CleanTextWithLimit normalizes text and caps the result to maxRunes runes when maxRunes > 0.
func CleanTextWithLimit(text string, maxRunes int) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}

	var b strings.Builder
	b.Grow(len(text))

	lastSpace := true
	runeCount := 0

	for _, r := range text {
		if isFilteredControl(r) || isEmojiRune(r) {
			continue
		}
		if unicode.IsSpace(r) {
			if !lastSpace && withinLimit(runeCount, maxRunes) {
				b.WriteByte(' ')
				runeCount++
				lastSpace = true
			}
			continue
		}
		if !withinLimit(runeCount, maxRunes) {
			break
		}
		b.WriteRune(r)
		runeCount++
		lastSpace = false
	}

	return strings.TrimSpace(b.String())
}

func withinLimit(count, max int) bool {
	return max <= 0 || count < max
}

func isFilteredControl(r rune) bool {
	if r == '\n' || r == '\r' || r == '\t' {
		return false
	}
	return unicode.IsControl(r)
}

func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F600 && r <= 0x1F64F:
		return true
	case r >= 0x1F300 && r <= 0x1F5FF:
		return true
	case r >= 0x1F680 && r <= 0x1F6FF:
		return true
	case r >= 0x1F1E0 && r <= 0x1F1FF:
		return true
	case r >= 0x2700 && r <= 0x27BF:
		return true
	case r >= 0x1F900 && r <= 0x1F9FF:
		return true
	case r >= 0x1FA70 && r <= 0x1FAFF:
		return true
	case r >= 0x1FAD0 && r <= 0x1FAFF:
		return true
	case r == 0x200D: // zero-width joiner
		return true
	case r == 0xFE0F: // variation selector-16
		return true
	default:
		return false
	}
}
