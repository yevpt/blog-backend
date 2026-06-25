package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestSanitizePath(t *testing.T) {
	assert.Equal(t, "/articles/1", svc.SanitizePath("/articles/1?token=secret#x"))
	assert.Equal(t, "/", svc.SanitizePath("/"))
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
