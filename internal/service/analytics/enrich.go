package analytics

import "github.com/vpt/blog-backend/internal/model"

const maxTitleLen = 255

// Enricher 把 RawEvent 富化为可入库的 AnalyticsEvent。
type Enricher interface {
	Enrich(raw RawEvent) model.AnalyticsEvent
}

type enricher struct {
	geo      GeoResolver
	siteHost string
	ipSalt   string
}

// NewEnricher 构造富化器。siteHost 用于 referer internal 判定，ipSalt 用于 ip 哈希。
func NewEnricher(geo GeoResolver, siteHost, ipSalt string) Enricher {
	return &enricher{geo: geo, siteHost: siteHost, ipSalt: ipSalt}
}

func (e *enricher) Enrich(raw RawEvent) model.AnalyticsEvent {
	deviceType, browser, os := ParseUserAgent(raw.UA)
	isBot, botReason := DetectBot(raw.UA, deviceType)
	refHost := RefererHost(raw.Referer)
	country, region := e.geo.Resolve(raw.IP)

	title := raw.Title
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen]
	}

	return model.AnalyticsEvent{
		EventType:       raw.EventType,
		VisitorID:       raw.VisitorID,
		UserID:          raw.UserID,
		IsAuthenticated: raw.UserID != nil,
		SessionID:       raw.SessionID,
		Path:            SanitizePath(raw.Path),
		Title:           title,
		RefererHost:     refHost,
		RefererType:     ClassifyReferer(refHost, e.siteHost),
		DeviceType:      deviceType,
		Browser:         browser,
		OS:              os,
		Country:         country,
		Region:          region,
		IPHash:          HashIP(raw.IP, e.ipSalt),
		IsBot:           isBot,
		BotReason:       botReason,
		// Origin 不在允许列表内则标记为可疑流量。
		IsSuspect: !raw.OriginAllowed,
	}
}
