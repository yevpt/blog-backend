package textnorm

import (
	"strings"
	"unicode"

	"github.com/caiguanhao/opencc"
	"golang.org/x/text/unicode/norm"
)

// Normalize 为审核匹配生成稳定文本，不修改对外展示正文。
func Normalize(text string) string {
	text = norm.NFKC.String(text)
	text = opencc.Convert("t2s", text)
	text = strings.ToLower(text)

	var normalized strings.Builder
	var previous rune
	hasPrevious := false
	for _, current := range text {
		if isIgnored(current) {
			continue
		}
		if hasPrevious && current == previous {
			continue
		}
		normalized.WriteRune(current)
		previous = current
		hasPrevious = true
	}
	return normalized.String()
}

func isIgnored(value rune) bool {
	return unicode.IsSpace(value) || isDefaultIgnorable(value) ||
		unicode.IsPunct(value) || unicode.IsSymbol(value)
}

func isDefaultIgnorable(value rune) bool {
	return unicode.Is(unicode.Cf, value) ||
		unicode.Is(unicode.Other_Default_Ignorable_Code_Point, value) ||
		unicode.Is(unicode.Variation_Selector, value)
}
