package moderation

import (
	"regexp"
	"regexp/syntax"
)

func compileNormalizedRegexp(pattern string) (*regexp.Regexp, error) {
	tree, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, err
	}
	normalizeRegexpLiterals(tree)
	return regexp.Compile(tree.String())
}

func normalizeRegexpLiterals(expression *syntax.Regexp) {
	if expression.Op == syntax.OpLiteral {
		normalizeRegexpLiteral(expression)
	}
	for _, child := range expression.Sub {
		normalizeRegexpLiterals(child)
	}
}

func normalizeRegexpLiteral(expression *syntax.Regexp) {
	normalized := []rune(NormalizeText(string(expression.Rune)))
	if len(normalized) == 0 {
		expression.Op = syntax.OpEmptyMatch
		expression.Rune = nil
		return
	}
	expression.Rune = normalized
}
