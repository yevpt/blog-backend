package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestDetectBot(t *testing.T) {
	bot, reason := svc.DetectBot("Mozilla/5.0 (compatible; Googlebot/2.1)", "desktop")
	assert.True(t, bot)
	assert.Equal(t, "ua_blacklist", reason)

	bot2, reason2 := svc.DetectBot("python-requests/2.31", "desktop")
	assert.True(t, bot2)
	assert.Equal(t, "ua_blacklist", reason2)

	bot3, reason3 := svc.DetectBot("some-ua", "bot")
	assert.True(t, bot3)
	assert.Equal(t, "ua_device", reason3)

	bot4, reason4 := svc.DetectBot("Mozilla/5.0 (Windows NT 10.0) Chrome/120", "desktop")
	assert.False(t, bot4)
	assert.Equal(t, "", reason4)
}
