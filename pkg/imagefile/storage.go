package imagefile

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"image"
	"strings"

	"github.com/vpt/blog-backend/pkg/imageutil"
)

// PrepareOptions 控制入库前是否原样保留，以及超出限制时如何压缩为 WebP。
type PrepareOptions struct {
	MaxStoredBytes int // 最大字节数，0 表示不限制
	MaxWidth       int // 最大宽度，0 表示不限制
	MaxHeight      int // 最大高度，0 表示不限制
	MaxPixels      int // 最大像素数（宽×高），0 使用 DefaultMaxPixels
	WebPQuality    int // 需要压缩时的 WebP 起始质量，0 使用默认
	MinWebPQuality int // 需要压缩时的 WebP 最低质量，0 使用默认
}

// PrepareForStorage 校验图片；尺寸与体积均已符合时原样返回，否则以尽量高的 WebP 质量压到限制内。
func PrepareForStorage(name string, data []byte, opts PrepareOptions) (Result, error) {
	if len(data) == 0 {
		return Result{}, ErrInvalidImage
	}

	format, err := validateImageBytes(data, effectiveMaxPixels(opts.MaxPixels))
	if err != nil {
		return Result{}, err
	}
	validated := buildResult(data, format)
	if fits, fitErr := fitsStorageLimits(validated.Data, opts); fitErr != nil {
		return Result{}, fitErr
	} else if fits {
		return validated, nil
	}

	webpQuality := opts.WebPQuality
	if webpQuality <= 0 {
		webpQuality = imageutil.DefaultWebPQuality
	}
	minQuality := opts.MinWebPQuality
	if minQuality <= 0 {
		minQuality = imageutil.DefaultMinWebPQuality
	}

	processed, err := imageutil.Process(bytes.NewReader(validated.Data), imageutil.Options{
		Format:         imageutil.FormatWebP,
		MaxWidth:       opts.MaxWidth,
		MaxHeight:      opts.MaxHeight,
		MaxBytes:       opts.MaxStoredBytes,
		WebPQuality:    webpQuality,
		MinWebPQuality: minQuality,
	})
	if err != nil {
		return Result{}, err
	}

	return Result{
		Data:        processed.Bytes,
		ContentType: processed.ContentType,
		Ext:         processed.Ext,
		MD5:         processed.MD5,
	}, nil
}

func buildResult(data []byte, format string) Result {
	contentType, ext, _ := formatInfo(format)
	sum := md5.Sum(data)
	return Result{
		Data:        data,
		ContentType: contentType,
		Ext:         ext,
		MD5:         hex.EncodeToString(sum[:]),
	}
}

func fitsStorageLimits(data []byte, opts PrepareOptions) (bool, error) {
	if opts.MaxStoredBytes > 0 && len(data) > opts.MaxStoredBytes {
		return false, nil
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return false, err
	}
	if strings.EqualFold(format, "gif") {
		return false, nil
	}
	if int64(cfg.Width)*int64(cfg.Height) > effectiveMaxPixels(opts.MaxPixels) {
		return false, nil
	}
	if opts.MaxWidth > 0 && cfg.Width > opts.MaxWidth {
		return false, nil
	}
	if opts.MaxHeight > 0 && cfg.Height > opts.MaxHeight {
		return false, nil
	}
	return true, nil
}
