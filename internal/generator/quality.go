package generator

import "fmt"

const (
	defaultStrictRatio  = 0.80
	defaultRelaxedRatio = 0.65
	defaultMaxAttempts  = 3
)

type qualityGate struct {
	targetMin   int
	strictMin   int
	relaxedMin  int
	maxAttempts int
}

func newQualityGate(target ScriptLengthTarget) qualityGate {
	strictMin := scaleLength(target.Min, defaultStrictRatio)
	relaxedMin := scaleLength(target.Min, defaultRelaxedRatio)
	if relaxedMin >= strictMin {
		relaxedMin = strictMin - 1
	}
	if relaxedMin < 1 {
		relaxedMin = 1
	}

	return qualityGate{
		targetMin:   target.Min,
		strictMin:   strictMin,
		relaxedMin:  relaxedMin,
		maxAttempts: defaultMaxAttempts,
	}
}

func (g qualityGate) minimumForAttempt(attempt int) int {
	if attempt < g.maxAttempts-1 {
		return g.strictMin
	}
	return g.relaxedMin
}

func (g qualityGate) accepted(units, attempt int) bool {
	return units >= g.minimumForAttempt(attempt)
}

func (g qualityGate) retryInstruction(attempt, units int) string {
	if attempt >= g.maxAttempts-1 {
		return ""
	}
	return fmt.Sprintf(
		"上一次输出长度不足，只有约 %d 字，未达到最低 %d 字。请完整重写并显著扩写，补足背景、细节、例子、转折和收束，正文至少写到 %d 字后再结束。",
		units,
		g.minimumForAttempt(attempt),
		g.strictMin,
	)
}

func scaleLength(value int, ratio float64) int {
	scaled := int(float64(value) * ratio)
	if scaled < 1 {
		return 1
	}
	return scaled
}
