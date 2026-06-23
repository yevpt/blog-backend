package imagefile

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"image"
	"strings"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
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
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Result{}, ErrInvalidImage
	}
	contentType, ext, ok := formatInfo(format)
	if !ok {
		return Result{}, ErrInvalidImage
	}
	sum := md5.Sum(data)
	return Result{
		Data:        data,
		ContentType: contentType,
		Ext:         ext,
		MD5:         hex.EncodeToString(sum[:]),
	}, nil
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
