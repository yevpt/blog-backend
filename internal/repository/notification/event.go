package notification

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
)

// CreateEvent 写入一条待分发事件。
func (r *repo) CreateEvent(ctx context.Context, event *model.NotificationEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

// LeasePendingEvents 领取可处理事件。
//
// 第一步用条件更新抢占：把 pending 行，以及租约已过期的 processing 行，
// 一次性置为 processing 并写入本 worker 的租约与标识；MySQL 的
// UPDATE ... ORDER BY ... LIMIT 保证只领取 limit 行。
// 第二步按 worker 标识与本次租约回读，得到确实归属本 worker 的事件。
func (r *repo) LeasePendingEvents(ctx context.Context, workerID string, leaseSeconds int, limit int) ([]model.NotificationEvent, error) {
	now := time.Now()
	leaseUntil := leaseUntilFrom(now, leaseSeconds)

	// 抢占：仅领取到点且租约空缺/过期的行。
	claim := r.db.WithContext(ctx).Model(&model.NotificationEvent{}).
		Where("dispatch_status IN ?", []string{EventStatusPending, EventStatusProcessing}).
		Where("next_process_at <= ?", now).
		Where("(lease_until IS NULL OR lease_until < ?)", now).
		Order("next_process_at").
		Limit(limit).
		Updates(map[string]any{
			"dispatch_status": EventStatusProcessing,
			"lease_until":     leaseUntil,
			"locked_by":       workerID,
		})
	if claim.Error != nil {
		return nil, claim.Error
	}
	if claim.RowsAffected == 0 {
		return nil, nil
	}

	// 回读本 worker 本次租约抢到的行。
	var events []model.NotificationEvent
	if err := r.db.WithContext(ctx).
		Where("locked_by = ? AND lease_until = ? AND dispatch_status = ?", workerID, leaseUntil, EventStatusProcessing).
		Order("next_process_at").
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// MarkEventDone 标记事件分发完成并释放租约。
func (r *repo) MarkEventDone(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.NotificationEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"dispatch_status": EventStatusDone,
			"lease_until":     nil,
			"locked_by":       nil,
		}).Error
}

// MarkEventRetry 分发失败时回退为 pending，设置下次处理时间与错误并释放租约。
func (r *repo) MarkEventRetry(ctx context.Context, id uint, nextProcessAt time.Time, lastErr string) error {
	return r.db.WithContext(ctx).Model(&model.NotificationEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"dispatch_status": EventStatusPending,
			"next_process_at": nextProcessAt,
			"last_error":      lastErr,
			"lease_until":     nil,
			"locked_by":       nil,
		}).Error
}
