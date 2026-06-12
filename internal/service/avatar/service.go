package avatar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vpt/blog-backend/pkg/imageutil"
	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	// ErrRemoteAvatarInvalid 表示远程头像不是可接受的图片。
	ErrRemoteAvatarInvalid = errors.New("远程头像不是有效图片")
)

const (
	defaultTimeout           = 2 * time.Second
	defaultDownloadMaxBytes  = 2 << 20
	defaultAvatarMaxBytes    = 10 * 1024
	defaultAvatarMaxSize     = 120
	defaultAvatarJPEGQuality = 85
	defaultAvatarMinQuality  = 35
)

// Options 控制远程头像下载和压缩策略。
type Options struct {
	Timeout         time.Duration     // 下载和处理总超时
	MaxBytes        int64             // 远程响应最大读取字节数
	ImageOptions    imageutil.Options // 图片压缩参数，可按场景复用调整
	ObjectKeyPrefix string            // 对象 key 前缀，默认 avatar/user
	HTTPClient      *http.Client      // 可注入 HTTP client，测试或特殊网络环境使用
}

// Service 负责把远程头像保存为本站对象存储 key。
type Service struct {
	store storage.ObjectStore
	opts  Options
}

// NewService 创建头像保存服务。
func NewService(store storage.ObjectStore, opts Options) *Service {
	return &Service{store: store, opts: normalizeOptions(opts)}
}

// SaveRemoteAvatar 下载、校验、压缩并保存远程头像。
func (s *Service) SaveRemoteAvatar(ctx context.Context, avatarURL string) (string, error) {
	if s == nil || s.store == nil || strings.TrimSpace(avatarURL) == "" {
		return "", nil
	}

	// 使用独立超时包住头像链路，避免 OAuth 注册 callback 长时间等待第三方头像服务。
	if s.opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.opts.Timeout)
		defer cancel()
	}

	body, err := s.download(ctx, avatarURL)
	if err != nil {
		return "", err
	}

	result, err := imageutil.Process(bytes.NewReader(body), s.opts.ImageOptions)
	if err != nil {
		return "", err
	}

	objectName := strings.Trim(s.opts.ObjectKeyPrefix, "/") + "/" + result.MD5 + result.Ext
	exists, err := s.store.ObjectExists(ctx, objectName)
	if err == nil && exists {
		return objectName, nil
	}
	// 对象名由最终图片内容 MD5 生成，查重失败时重复上传同 key 仍然是幂等的。
	// 这样可以兼容只授予 PutObject、未授予 HeadObject 的 Garage/S3 凭证。
	if err := s.store.PutObject(ctx, objectName, result.Bytes, result.ContentType); err != nil {
		return "", err
	}
	return objectName, nil
}

func (s *Service) download(ctx context.Context, avatarURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "blog-backend-avatar")

	resp, err := s.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("远程头像下载失败: status=%d", resp.StatusCode)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.HasPrefix(contentType, "image/") {
		return nil, ErrRemoteAvatarInvalid
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, s.opts.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > s.opts.MaxBytes {
		return nil, ErrRemoteAvatarInvalid
	}
	return body, nil
}

func (s *Service) httpClient() *http.Client {
	if s.opts.HTTPClient != nil {
		return s.opts.HTTPClient
	}
	return http.DefaultClient
}

func normalizeOptions(opts Options) Options {
	if opts.Timeout == 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultDownloadMaxBytes
	}
	if opts.ObjectKeyPrefix == "" {
		opts.ObjectKeyPrefix = "avatar/user"
	}
	if opts.ImageOptions.Format == "" {
		opts.ImageOptions.Format = imageutil.FormatJPEG
	}
	if opts.ImageOptions.MaxWidth == 0 {
		opts.ImageOptions.MaxWidth = defaultAvatarMaxSize
	}
	if opts.ImageOptions.MaxHeight == 0 {
		opts.ImageOptions.MaxHeight = defaultAvatarMaxSize
	}
	if opts.ImageOptions.MaxBytes == 0 {
		opts.ImageOptions.MaxBytes = defaultAvatarMaxBytes
	}
	if opts.ImageOptions.JPEGQuality == 0 {
		opts.ImageOptions.JPEGQuality = defaultAvatarJPEGQuality
	}
	if opts.ImageOptions.MinJPEGQuality == 0 {
		opts.ImageOptions.MinJPEGQuality = defaultAvatarMinQuality
	}
	return opts
}
