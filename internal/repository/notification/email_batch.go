package notification

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// CreateEmailBatchWithItems 在事务内创建批次、批次条目，并把任务标记为 batched。
// 三步必须同事务：批次、连接行、任务归属保持一致，避免出现孤儿批次或漏归属任务。
func (r *repo) CreateEmailBatchWithItems(ctx context.Context, batch *model.NotificationEmailBatch, taskIDs []uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 创建批次，拿到自增 ID。
		if err := tx.Create(batch).Error; err != nil {
			return err
		}

		// 为每条任务建立批次连接行。
		items := make([]model.NotificationEmailBatchItem, 0, len(taskIDs))
		for _, taskID := range taskIDs {
			items = append(items, model.NotificationEmailBatchItem{BatchID: batch.ID, TaskID: taskID})
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}

		// 把任务标记为已归批，写入归属批次。
		if len(taskIDs) > 0 {
			if err := tx.Model(&model.NotificationEmailTask{}).
				Where("id IN ?", taskIDs).
				Updates(map[string]any{
					"status":   EmailTaskStatusBatched,
					"batch_id": batch.ID,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// LeaseEmailBatches 领取到点待发送的批次。
func (r *repo) LeaseEmailBatches(ctx context.Context, workerID string, leaseSeconds int, limit int) ([]model.NotificationEmailBatch, error) {
	now := time.Now()
	leaseUntil := leaseUntilFrom(now, leaseSeconds)

	claim := r.db.WithContext(ctx).Model(&model.NotificationEmailBatch{}).
		Where("status = ?", EmailBatchStatusPending).
		Where("scheduled_at <= ?", now).
		Where("(lease_until IS NULL OR lease_until < ?)", now).
		Order("scheduled_at").
		Limit(limit).
		Updates(map[string]any{
			"status":      EmailBatchStatusSending,
			"lease_until": leaseUntil,
			"locked_by":   workerID,
		})
	if claim.Error != nil {
		return nil, claim.Error
	}
	if claim.RowsAffected == 0 {
		return nil, nil
	}

	var batches []model.NotificationEmailBatch
	if err := r.db.WithContext(ctx).
		Where("locked_by = ? AND lease_until = ? AND status = ?", workerID, leaseUntil, EmailBatchStatusSending).
		Order("scheduled_at").
		Find(&batches).Error; err != nil {
		return nil, err
	}
	return batches, nil
}

// MarkBatchSent 标记批次及其任务发送成功并记录 message_id。
func (r *repo) MarkBatchSent(ctx context.Context, batchID uint, messageID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 批次置为 sent 并落发送时间与 message_id，释放租约。
		if err := tx.Model(&model.NotificationEmailBatch{}).
			Where("id = ?", batchID).
			Updates(map[string]any{
				"status":      EmailBatchStatusSent,
				"sent_at":     now,
				"message_id":  messageID,
				"lease_until": nil,
				"locked_by":   nil,
			}).Error; err != nil {
			return err
		}

		// 同步把该批次下的任务置为 sent。
		return tx.Model(&model.NotificationEmailTask{}).
			Where("batch_id = ?", batchID).
			Update("status", EmailTaskStatusSent).Error
	})
}

// ListBatchTasks 取某批次包含的全部邮件任务。
// 先从连接表取 task_id，再批量取任务，避免 JOIN 扫描的脆弱性。
func (r *repo) ListBatchTasks(ctx context.Context, batchID uint) ([]model.NotificationEmailTask, error) {
	var taskIDs []uint
	if err := r.db.WithContext(ctx).Model(&model.NotificationEmailBatchItem{}).
		Where("batch_id = ?", batchID).
		Pluck("task_id", &taskIDs).Error; err != nil {
		return nil, err
	}
	if len(taskIDs) == 0 {
		return nil, nil
	}
	var tasks []model.NotificationEmailTask
	if err := r.db.WithContext(ctx).Where("id IN ?", taskIDs).Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// CreateSendLog 追加一条真实发送尝试记录。
func (r *repo) CreateSendLog(ctx context.Context, log *model.EmailSendLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// MarkBatchRetry 发送失败时回退批次为 pending，设置下次发送时间与错误信息。
func (r *repo) MarkBatchRetry(ctx context.Context, batchID uint, scheduledAt time.Time, lastErr string) error {
	return r.db.WithContext(ctx).Model(&model.NotificationEmailBatch{}).
		Where("id = ?", batchID).
		Updates(map[string]any{
			"status":       EmailBatchStatusPending,
			"scheduled_at": scheduledAt,
			"attempts":     gorm.Expr("attempts + 1"),
			"last_error":   lastErr,
			"lease_until":  nil,
			"locked_by":    nil,
		}).Error
}
