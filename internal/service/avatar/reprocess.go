package avatar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
)

// objectGetter 从对象存储直读内容，绕过 CDN/HTTP。
type objectGetter interface {
	GetObject(ctx context.Context, objectName string) ([]byte, error)
}

// InspectStoredAvatar 检查已存储头像是否合规；第二个返回值非空表示无法继续归一化。
func InspectStoredAvatar(data []byte) (compliant bool, blockedReason string) {
	if len(data) == 0 {
		return false, "文件为空"
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return false, "无法解码为有效图片"
	}
	if strings.EqualFold(format, "gif") {
		return false, ""
	}
	if cfg.Width > defaultAvatarMaxSize || cfg.Height > defaultAvatarMaxSize {
		return false, ""
	}
	if len(data) > defaultAvatarMaxBytes {
		return false, ""
	}
	return true, ""
}

// FormatNormalizeIssue 生成管理端可读的头像处理失败说明。
func FormatNormalizeIssue(objectKey, reason string) string {
	objectKey = strings.TrimSpace(objectKey)
	reason = strings.TrimSpace(reason)
	if objectKey == "" {
		return reason
	}
	if reason == "" {
		return objectKey
	}
	return fmt.Sprintf("%s：%s", objectKey, reason)
}

// NormalizeFailureReason 将归一化错误转为管理端说明，避免直接暴露上传场景文案。
func NormalizeFailureReason(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrAvatarInvalid):
		return "无法解码为有效图片"
	case errors.Is(err, ErrAvatarGIFNotAllowed):
		return "不支持 GIF"
	case errors.Is(err, ErrAvatarCompressedTooLarge):
		return "压缩后仍超过 20KB"
	case errors.Is(err, ErrAvatarTooLarge):
		return "原始文件超过 256KB"
	default:
		return err.Error()
	}
}

// IsCompliantAvatarData 判断已存储头像是否满足当前压缩规范。
func IsCompliantAvatarData(data []byte) (bool, error) {
	compliant, blocked := InspectStoredAvatar(data)
	if blocked != "" {
		return false, fmt.Errorf("%w", ErrAvatarInvalid)
	}
	return compliant, nil
}

// LoadStoredAvatar 从对象存储读取本站托管头像内容（优先 S3 直读）。
func (s *Service) LoadStoredAvatar(ctx context.Context, objectKey string) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("对象存储不可用")
	}
	objectKey = strings.TrimSpace(objectKey)
	if !IsManagedAvatarKey(objectKey) {
		return nil, fmt.Errorf("头像 key 不在托管范围内")
	}
	exists, err := s.store.ObjectExists(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("头像对象不存在")
	}
	if getter, ok := s.store.(objectGetter); ok {
		return getter.GetObject(ctx, objectKey)
	}

	objectURL, err := s.store.ObjectURL(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(objectURL) == "" {
		return nil, fmt.Errorf("头像对象不存在")
	}
	return s.download(ctx, objectURL)
}

// ReprocessData 将原始头像字节压缩并写入对象存储。
func (s *Service) ReprocessData(ctx context.Context, data []byte) (SaveResult, error) {
	if s == nil || s.store == nil {
		return SaveResult{}, errors.New("对象存储不可用")
	}
	if len(data) == 0 {
		return SaveResult{}, ErrAvatarInvalid
	}
	if err := rejectGIFBytes(data); err != nil {
		return SaveResult{}, err
	}
	return s.compressAndStore(ctx, "avatar", data, false)
}

// ReprocessDataForNormalize 将原始头像字节压缩并写入对象存储（归一化专用，强制覆盖）。
func (s *Service) ReprocessDataForNormalize(ctx context.Context, data []byte) (SaveResult, error) {
	return s.ReprocessStoredAvatar(ctx, data, "")
}

// ReprocessStoredAvatar 将头像压缩后写入 targetKey；空 targetKey 时按内容 MD5 生成新 key。
func (s *Service) ReprocessStoredAvatar(ctx context.Context, data []byte, targetKey string) (SaveResult, error) {
	if s == nil || s.store == nil {
		return SaveResult{}, errors.New("对象存储不可用")
	}
	if len(data) == 0 {
		return SaveResult{}, ErrAvatarInvalid
	}
	return s.compressAndStoreAt(ctx, "avatar", data, targetKey)
}
