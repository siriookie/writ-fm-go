package generator

import "strings"

func deriveTopicFromSourceMaterials(materials string) string {
	materials = strings.TrimSpace(materials)
	if materials == "" {
		return ""
	}
	for _, line := range strings.Split(materials, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimSpace(line)
		if line == "" || line == "inline" {
			continue
		}
		return truncateRunes(line, 120)
	}
	return truncateRunes(strings.Join(strings.Fields(materials), " "), 120)
}

func truncateRunes(text string, max int) string {
	if max <= 0 {
		return text
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
}
