package moderation

import (
	"regexp"

	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

func compileNormalizedRegexp(pattern string) (*regexp.Regexp, error) {
	return textnorm.CompileRegexp(pattern)
}
