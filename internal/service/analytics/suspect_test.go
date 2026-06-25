package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestSuspectDecisionOrigin(t *testing.T) {
	ok, reason := svc.DecideSuspect(svc.RawEvent{OriginAllowed: false}, true, "")
	assert.True(t, ok)
	assert.Equal(t, "origin_denied", reason)
}

func TestSuspectDecisionToken(t *testing.T) {
	ok, reason := svc.DecideSuspect(svc.RawEvent{OriginAllowed: true}, false, "collect_token_invalid")
	assert.True(t, ok)
	assert.Equal(t, "collect_token_invalid", reason)
}

func TestSuspectDecisionWebDriver(t *testing.T) {
	ok, reason := svc.DecideSuspect(svc.RawEvent{OriginAllowed: true, Signals: svc.CollectSignals{WebDriver: true}}, true, "")
	assert.True(t, ok)
	assert.Equal(t, "webdriver", reason)
}

func TestSuspectDecisionNoInteractionHintAloneDoesNotMark(t *testing.T) {
	ok, reason := svc.DecideSuspect(svc.RawEvent{OriginAllowed: true, EventType: "page_view", Signals: svc.CollectSignals{NoInteraction: true}}, true, "")
	assert.False(t, ok)
	assert.Empty(t, reason)
}
