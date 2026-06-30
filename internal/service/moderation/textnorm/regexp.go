package textnorm

import (
	"regexp"
	"regexp/syntax"
)

// CompileRegexp 只归一化正则字面量，保留转义、字符类和 Unicode 属性语义。
func CompileRegexp(pattern string) (*regexp.Regexp, error) {
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
	normalized := []rune(Normalize(string(expression.Rune)))
	if len(normalized) == 0 {
		expression.Op = syntax.OpEmptyMatch
		expression.Rune = nil
		return
	}
	expression.Rune = normalized
}
