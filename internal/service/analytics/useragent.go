package analytics

import "github.com/mileusna/useragent"

// ParseUserAgent 解析 UA，返回设备类型/浏览器/操作系统。
func ParseUserAgent(ua string) (deviceType, browser, os string) {
	if ua == "" {
		return "desktop", "", ""
	}
	p := useragent.Parse(ua)
	switch {
	case p.Bot:
		deviceType = "bot"
	case p.Tablet:
		deviceType = "tablet"
	case p.Mobile:
		deviceType = "mobile"
	default:
		deviceType = "desktop"
	}
	return deviceType, p.Name, p.OS
}
