package imagecdn

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	appconfig "github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/imageutil"
	"github.com/vpt/blog-backend/pkg/storage"
	"golang.org/x/sync/singleflight"
)

// ErrNotFound 表示对象不存在或无法读取。
var ErrNotFound = errors.New("图片对象不存在")

// ErrSourceTooLarge 表示源图超过 CDN 回源读取上限。
var ErrSourceTooLarge = errors.New("源图超过读取上限")

// objectGetter 从对象存储直读图片原图。
type objectGetter interface {
	GetImageObject(ctx context.Context, objectName string) ([]byte, error)
}

// Service 处理 CDN 回源图片直传与变换。
type Service struct {
	store objectGetter
	cfg   appconfig.ImageConfig
	group singleflight.Group
}

// NewService 创建 CDN 图片服务。
func NewService(store objectGetter, cfg appconfig.ImageConfig) *Service {
	return &Service{store: store, cfg: cfg}
}

type servePayload struct {
	body        []byte
	contentType string
	etag        string
}

// ServeObject 将对象写入响应；transform 为 true 时按 width/quality 缩放转码。
func (s *Service) ServeObject(
	w http.ResponseWriter,
	r *http.Request,
	objectKey string,
	width int,
	quality int,
	transform bool,
) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("图片服务未初始化")
	}

	if transform {
		payload, err := s.serveTransformed(r.Context(), objectKey, width, quality)
		if err != nil {
			return err
		}
		writeImageResponse(w, payload, s.cacheControl())
		return nil
	}

	data, err := s.store.GetImageObject(r.Context(), objectKey)
	if err != nil {
		return mapReadObjectError(err)
	}
	if len(data) == 0 {
		return fmt.Errorf("对象为空")
	}

	payload := servePayload{
		body:        data,
		contentType: detectContentType(data),
		etag:        hashETag(data),
	}
	writeImageResponse(w, payload, s.cacheControl())
	return nil
}

func (s *Service) serveTransformed(ctx context.Context, objectKey string, width, quality int) (servePayload, error) {
	key := fmt.Sprintf("%s:w%d:q%d", objectKey, width, quality)
	value, err, _ := s.group.Do(key, func() (any, error) {
		data, err := s.store.GetImageObject(ctx, objectKey)
		if err != nil {
			return servePayload{}, mapReadObjectError(err)
		}

		result, err := imageutil.Process(bytes.NewReader(data), imageutil.Options{
			MaxWidth:    width,
			WebPQuality: quality,
			Format:      imageutil.FormatWebP,
		})
		if err != nil {
			return servePayload{}, err
		}

		return servePayload{
			body:        result.Bytes,
			contentType: result.ContentType,
			etag:        "\"" + result.MD5 + "\"",
		}, nil
	})
	if err != nil {
		return servePayload{}, err
	}

	payload, ok := value.(servePayload)
	if !ok {
		return servePayload{}, fmt.Errorf("变换结果类型错误")
	}
	return payload, nil
}

func (s *Service) cacheControl() string {
	maxAge := s.cfg.ResponseCacheMaxAge
	if maxAge <= 0 {
		maxAge = 604800
	}
	return fmt.Sprintf("public, max-age=%d, immutable", maxAge)
}

func writeImageResponse(w http.ResponseWriter, payload servePayload, cacheControl string) {
	w.Header().Set("Content-Type", payload.contentType)
	w.Header().Set("Cache-Control", cacheControl)
	if payload.etag != "" {
		w.Header().Set("ETag", payload.etag)
	}
	_, _ = w.Write(payload.body)
}

func detectContentType(data []byte) string {
	contentType := http.DetectContentType(data)
	if contentType == "application/octet-stream" {
		return "image/webp"
	}
	return contentType
}

func hashETag(data []byte) string {
	sum := md5.Sum(data)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}

// ResetTransformGroup 仅供测试重置 singleflight 状态。
func (s *Service) ResetTransformGroup() {
	s.group = singleflight.Group{}
}

func mapReadObjectError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, storage.ErrObjectTooLarge) {
		return ErrSourceTooLarge
	}
	if storage.IsObjectNotFound(err) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}
