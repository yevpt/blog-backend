package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/model"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

type fakeGeo struct{}

func (fakeGeo) Resolve(string) (string, string) { return "中国", "浙江省" }

func TestEnrich(t *testing.T) {
	e := svc.NewEnricher(fakeGeo{}, "example.com", "salt")
	uid := uint(7)
	got := e.Enrich(svc.RawEvent{
		EventType: "page_view",
		VisitorID: "v1",
		SessionID: "s1",
		Path:      "/articles/1?token=x",
		Title:     "标题",
		Referer:   "https://www.google.com/search?q=a",
		UA:        "Mozilla/5.0 (Windows NT 10.0) Chrome/120 Safari/537.36",
		IP:        "1.2.3.4",
		UserID:    &uid,
	})

	assert.Equal(t, "/articles/1", got.Path)
	assert.Equal(t, "www.google.com", got.RefererHost)
	assert.Equal(t, "search", got.RefererType)
	assert.Equal(t, "desktop", got.DeviceType)
	assert.Equal(t, "中国", got.Country)
	assert.Equal(t, "浙江省", got.Region)
	assert.Equal(t, uint(7), *got.UserID)
	assert.True(t, got.IsAuthenticated)
	assert.False(t, got.IsBot)
	assert.NotEmpty(t, got.IPHash)

	var _ model.AnalyticsEvent = got
}

func TestEnrichBot(t *testing.T) {
	e := svc.NewEnricher(fakeGeo{}, "example.com", "salt")
	got := e.Enrich(svc.RawEvent{EventType: "page_view", VisitorID: "v", SessionID: "s",
		Path: "/", UA: "Googlebot/2.1", IP: "1.2.3.4"})
	assert.True(t, got.IsBot)
	assert.Equal(t, "ua_blacklist", got.BotReason)
	assert.False(t, got.IsAuthenticated)
}

func TestEnrichPassesThroughSuspect(t *testing.T) {
	// suspect 判定已上移到 DecideSuspect，富化仅透传 raw.IsSuspect/SuspectReason。
	e := svc.NewEnricher(fakeGeo{}, "example.com", "salt")

	clean := e.Enrich(svc.RawEvent{EventType: "page_view", VisitorID: "v", SessionID: "s",
		Path: "/", IP: "1.2.3.4"})
	assert.False(t, clean.IsSuspect)
	assert.Empty(t, clean.SuspectReason)

	suspect := e.Enrich(svc.RawEvent{EventType: "page_view", VisitorID: "v", SessionID: "s",
		Path: "/", IP: "1.2.3.4", IsSuspect: true, SuspectReason: "origin_denied"})
	assert.True(t, suspect.IsSuspect)
	assert.Equal(t, "origin_denied", suspect.SuspectReason)
}
