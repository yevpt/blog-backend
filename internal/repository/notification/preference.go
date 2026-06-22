package notification

import (
	"context"
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// GetPreference 读取用户对某事件类型的偏好。
// 先按精确 event_type 命中，未命中再回退到 `*` 默认行；都没有时返回 nil，由上层用默认值。
func (r *repo) GetPreference(ctx context.Context, userID uint, eventType string) (*model.NotificationPreference, error) {
	// 精确匹配该事件类型。
	if pref, err := r.takePreference(ctx, userID, eventType); err != nil {
		return nil, err
	} else if pref != nil {
		return pref, nil
	}

	// 回退到通配默认偏好。
	return r.takePreference(ctx, userID, "*")
}

// takePreference 取单条偏好，未命中返回 (nil, nil)。
func (r *repo) takePreference(ctx context.Context, userID uint, eventType string) (*model.NotificationPreference, error) {
	var pref model.NotificationPreference
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND event_type = ?", userID, eventType).
		Take(&pref).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pref, nil
}
