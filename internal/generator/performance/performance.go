package performance

import (
	"regexp"
	"strings"
)

type Mode string

const (
	ModeConstrained Mode = "constrained"
	ModeExpressive  Mode = "expressive"
)

type Token string

const (
	TokenPause    Token = "pause"
	TokenBreath   Token = "breath"
	TokenLaugh    Token = "laugh"
	TokenCough    Token = "cough"
	TokenWarm     Token = "warm"
	TokenCalm     Token = "calm"
	TokenSerious  Token = "serious"
	TokenHappy    Token = "happy"
	TokenSad      Token = "sad"
	TokenTense    Token = "tense"
	TokenSoft     Token = "soft"
	TokenWhisper  Token = "whisper"
	TokenSlow     Token = "slow"
	TokenFast     Token = "fast"
	TokenMeasured Token = "measured"
)

type Cue struct {
	Raw      string
	Token    Token
	Strength int
	Mapped   bool
}

type Element struct {
	Text string
	Cue  *Cue
}

type Script struct {
	Mode     Mode
	Backend  string
	Elements []Element
	Cues     []Cue
}

type parsedMatch struct {
	start int
	end   int
	raw   string
	cue   Cue
}

var (
	constrainedTokenRE = regexp.MustCompile(`\[(pause|breath|laugh|cough|warm|calm|serious|happy|sad|tense|soft|whisper|slow|fast|measured)\]`)
	legacyCueRE        = regexp.MustCompile(`\[[a-z_]+\]`)
	expressiveCueRE    = regexp.MustCompile(`（[^）]+）|\([^)]*\)`)
)

func NormalizeMode(mode Mode) Mode {
	switch mode {
	case ModeExpressive:
		return ModeExpressive
	default:
		return ModeConstrained
	}
}

func NormalizePerformanceCues(script string, mode Mode, backend string) Script {
	mode = NormalizeMode(mode)
	if mode == ModeExpressive {
		return normalizeExpressive(script, backend)
	}
	return normalizeConstrained(script, backend)
}

func RenderPerformanceForBackend(script Script, backend string) string {
	backend = normalizeBackend(backend)
	switch backend {
	case "mimo":
		return renderMimo(script)
	case "microsoft":
		return renderMicrosoft(script)
	case "kokoro", "kokoro_modal":
		return renderTextual(script, backendTextMappings{
			actions: map[Token]string{
				TokenPause:  "……",
				TokenBreath: "（轻轻吸气）",
				TokenLaugh:  "（轻笑）",
				TokenCough:  "（轻咳）",
			},
			styles: map[Token]string{
				TokenWarm:     "（语气温暖）",
				TokenCalm:     "（语气平静）",
				TokenSerious:  "（语气认真）",
				TokenHappy:    "（语气愉快）",
				TokenSad:      "（语气低回）",
				TokenTense:    "（略带紧张感）",
				TokenSoft:     "（放轻声音）",
				TokenWhisper:  "（小声）",
				TokenSlow:     "（语速放慢）",
				TokenFast:     "（语速稍快）",
				TokenMeasured: "（字句沉稳）",
			},
			allowRaw: backend == "kokoro_modal" && script.Mode == ModeExpressive,
		})
	default:
		return renderTextual(script, backendTextMappings{
			actions: map[Token]string{
				TokenPause:  "……",
				TokenBreath: "（深呼吸）",
				TokenLaugh:  "（轻笑）",
				TokenCough:  "（咳嗽一声）",
			},
			styles: map[Token]string{
				TokenWarm:     "（语气温暖）",
				TokenCalm:     "（语气平静）",
				TokenSerious:  "（语气认真）",
				TokenHappy:    "（开心）",
				TokenSad:      "（有些伤感）",
				TokenTense:    "（略带紧张）",
				TokenSoft:     "（放轻声音）",
				TokenWhisper:  "（小声）",
				TokenSlow:     "（语速放慢）",
				TokenFast:     "（语速加快）",
				TokenMeasured: "（字句沉稳）",
			},
			allowRaw: script.Mode == ModeExpressive,
		})
	}
}

func normalizeConstrained(script string, backend string) Script {
	script = strings.TrimSpace(script)
	if script == "" {
		return Script{Mode: ModeConstrained, Backend: normalizeBackend(backend)}
	}

	matches := constrainedTokenRE.FindAllStringSubmatchIndex(script, -1)
	if len(matches) == 0 {
		cleaned := strings.TrimSpace(legacyCueRE.ReplaceAllString(script, ""))
		return Script{
			Mode:    ModeConstrained,
			Backend: normalizeBackend(backend),
			Elements: []Element{
				{Text: collapseWhitespace(cleaned)},
			},
		}
	}

	elements := make([]Element, 0, len(matches)*2+1)
	cues := make([]Cue, 0, len(matches))
	last := 0
	for _, match := range matches {
		if match[0] > last {
			text := collapseWhitespace(legacyCueRE.ReplaceAllString(script[last:match[0]], ""))
			if text != "" {
				elements = append(elements, Element{Text: text})
			}
		}

		raw := script[match[0]:match[1]]
		token := Token(strings.ToLower(script[match[2]:match[3]]))
		cue := Cue{
			Raw:      raw,
			Token:    token,
			Strength: 1,
			Mapped:   true,
		}
		cues = append(cues, cue)
		cueCopy := cue
		elements = append(elements, Element{Cue: &cueCopy})
		last = match[1]
	}

	if last < len(script) {
		text := collapseWhitespace(legacyCueRE.ReplaceAllString(script[last:], ""))
		if text != "" {
			elements = append(elements, Element{Text: text})
		}
	}

	return Script{
		Mode:     ModeConstrained,
		Backend:  normalizeBackend(backend),
		Elements: elements,
		Cues:     cues,
	}
}

func normalizeExpressive(script string, backend string) Script {
	script = strings.TrimSpace(script)
	if script == "" {
		return Script{Mode: ModeExpressive, Backend: normalizeBackend(backend)}
	}

	var matches []parsedMatch
	for _, idx := range constrainedTokenRE.FindAllStringSubmatchIndex(script, -1) {
		token := Token(strings.ToLower(script[idx[2]:idx[3]]))
		matches = append(matches, parsedMatch{
			start: idx[0],
			end:   idx[1],
			raw:   script[idx[0]:idx[1]],
			cue: Cue{
				Raw:      script[idx[0]:idx[1]],
				Token:    token,
				Strength: 1,
				Mapped:   true,
			},
		})
	}
	for _, idx := range expressiveCueRE.FindAllStringIndex(script, -1) {
		raw := script[idx[0]:idx[1]]
		cue := mapExpressiveCue(raw)
		matches = append(matches, parsedMatch{
			start: idx[0],
			end:   idx[1],
			raw:   raw,
			cue:   cue,
		})
	}

	if len(matches) == 0 {
		return Script{
			Mode:    ModeExpressive,
			Backend: normalizeBackend(backend),
			Elements: []Element{
				{Text: collapseWhitespace(legacyCueRE.ReplaceAllString(script, ""))},
			},
		}
	}

	sortMatches(matches)
	elements := make([]Element, 0, len(matches)*2+1)
	cues := make([]Cue, 0, len(matches))
	last := 0
	for _, match := range matches {
		if match.start < last {
			continue
		}
		if match.start > last {
			text := collapseWhitespace(script[last:match.start])
			if text != "" {
				elements = append(elements, Element{Text: text})
			}
		}
		cue := match.cue
		cues = append(cues, cue)
		cueCopy := cue
		elements = append(elements, Element{Cue: &cueCopy})
		last = match.end
	}
	if last < len(script) {
		text := collapseWhitespace(script[last:])
		if text != "" {
			elements = append(elements, Element{Text: text})
		}
	}

	return Script{
		Mode:     ModeExpressive,
		Backend:  normalizeBackend(backend),
		Elements: elements,
		Cues:     cues,
	}
}

func mapExpressiveCue(raw string) Cue {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "（"), "）"))
	trimmed = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "("), ")"))
	lowered := strings.ToLower(trimmed)

	cue := Cue{
		Raw:      raw,
		Strength: 1,
	}

	switch {
	case containsAny(trimmed, "深呼吸", "吸气", "呼——", "呼吸急促"), strings.Contains(lowered, "breath"):
		cue.Token = TokenBreath
		cue.Mapped = true
	case containsAny(trimmed, "沉默", "停顿", "稍作停顿", "片刻"), strings.Contains(lowered, "pause"), strings.Contains(lowered, "silence"):
		cue.Token = TokenPause
		cue.Mapped = true
	case containsAny(trimmed, "苦笑", "轻笑", "笑", "笑了一下"), strings.Contains(lowered, "laugh"), strings.Contains(lowered, "chuckle"):
		cue.Token = TokenLaugh
		cue.Mapped = true
	case containsAny(trimmed, "咳", "清嗓"), strings.Contains(lowered, "cough"):
		cue.Token = TokenCough
		cue.Mapped = true
	case containsAny(trimmed, "小声", "耳语", "悄悄话", "压低声音"), strings.Contains(lowered, "whisper"):
		cue.Token = TokenWhisper
		cue.Mapped = true
	case containsAny(trimmed, "放轻", "轻柔"), strings.Contains(lowered, "soft"):
		cue.Token = TokenSoft
		cue.Mapped = true
	case containsAny(trimmed, "语速加快", "变快", "快一点", "碎碎念", "提高音量喊话", "喊话"), strings.Contains(lowered, "fast"):
		cue.Token = TokenFast
		cue.Mapped = true
	case containsAny(trimmed, "语速放慢", "变慢", "慢一点", "有气无力"), strings.Contains(lowered, "slow"):
		cue.Token = TokenSlow
		cue.Mapped = true
	case containsAny(trimmed, "沉稳", "克制", "稳住"), strings.Contains(lowered, "measured"):
		cue.Token = TokenMeasured
		cue.Mapped = true
	case containsAny(trimmed, "温暖", "柔和"), strings.Contains(lowered, "warm"):
		cue.Token = TokenWarm
		cue.Mapped = true
	case containsAny(trimmed, "平静", "冷静"), strings.Contains(lowered, "calm"):
		cue.Token = TokenCalm
		cue.Mapped = true
	case containsAny(trimmed, "严肃", "认真"), strings.Contains(lowered, "serious"):
		cue.Token = TokenSerious
		cue.Mapped = true
	case containsAny(trimmed, "开心", "愉快", "高兴"), strings.Contains(lowered, "happy"):
		cue.Token = TokenHappy
		cue.Mapped = true
	case containsAny(trimmed, "悲伤", "疲惫", "有气无力", "哽咽"), strings.Contains(lowered, "sad"):
		cue.Token = TokenSad
		cue.Mapped = true
	case containsAny(trimmed, "紧张", "压迫", "提高音量", "喊"), strings.Contains(lowered, "tense"), strings.Contains(lowered, "nervous"):
		cue.Token = TokenTense
		cue.Mapped = true
	default:
		cue.Mapped = false
	}

	return cue
}

func normalizeBackend(backend string) string {
	return strings.ToLower(strings.TrimSpace(backend))
}

func containsAny(value string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(value, sub) {
			return true
		}
	}
	return false
}

func collapseWhitespace(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func sortMatches(matches []parsedMatch) {
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].start < matches[i].start {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
}
