// Package moderationmedia 负责审核图片的读取、指纹与低清预览准备。
package moderationmedia

import (
	"context"
	"errors"
	"time"

	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	// ErrInvalidImage 表示对象不是受支持图片或超过审核配置边界。
	ErrInvalidImage = errors.New("审核图片无效")
	// ErrImageUnavailable 表示对象存储或图片登记服务不可用。
	ErrImageUnavailable = errors.New("审核图片服务不可用")
)

// Fingerprint 是全站图片复用的最终身份。
type Fingerprint struct {
	SHA256 string
	MD5    string
	Size   uint64
}

// PendingImage 是需要登记或刷新为待审状态的图片事实。
type PendingImage struct {
	Fingerprint
	PreviewObjectKey string
	LastUsedAt       time.Time
}

// PreparedImage 是一个提交版本可安全持久化的有序图片快照。
type PreparedImage struct {
	Fingerprint
	ObjectKey        string
	MediaType        string
	IsGIF            bool
	PreviewObjectKey string
	Approved         bool
}

// PreparedSet 保留用户提交的图片顺序。
type PreparedSet struct {
	Images []PreparedImage
}

// Registry 隔离全站图片审核记录的数据访问。
type Registry interface {
	UseApproved(ctx context.Context, fingerprint Fingerprint, usedAt time.Time) (bool, error)
	UpsertPending(ctx context.Context, image PendingImage) error
}

type objectStore interface {
	storage.ObjectStore
	storage.ObjectReader
}

// Service 准备审核版本所需的图片事实。
type Service interface {
	Prepare(ctx context.Context, userID uint64, objectKeys []string) (PreparedSet, error)
}

type service struct {
	store    objectStore
	registry Registry
	cfg      config.ModerationImageConfig
	now      func() time.Time
}

// NewService 通过构造注入创建图片审核准备服务。
func NewService(store objectStore, registry Registry, cfg config.ModerationImageConfig, now func() time.Time) Service {
	if now == nil {
		now = time.Now
	}
	return &service{store: store, registry: registry, cfg: cfg, now: now}
}
