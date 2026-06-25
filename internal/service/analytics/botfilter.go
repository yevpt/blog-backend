package analytics

import (
	"regexp"
	"strings"
)

// botUARegex 覆盖常见爬虫/脚本 UA 特征（社区列表精简版）。
var botUARegex = regexp.MustCompile(`(?i)(bot|spider|crawl|slurp|headless|phantomjs|curl|wget|python-requests|python-urllib|go-http-client|java/|okhttp|scrapy|httpclient|facebookexternalhit|googlebot|bingbot|baiduspider|yandex|sogou|semrush|ahrefs|mj12)`)

// DetectBot 判定是否爬虫/机器人，返回是否命中与原因。
func DetectBot(ua, deviceType string) (bool, string) {
	// 优先按 UA 黑名单判定，使具名爬虫（如 Googlebot）统一归因为 ua_blacklist。
	if botUARegex.MatchString(strings.ToLower(ua)) {
		return true, "ua_blacklist"
	}
	if deviceType == "bot" {
		return true, "ua_device"
	}
	return false, ""
}
