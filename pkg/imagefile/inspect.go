package imagefile

import (
	"bytes"
	"image"
	"image/gif"
	"strings"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// DefaultMaxPixels 是解码前允许的最大像素数，用于抵御压缩炸弹导致的 OOM。
const DefaultMaxPixels = 12_000_000

func effectiveMaxPixels(maxPixels int) int64 {
	if maxPixels > 0 {
		return int64(maxPixels)
	}
	return DefaultMaxPixels
}

// inspectImage 先读头部尺寸并限制像素，再完整解码以确认文件可解析。
func inspectImage(data []byte, maxPixels int64) (format string, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", ErrInvalidImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return "", ErrInvalidImage
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return "", ErrImageTooManyPixels
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return "", ErrInvalidImage
	}
	return format, nil
}

func validateImageBytes(data []byte, maxPixels int64) (format string, err error) {
	format, err = inspectImage(data, maxPixels)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(format, "gif") {
		if _, err := gif.DecodeAll(bytes.NewReader(data)); err != nil {
			return "", ErrInvalidImage
		}
	}
	return format, nil
}
