package notification

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm/clause"
)

// CreateEmailTask 幂等入队邮件任务。
// 命中 uk_email_task_idempotency 唯一约束时 DoNothing，返回 created=false。
func (r *repo) CreateEmailTask(ctx context.Context, task *model.NotificationEmailTask) (bool, error) {
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(task)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// LeaseEmailTasks 领取可聚合的邮件任务。
// 与事件领取同构：抢占到点且租约空缺/过期的 pending 行，再按租约回读。
func (r *repo) LeaseEmailTasks(ctx context.Context, workerID string, leaseSeconds int, limit int) ([]model.NotificationEmailTask, error) {
	now := time.Now()
	leaseUntil := leaseUntilFrom(now, leaseSeconds)

	claim := r.db.WithContext(ctx).Model(&model.NotificationEmailTask{}).
		Where("status = ?", EmailTaskStatusPending).
		Where("next_attempt_at <= ?", now).
		Where("available_at <= ?", now).
		Where("(lease_until IS NULL OR lease_until < ?)", now).
		Order("priority, next_attempt_at").
		Limit(limit).
		Updates(map[string]any{
			"lease_until": leaseUntil,
			"locked_by":   workerID,
		})
	if claim.Error != nil {
		return nil, claim.Error
	}
	if claim.RowsAffected == 0 {
		return nil, nil
	}

	var tasks []model.NotificationEmailTask
	if err := r.db.WithContext(ctx).
		Where("locked_by = ? AND lease_until = ? AND status = ?", workerID, leaseUntil, EmailTaskStatusPending).
		Order("priority, next_attempt_at").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}
