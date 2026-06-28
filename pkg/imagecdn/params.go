package imagecdn

import (
	"net/url"
	"strconv"
	"strings"

	appconfig "github.com/vpt/blog-backend/pkg/config"
)

// ParseTransformParams 解析 CDN 变换 query；无 w 时 transform=false。
func ParseTransformParams(raw url.Values, cfg appconfig.ImageConfig) (width int, quality int, transform bool) {
	widthRaw := strings.TrimSpace(raw.Get("w"))
	if widthRaw == "" {
		return 0, 0, false
	}

	parsedWidth, err := strconv.Atoi(widthRaw)
	if err != nil || parsedWidth <= 0 {
		return 0, 0, false
	}

	maxWidth := cfg.MaxWidth
	if maxWidth <= 0 {
		maxWidth = 3840
	}
	if parsedWidth > maxWidth {
		parsedWidth = maxWidth
	}

	defaultQuality := cfg.DefaultQuality
	if defaultQuality <= 0 {
		defaultQuality = 75
	}

	parsedQuality := defaultQuality
	if qualityRaw := strings.TrimSpace(raw.Get("q")); qualityRaw != "" {
		if q, err := strconv.Atoi(qualityRaw); err == nil {
			parsedQuality = q
		}
	}
	if parsedQuality < 1 {
		parsedQuality = 1
	}
	if parsedQuality > 100 {
		parsedQuality = 100
	}

	return parsedWidth, parsedQuality, true
}
