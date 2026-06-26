package analytics_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestSanitizePath(t *testing.T) {
	assert.Equal(t, "/articles/1", svc.SanitizePath("/articles/1?token=secret#x"))
	assert.Equal(t, "/", svc.SanitizePath("/"))
}

func TestSanitizePath_TruncatesWithoutBreakingUTF8(t *testing.T) {
	path := "/" + strings.Repeat("你", 300)

	got := svc.SanitizePath(path)

	assert.LessOrEqual(t, len(got), 512)
	assert.True(t, utf8.ValidString(got))
	assert.NotContains(t, got, "�")
}

func TestRefererHost(t *testing.T) {
	assert.Equal(t, "www.google.com", svc.RefererHost("https://www.google.com/search?q=x"))
	assert.Equal(t, "", svc.RefererHost(""))
	assert.Equal(t, "", svc.RefererHost("not a url"))
}

func TestHashIP(t *testing.T) {
	assert.Equal(t, "", svc.HashIP("", "salt"))
	// 同段 IP 末段不同 → 同一哈希（已去末段）
	a := svc.HashIP("1.2.3.4", "salt")
	b := svc.HashIP("1.2.3.99", "salt")
	assert.Equal(t, a, b)
	assert.Len(t, a, 16)
	// 不同 salt → 不同哈希
	assert.NotEqual(t, a, svc.HashIP("1.2.3.4", "other"))
}
