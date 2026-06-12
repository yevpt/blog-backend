package imageutil

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	xdraw "golang.org/x/image/draw"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

var (
	// ErrInvalidImage 表示输入内容无法被 Go image 解码为真实图片。
	ErrInvalidImage = errors.New("无效的图片文件")
	// ErrUnsupportedFormat 表示调用方指定了当前处理器不支持的输出格式。
	ErrUnsupportedFormat = errors.New("不支持的图片输出格式")
	// ErrImageTooLarge 表示在给定参数内无法把图片压缩到目标大小。
	ErrImageTooLarge = errors.New("图片无法压缩到目标大小")
)

// Format 表示图片处理后的输出格式。
type Format string

const (
	// FormatJPEG 输出 JPEG，适合头像和多数照片类图片。
	FormatJPEG Format = "jpeg"
	// FormatPNG 输出 PNG，适合需要无损或透明背景的图片。
	FormatPNG Format = "png"
)

// Options 控制图片处理行为，供头像、前端上传等场景复用。
type Options struct {
	MaxWidth       int    // 最大宽度，0 表示不限制
	MaxHeight      int    // 最大高度，0 表示不限制
	MaxBytes       int    // 最大输出体积，0 表示不限制
	Format         Format // 输出格式，空值默认 JPEG
	JPEGQuality    int    // JPEG 初始质量，0 使用默认 85
	MinJPEGQuality int    // JPEG 最低质量，0 使用默认 40
}

// Result 是图片处理后的结果。
type Result struct {
	Bytes       []byte // 压缩后的图片内容
	Format      Format // 输出格式
	ContentType string // 对应 HTTP Content-Type
	Ext         string // 推荐文件扩展名，包含点号
	Width       int    // 输出宽度
	Height      int    // 输出高度
	MD5         string // 输出内容的 MD5 十六进制摘要
}

// Process 解码、缩放并编码图片；全程内存处理，不创建临时文件。
func Process(r io.Reader, opts Options) (*Result, error) {
	opts = normalizeOptions(opts)

	src, _, err := image.Decode(r)
	if err != nil {
		return nil, ErrInvalidImage
	}

	img := resizeToFit(src, opts.MaxWidth, opts.MaxHeight)
	encoded, err := encodeWithinLimit(img, opts)
	if err != nil {
		return nil, err
	}

	sum := md5.Sum(encoded)
	bounds := img.Bounds()
	return &Result{
		Bytes:       encoded,
		Format:      opts.Format,
		ContentType: contentType(opts.Format),
		Ext:         extension(opts.Format),
		Width:       bounds.Dx(),
		Height:      bounds.Dy(),
		MD5:         hex.EncodeToString(sum[:]),
	}, nil
}

func normalizeOptions(opts Options) Options {
	if opts.Format == "" {
		opts.Format = FormatJPEG
	}
	if opts.JPEGQuality <= 0 || opts.JPEGQuality > 100 {
		opts.JPEGQuality = 85
	}
	if opts.MinJPEGQuality <= 0 || opts.MinJPEGQuality > opts.JPEGQuality {
		opts.MinJPEGQuality = 40
	}
	return opts
}

func resizeToFit(src image.Image, maxWidth, maxHeight int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return src
	}
	if maxWidth <= 0 && maxHeight <= 0 {
		return src
	}

	scale := 1.0
	if maxWidth > 0 && width > maxWidth {
		scale = min(scale, float64(maxWidth)/float64(width))
	}
	if maxHeight > 0 && height > maxHeight {
		scale = min(scale, float64(maxHeight)/float64(height))
	}
	if scale >= 1 {
		return src
	}

	targetWidth := max(1, int(float64(width)*scale))
	targetHeight := max(1, int(float64(height)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	return dst
}

func encodeWithinLimit(img image.Image, opts Options) ([]byte, error) {
	switch opts.Format {
	case FormatJPEG:
		return encodeJPEGWithinLimit(img, opts)
	case FormatPNG:
		return encodePNGWithinLimit(img, opts)
	default:
		return nil, ErrUnsupportedFormat
	}
}

func encodeJPEGWithinLimit(img image.Image, opts Options) ([]byte, error) {
	current := img
	for {
		for quality := opts.JPEGQuality; quality >= opts.MinJPEGQuality; quality -= 5 {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, current, &jpeg.Options{Quality: quality}); err != nil {
				return nil, err
			}
			if opts.MaxBytes <= 0 || buf.Len() <= opts.MaxBytes {
				return buf.Bytes(), nil
			}
		}

		bounds := current.Bounds()
		width := bounds.Dx()
		height := bounds.Dy()
		if opts.MaxBytes <= 0 || width <= 24 || height <= 24 {
			return nil, ErrImageTooLarge
		}
		nextWidth := max(1, width*9/10)
		nextHeight := max(1, height*9/10)
		dst := image.NewRGBA(image.Rect(0, 0, nextWidth, nextHeight))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), current, bounds, xdraw.Over, nil)
		current = dst
	}
}

func encodePNGWithinLimit(img image.Image, opts Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	if opts.MaxBytes > 0 && buf.Len() > opts.MaxBytes {
		return nil, ErrImageTooLarge
	}
	return buf.Bytes(), nil
}

func contentType(format Format) string {
	switch format {
	case FormatPNG:
		return "image/png"
	default:
		return "image/jpeg"
	}
}

func extension(format Format) string {
	switch format {
	case FormatPNG:
		return ".png"
	default:
		return ".jpg"
	}
}
