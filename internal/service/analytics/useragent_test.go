package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestParseUserAgent(t *testing.T) {
	chrome := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
	dt, br, os := svc.ParseUserAgent(chrome)
	assert.Equal(t, "desktop", dt)
	assert.Equal(t, "Chrome", br)
	assert.Equal(t, "Windows", os)

	iphone := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148"
	dt, _, _ = svc.ParseUserAgent(iphone)
	assert.Equal(t, "mobile", dt)

	dt2, _, _ := svc.ParseUserAgent("")
	assert.Equal(t, "desktop", dt2)
}
