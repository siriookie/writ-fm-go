package news

import (
	"fmt"
	"strings"
)

// FormatHeadlines renders RSS items with enough context for outline prompt injection.
func FormatHeadlines(headlines []Headline, maxItems int) string {
	if len(headlines) == 0 {
		return ""
	}
	if maxItems <= 0 || maxItems > len(headlines) {
		maxItems = len(headlines)
	}

	blocks := make([]string, 0, maxItems)
	for i, item := range headlines[:maxItems] {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		source := strings.TrimSpace(item.Source)
		if source == "" {
			source = "来源"
		}

		lines := []string{fmt.Sprintf("%d. [%s] %s", i+1, source, title)}
		if published := strings.TrimSpace(item.Published); published != "" {
			lines = append(lines, fmt.Sprintf("   时间：%s", published))
		}
		if link := strings.TrimSpace(item.Link); link != "" {
			lines = append(lines, fmt.Sprintf("   链接：%s", link))
		}
		if summary := truncateForPrompt(item.Summary, 420); summary != "" {
			lines = append(lines, fmt.Sprintf("   正文摘要：%s", summary))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

// FormatDetailedMaterials renders richer source material for script generation.
// It keeps each feed item bounded so long article content cannot dominate the prompt.
func FormatDetailedMaterials(headlines []Headline, maxItems, maxRunesPerItem int) string {
	if len(headlines) == 0 {
		return ""
	}
	if maxItems <= 0 || maxItems > len(headlines) {
		maxItems = len(headlines)
	}
	if maxRunesPerItem <= 0 {
		maxRunesPerItem = 1800
	}

	blocks := make([]string, 0, maxItems)
	for i, item := range headlines[:maxItems] {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		source := strings.TrimSpace(item.Source)
		if source == "" {
			source = "来源"
		}

		lines := []string{fmt.Sprintf("%d. [%s] %s", i+1, source, title)}
		if published := strings.TrimSpace(item.Published); published != "" {
			lines = append(lines, fmt.Sprintf("   时间：%s", published))
		}
		if link := strings.TrimSpace(item.Link); link != "" {
			lines = append(lines, fmt.Sprintf("   链接：%s", link))
		}
		if summary := truncateForPrompt(item.Summary, 500); summary != "" {
			lines = append(lines, fmt.Sprintf("   摘要：%s", summary))
		}
		if content := firstNonEmpty(item.Content, item.Summary); content != "" {
			lines = append(lines, fmt.Sprintf("   正文材料：%s", truncateForPrompt(content, maxRunesPerItem)))
		}
		if comments := truncateForPrompt(item.Comments, 800); comments != "" {
			lines = append(lines, fmt.Sprintf("   评论/反馈：%s", comments))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateForPrompt(text string, maxRunes int) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if text == "" || maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
