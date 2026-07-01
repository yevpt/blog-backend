package avatar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/vpt/blog-backend/pkg/imagefile"
	"github.com/vpt/blog-backend/pkg/imageutil"
	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	// ErrRemoteAvatarInvalid 表示远程头像不是可接受的图片。
	ErrRemoteAvatarInvalid = errors.New("远程头像不是有效图片")
	// ErrAvatarInvalid 表示上传头像不是可接受的图片。
	ErrAvatarInvalid = errors.New("头像格式不支持，请上传 JPG、PNG 或 WebP")
	// ErrAvatarTooLarge 表示上传头像原始文件过大。
	ErrAvatarTooLarge = errors.New("头像不能超过 256KB")
	// ErrAvatarTooManyPixels 表示上传头像分辨率超过像素上限。
	ErrAvatarTooManyPixels = imagefile.ErrImageTooManyPixels
	// ErrAvatarGIFNotAllowed 表示不接受 GIF 头像。
	ErrAvatarGIFNotAllowed = errors.New("不支持 GIF 头像")
	// ErrAvatarCompressedTooLarge 表示头像压缩后仍超过限制。
	ErrAvatarCompressedTooLarge = errors.New("头像过大，请换一张更小的图片")
)

const (
	MaxRawAvatarBytes        = 256 * 1024
	defaultTimeout           = 2 * time.Second
	defaultDownloadMaxBytes  = MaxRawAvatarBytes
	defaultAvatarMaxBytes    = 20 * 1024
	defaultAvatarMaxSize     = 240
	defaultAvatarWebPQuality = 85
	defaultAvatarMinQuality  = 35
	avatarObjectPrefix       = "avatar/user"
)

// SaveResult 表示头像保存到对象存储后的结果。
type SaveResult struct {
	ObjectKey string
	Created   bool
}

// Options 控制远程头像下载和压缩策略。
type Options struct {
	Timeout         time.Duration     // 下载和处理总超时
	MaxBytes        int64             // 远程响应最大读取字节数
	ImageOptions    imageutil.Options // 图片压缩参数，可按场景复用调整
	ObjectKeyPrefix string            // 对象 key 前缀，默认 avatar/user
	HTTPClient      *http.Client      // 可注入 HTTP client，测试或特殊网络环境使用
}

// Service 负责把远程或本地上传头像保存为本站对象存储 key。
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

	if s.opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.opts.Timeout)
		defer cancel()
	}

	body, err := s.download(ctx, avatarURL)
	if err != nil {
		return "", err
	}
	if err := rejectGIFBytes(body); err != nil {
		return "", err
	}

	result, err := s.compressAndStore(ctx, "remote-avatar", body, false)
	if err != nil {
		return "", err
	}
	return result.ObjectKey, nil
}

// SaveUploadedAvatar 校验、压缩并保存本地上传头像。
func (s *Service) SaveUploadedAvatar(ctx context.Context, name string, data []byte) (SaveResult, error) {
	if s == nil || s.store == nil {
		return SaveResult{}, errors.New("对象存储不可用")
	}
	if len(data) == 0 {
		return SaveResult{}, ErrAvatarInvalid
	}

	validated, err := validateUploadedAvatar(name, data)
	if err != nil {
		return SaveResult{}, err
	}
	return s.compressAndStore(ctx, name, validated.Data, false)
}

func (s *Service) compressAndStore(ctx context.Context, name string, data []byte, forcePut bool) (SaveResult, error) {
	stored, err := imagefile.PrepareForStorage(name, data, avatarPrepareOptions(s.opts))
	if err != nil {
		return SaveResult{}, mapProcessErr(err)
	}
	if s.opts.ImageOptions.MaxBytes > 0 && len(stored.Data) > s.opts.ImageOptions.MaxBytes {
		return SaveResult{}, ErrAvatarCompressedTooLarge
	}

	objectName := strings.Trim(s.opts.ObjectKeyPrefix, "/") + "/" + stored.MD5 + stored.Ext
	if !forcePut {
		exists, err := s.store.ObjectExists(ctx, objectName)
		if err == nil && exists {
			return SaveResult{ObjectKey: objectName, Created: false}, nil
		}
	}
	if err := s.store.PutObject(ctx, objectName, stored.Data, stored.ContentType); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{ObjectKey: objectName, Created: true}, nil
}

func (s *Service) compressAndStoreAt(ctx context.Context, name string, data []byte, targetKey string) (SaveResult, error) {
	stored, err := imagefile.PrepareForStorage(name, data, avatarPrepareOptions(s.opts))
	if err != nil {
		return SaveResult{}, mapProcessErr(err)
	}
	if s.opts.ImageOptions.MaxBytes > 0 && len(stored.Data) > s.opts.ImageOptions.MaxBytes {
		return SaveResult{}, ErrAvatarCompressedTooLarge
	}

	objectName := outputKeyForNormalize(targetKey, stored)
	if err := s.store.PutObject(ctx, objectName, stored.Data, stored.ContentType); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{ObjectKey: objectName, Created: true}, nil
}

func avatarPrepareOptions(opts Options) imagefile.PrepareOptions {
	return imagefile.PrepareOptions{
		MaxStoredBytes: opts.ImageOptions.MaxBytes,
		MaxWidth:       opts.ImageOptions.MaxWidth,
		MaxHeight:      opts.ImageOptions.MaxHeight,
		WebPQuality:    opts.ImageOptions.WebPQuality,
		MinWebPQuality: opts.ImageOptions.MinWebPQuality,
	}
}

func outputKeyForNormalize(targetKey string, stored imagefile.Result) string {
	targetKey = strings.TrimLeft(strings.TrimSpace(targetKey), "/")
	prefix := avatarObjectPrefix
	if targetKey == "" || !IsManagedAvatarKey(targetKey) {
		return strings.Trim(prefix, "/") + "/" + stored.MD5 + stored.Ext
	}
	currentExt := filepath.Ext(targetKey)
	if currentExt != "" && !strings.EqualFold(currentExt, stored.Ext) {
		return strings.TrimSuffix(targetKey, currentExt) + stored.Ext
	}
	return targetKey
}

func validateUploadedAvatar(name string, data []byte) (imagefile.Result, error) {
	result, err := imagefile.Validate(name, data, MaxRawAvatarBytes)
	if err != nil {
		return imagefile.Result{}, mapValidateErr(err)
	}
	if isGIFResult(result) {
		return imagefile.Result{}, ErrAvatarGIFNotAllowed
	}
	return result, nil
}

func rejectGIFBytes(data []byte) error {
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ErrRemoteAvatarInvalid
	}
	if strings.EqualFold(format, "gif") {
		return ErrAvatarGIFNotAllowed
	}
	return nil
}

func isGIFResult(result imagefile.Result) bool {
	return result.ContentType == "image/gif" || strings.EqualFold(result.Ext, ".gif")
}

func mapValidateErr(err error) error {
	switch {
	case errors.Is(err, imagefile.ErrImageTooLarge):
		return ErrAvatarTooLarge
	case errors.Is(err, imagefile.ErrImageTooManyPixels):
		return ErrAvatarTooManyPixels
	case errors.Is(err, imagefile.ErrInvalidImage):
		return ErrAvatarInvalid
	default:
		return ErrAvatarInvalid
	}
}

func mapProcessErr(err error) error {
	switch {
	case errors.Is(err, imageutil.ErrInvalidImage), errors.Is(err, imageutil.ErrUnsupportedFormat):
		return ErrAvatarInvalid
	case errors.Is(err, imagefile.ErrImageTooManyPixels):
		return ErrAvatarTooManyPixels
	case errors.Is(err, imageutil.ErrImageTooLarge):
		return ErrAvatarCompressedTooLarge
	default:
		return err
	}
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
		opts.ObjectKeyPrefix = avatarObjectPrefix
	}
	if opts.ImageOptions.Format == "" {
		opts.ImageOptions.Format = imageutil.FormatWebP
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
	if opts.ImageOptions.WebPQuality == 0 {
		opts.ImageOptions.WebPQuality = defaultAvatarWebPQuality
	}
	if opts.ImageOptions.MinWebPQuality == 0 {
		opts.ImageOptions.MinWebPQuality = defaultAvatarMinQuality
	}
	return opts
}

// IsManagedAvatarKey 判断对象 key 是否属于本站托管头像前缀。
func IsManagedAvatarKey(key string) bool {
	key = strings.TrimSpace(key)
	return key != "" && strings.HasPrefix(key, avatarObjectPrefix+"/")
}
