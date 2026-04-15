package performance

import (
	"fmt"
	"strings"
)

type backendTextMappings struct {
	actions  map[Token]string
	styles   map[Token]string
	allowRaw bool
}

func renderTextual(script Script, mappings backendTextMappings) string {
	var builder strings.Builder
	var pending []Cue

	flushText := func(text string) {
		if len(pending) > 0 {
			for _, cue := range pending {
				switch {
				case cue.Mapped && cue.Token != "":
					if action := mappings.actions[cue.Token]; action != "" {
						builder.WriteString(action)
					} else if style := mappings.styles[cue.Token]; style != "" {
						builder.WriteString(style)
					}
				case mappings.allowRaw && cue.Raw != "":
					builder.WriteString(cue.Raw)
				}
			}
			pending = nil
		}
		builder.WriteString(text)
	}

	for _, element := range script.Elements {
		if element.Cue != nil {
			pending = append(pending, *element.Cue)
			continue
		}
		if element.Text != "" {
			flushText(element.Text)
		}
	}

	if len(pending) > 0 {
		for _, cue := range pending {
			switch {
			case cue.Mapped && cue.Token != "":
				if action := mappings.actions[cue.Token]; action != "" {
					builder.WriteString(action)
				} else if style := mappings.styles[cue.Token]; style != "" {
					builder.WriteString(style)
				}
			case mappings.allowRaw && cue.Raw != "":
				builder.WriteString(cue.Raw)
			}
		}
	}

	return strings.TrimSpace(builder.String())
}

func renderMimo(script Script) string {
	var builder strings.Builder
	var pending []Cue
	emittedGlobalStyle := false

	flushText := func(text string) {
		if strings.TrimSpace(text) == "" {
			return
		}

		styles := collectMapped(pending, mimoStyleValue)
		actions, raw := collectActions(pending, mimoActionValue, script.Mode == ModeExpressive)

		if len(styles) > 0 {
			if !emittedGlobalStyle && builder.Len() == 0 {
				builder.WriteString("<style>")
				builder.WriteString(strings.Join(styles, " "))
				builder.WriteString("</style>")
				emittedGlobalStyle = true
			} else {
				for _, style := range styles {
					builder.WriteString("（")
					builder.WriteString(style)
					builder.WriteString("）")
				}
			}
		}

		for _, action := range actions {
			builder.WriteString(action)
		}
		for _, value := range raw {
			builder.WriteString(value)
		}
		pending = nil
		builder.WriteString(text)
	}

	for _, element := range script.Elements {
		if element.Cue != nil {
			pending = append(pending, *element.Cue)
			continue
		}
		if element.Text != "" {
			flushText(element.Text)
		}
	}

	if len(pending) > 0 {
		styles := collectMapped(pending, mimoStyleValue)
		actions, raw := collectActions(pending, mimoActionValue, script.Mode == ModeExpressive)
		if len(styles) > 0 && (!emittedGlobalStyle || builder.Len() > 0) {
			for _, style := range styles {
				builder.WriteString("（")
				builder.WriteString(style)
				builder.WriteString("）")
			}
		}
		for _, action := range actions {
			builder.WriteString(action)
		}
		for _, value := range raw {
			builder.WriteString(value)
		}
	}

	return strings.TrimSpace(builder.String())
}

func renderMicrosoft(script Script) string {
	var builder strings.Builder
	var pending []Cue

	flushText := func(text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		builder.WriteString(renderMicrosoftText(text, pending))
		pending = nil
	}

	for _, element := range script.Elements {
		if element.Cue != nil {
			pending = append(pending, *element.Cue)
			continue
		}
		if element.Text != "" {
			flushText(element.Text)
		}
	}

	if len(pending) > 0 {
		builder.WriteString(renderMicrosoftActions(pending))
	}

	return strings.TrimSpace(builder.String())
}

func collectMapped(cues []Cue, mapper func(Token) string) []string {
	values := make([]string, 0, len(cues))
	seen := make(map[string]struct{}, len(cues))
	for _, cue := range cues {
		if !cue.Mapped || cue.Token == "" {
			continue
		}
		value := mapper(cue.Token)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func collectActions(cues []Cue, mapper func(Token) string, allowRaw bool) (mapped []string, raw []string) {
	for _, cue := range cues {
		switch {
		case cue.Mapped && cue.Token != "":
			if value := mapper(cue.Token); value != "" {
				mapped = append(mapped, value)
			}
		case allowRaw && cue.Raw != "":
			raw = append(raw, cue.Raw)
		}
	}
	return mapped, raw
}

func mimoStyleValue(token Token) string {
	switch token {
	case TokenWarm:
		return "温暖"
	case TokenCalm:
		return "平静"
	case TokenSerious:
		return "认真"
	case TokenHappy:
		return "开心"
	case TokenSad:
		return "悲伤"
	case TokenTense:
		return "紧张"
	case TokenSoft:
		return "放轻声音"
	case TokenWhisper:
		return "悄悄话"
	case TokenSlow:
		return "变慢"
	case TokenFast:
		return "变快"
	case TokenMeasured:
		return "沉稳"
	default:
		return ""
	}
}

func mimoActionValue(token Token) string {
	switch token {
	case TokenPause:
		return "……"
	case TokenBreath:
		return "（深呼吸）"
	case TokenLaugh:
		return "（轻笑）"
	case TokenCough:
		return "（轻咳）"
	default:
		return ""
	}
}

func renderMicrosoftText(text string, cues []Cue) string {
	var builder strings.Builder
	builder.WriteString(renderMicrosoftActions(cues))

	escaped := escapeXMLText(text)
	if escaped == "" {
		return builder.String()
	}

	style := microsoftStyle(cues)
	rate, volume := microsoftProsody(cues)

	if style != "" {
		builder.WriteString(fmt.Sprintf(`<mstts:express-as style="%s">`, style))
	}
	if rate != "" || volume != "" {
		builder.WriteString(`<prosody`)
		if rate != "" {
			builder.WriteString(fmt.Sprintf(` rate="%s"`, rate))
		}
		if volume != "" {
			builder.WriteString(fmt.Sprintf(` volume="%s"`, volume))
		}
		builder.WriteString(`>`)
	}

	builder.WriteString(escaped)

	if rate != "" || volume != "" {
		builder.WriteString(`</prosody>`)
	}
	if style != "" {
		builder.WriteString(`</mstts:express-as>`)
	}

	return builder.String()
}

func renderMicrosoftActions(cues []Cue) string {
	var builder strings.Builder
	for _, cue := range cues {
		if !cue.Mapped {
			continue
		}
		switch cue.Token {
		case TokenPause:
			builder.WriteString(`<break time="450ms"/>`)
		case TokenBreath:
			builder.WriteString(`<break time="250ms"/>`)
		case TokenLaugh:
			builder.WriteString(`<mstts:express-as style="cheerful">呵。</mstts:express-as>`)
		case TokenCough:
			builder.WriteString(`咳。`)
		}
	}
	return builder.String()
}

func microsoftStyle(cues []Cue) string {
	for _, cue := range cues {
		if !cue.Mapped {
			continue
		}
		switch cue.Token {
		case TokenWarm:
			return "affectionate"
		case TokenCalm:
			return "calm"
		case TokenSerious:
			return "serious"
		case TokenHappy:
			return "cheerful"
		case TokenSad:
			return "sad"
		case TokenTense:
			return "fearful"
		}
	}
	return ""
}

func microsoftProsody(cues []Cue) (rate string, volume string) {
	for _, cue := range cues {
		if !cue.Mapped {
			continue
		}
		switch cue.Token {
		case TokenSoft, TokenWhisper:
			if volume == "" {
				volume = "x-soft"
			}
		case TokenSlow, TokenMeasured:
			if rate == "" {
				rate = "slow"
			}
		case TokenFast:
			rate = "fast"
		}
	}
	return rate, volume
}

func escapeXMLText(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(value)
}
