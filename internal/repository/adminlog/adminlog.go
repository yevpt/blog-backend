package adminlog

import (
	"context"

	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
)

// Repository 管理员操作日志的持久化接口。
type Repository interface {
	Create(ctx context.Context, entry *model.AdminOperationLog) error
	ListByTargetUser(ctx context.Context, targetUserID uint, offset, limit int) ([]model.AdminOperationLog, int64, error)
}

type repo struct {
	db *gorm.DB
}

// NewRepository 创建管理员操作日志仓储。
func NewRepository(db *gorm.DB) Repository {
	return &repo{db: db}
}

func (r *repo) Create(ctx context.Context, entry *model.AdminOperationLog) error {
	return r.db.WithContext(ctx).Create(entry).Error
}

func (r *repo) ListByTargetUser(ctx context.Context, targetUserID uint, offset, limit int) ([]model.AdminOperationLog, int64, error) {
	var total int64
	query := r.db.WithContext(ctx).Model(&model.AdminOperationLog{}).Where("target_user_id = ?", targetUserID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []model.AdminOperationLog
	err := query.Order("created_at DESC, id DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}
