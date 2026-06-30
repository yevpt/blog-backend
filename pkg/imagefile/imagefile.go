package imagefile

import (
	"errors"
	"strings"
)

var (
	ErrInvalidImage  = errors.New("图片格式不支持，请上传 JPG、PNG、WebP 或 GIF")
	ErrImageTooLarge = errors.New("图片不能超过限制大小")
)

type Result struct {
	Data        []byte
	ContentType string
	Ext         string
	MD5         string
}

func Validate(name string, data []byte, maxBytes int) (Result, error) {
	if len(data) == 0 {
		return Result{}, ErrInvalidImage
	}
	if maxBytes > 0 && len(data) > maxBytes {
		return Result{}, ErrImageTooLarge
	}
	format, err := validateImageBytes(data, DefaultMaxPixels)
	if err != nil {
		return Result{}, err
	}
	if _, _, ok := formatInfo(format); !ok {
		return Result{}, ErrInvalidImage
	}
	return buildResult(data, format), nil
}

func formatInfo(format string) (string, string, bool) {
	switch strings.ToLower(format) {
	case "jpeg":
		return "image/jpeg", ".jpg", true
	case "png":
		return "image/png", ".png", true
	case "gif":
		return "image/gif", ".gif", true
	case "webp":
		return "image/webp", ".webp", true
	default:
		return "", "", false
	}
}
