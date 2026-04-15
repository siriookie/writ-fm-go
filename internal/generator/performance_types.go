package generator

import (
	"strings"

	perf "github.com/writ-fm/go/internal/generator/performance"
)

type PerformanceMode = perf.Mode
type PerformanceToken = perf.Token
type PerformanceCue = perf.Cue
type NormalizedScript = perf.Script

const (
	PerformanceModeConstrained PerformanceMode = perf.ModeConstrained
	PerformanceModeExpressive  PerformanceMode = perf.ModeExpressive

	PerformanceTokenPause    PerformanceToken = perf.TokenPause
	PerformanceTokenBreath   PerformanceToken = perf.TokenBreath
	PerformanceTokenLaugh    PerformanceToken = perf.TokenLaugh
	PerformanceTokenCough    PerformanceToken = perf.TokenCough
	PerformanceTokenWarm     PerformanceToken = perf.TokenWarm
	PerformanceTokenCalm     PerformanceToken = perf.TokenCalm
	PerformanceTokenSerious  PerformanceToken = perf.TokenSerious
	PerformanceTokenHappy    PerformanceToken = perf.TokenHappy
	PerformanceTokenSad      PerformanceToken = perf.TokenSad
	PerformanceTokenTense    PerformanceToken = perf.TokenTense
	PerformanceTokenSoft     PerformanceToken = perf.TokenSoft
	PerformanceTokenWhisper  PerformanceToken = perf.TokenWhisper
	PerformanceTokenSlow     PerformanceToken = perf.TokenSlow
	PerformanceTokenFast     PerformanceToken = perf.TokenFast
	PerformanceTokenMeasured PerformanceToken = perf.TokenMeasured
)

func NormalizePerformanceMode(mode PerformanceMode) PerformanceMode {
	return perf.NormalizeMode(mode)
}

func NormalizePerformanceCues(script string, mode PerformanceMode, backend string) NormalizedScript {
	return perf.NormalizePerformanceCues(script, perf.Mode(mode), backend)
}

func RenderPerformanceForBackend(normalized NormalizedScript, backend string) string {
	return perf.RenderPerformanceForBackend(normalized, backend)
}

func performancePromptInstructions(mode PerformanceMode) string {
	allowed := []string{
		"[pause]", "[breath]", "[laugh]", "[cough]",
		"[warm]", "[calm]", "[serious]", "[happy]", "[sad]", "[tense]",
		"[soft]", "[whisper]", "[slow]", "[fast]", "[measured]",
	}

	if NormalizePerformanceMode(mode) == PerformanceModeExpressive {
		return strings.Join([]string{
			"表演控制模式：expressive。",
			"你可以在必要时使用自然语言括号提示，例如“（深呼吸）”“（小声）”“（沉默片刻）”。",
			"括号提示必须克制，每个自然段最多 1 到 2 个，不要把稿子写成舞台说明。",
			"如果只需要稳定控制，也可以直接使用系统白名单标记：" + strings.Join(allowed, " "),
			"优先保证广播可读性，提示只在确实会改善语气、停顿或节奏时才加入。",
		}, "\n")
	}

	return strings.Join([]string{
		"表演控制模式：constrained。",
		"只能使用系统允许的控制标记，不得自由发明括号提示、舞台说明或新的标签。",
		"允许使用的控制标记只有：" + strings.Join(allowed, " "),
		"控制标记要少而准，只在停顿、呼吸、轻笑、语速和语气确实需要控制时使用。",
	}, "\n")
}
