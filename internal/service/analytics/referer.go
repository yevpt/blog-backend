package analytics

import "strings"

var searchHosts = []string{"google.", "baidu.", "bing.", "sogou.", "so.com", "duckduckgo.", "yandex."}
var socialHosts = []string{"t.co", "twitter.", "x.com", "facebook.", "zhihu.", "weibo.", "douban.", "github.", "telegram.", "t.me"}

// ClassifyReferer 把来源 host 归类为 direct/search/social/external/internal。
func ClassifyReferer(refererHost, siteHost string) string {
	h := strings.ToLower(strings.TrimSpace(refererHost))
	if h == "" {
		return "direct"
	}
	if h == strings.ToLower(siteHost) || strings.HasSuffix(h, "."+strings.ToLower(siteHost)) {
		return "internal"
	}
	for _, s := range searchHosts {
		if strings.Contains(h, s) {
			return "search"
		}
	}
	for _, s := range socialHosts {
		if strings.Contains(h, s) {
			return "social"
		}
	}
	return "external"
}
