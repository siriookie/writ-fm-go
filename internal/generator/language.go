package generator

import (
	"fmt"
	"unicode"
)

func validateChineseScript(script string) error {
	total := 0
	han := 0
	for _, token := range textUnitRE.FindAllString(script, -1) {
		total++
		for _, r := range token {
			if unicode.Is(unicode.Han, r) {
				han++
				break
			}
		}
	}
	if total == 0 {
		return fmt.Errorf("empty text")
	}
	minHan := 10
	if total < 30 {
		minHan = 1
	}
	if han < minHan || float64(han)/float64(total) < 0.35 {
		return fmt.Errorf("got %d Han units out of %d text units", han, total)
	}
	return nil
}
