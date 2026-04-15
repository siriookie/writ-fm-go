package news

import (
	"fmt"
	"strings"
)

// FormatHeadlines renders headlines as markdown-like bullet lines for prompt injection.
func FormatHeadlines(headlines []Headline, maxItems int) string {
	if len(headlines) == 0 {
		return ""
	}
	if maxItems <= 0 || maxItems > len(headlines) {
		maxItems = len(headlines)
	}

	lines := make([]string, 0, maxItems)
	for _, item := range headlines[:maxItems] {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		source := strings.TrimSpace(item.Source)
		if source == "" {
			source = "来源"
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s", source, title))
	}
	return strings.Join(lines, "\n")
}
