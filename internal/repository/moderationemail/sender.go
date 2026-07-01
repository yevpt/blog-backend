package moderationemail

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// LeaseBatches 领取到期批次，并恢复租约已过期的 sending 批次。
func (r *repository) LeaseBatches(ctx context.Context, workerID string, leaseDuration time.Duration, limit int, now time.Time) ([]model.ModerationReviewEmailBatch, error) {
	leaseUntil := mysqlDateTime3(now.Add(leaseDuration))
	claim := r.db.WithContext(ctx).Model(&model.ModerationReviewEmailBatch{}).
		Where("next_attempt_at <= ?", now).
		Where("status = ? OR (status = ? AND lease_until < ?)", model.ModerationReviewEmailBatchPending, model.ModerationReviewEmailBatchSending, now).
		Order("created_at,id").
		Limit(boundedLimit(limit)).
		Updates(map[string]any{
			"status":      model.ModerationReviewEmailBatchSending,
			"lease_until": leaseUntil,
			"locked_by":   workerID,
		})
	if claim.Error != nil || claim.RowsAffected == 0 {
		return nil, claim.Error
	}
	var batches []model.ModerationReviewEmailBatch
	err := r.db.WithContext(ctx).
		Where("locked_by = ? AND lease_until = ? AND status = ?", workerID, leaseUntil, model.ModerationReviewEmailBatchSending).
		Order("created_at,id").
		Limit(boundedLimit(limit)).
		Find(&batches).Error
	return batches, err
}

// LoadBatchTasks 按稳定顺序加载批次中的内容快照。
func (r *repository) LoadBatchTasks(ctx context.Context, batchID uint64, limit int) ([]PendingTask, error) {
	var tasks []PendingTask
	err := r.db.WithContext(ctx).Table("moderation_review_email_task AS task").
		Select("task.id,task.revision_id,task.item_id,item.content_type,item.author_id,revision.submitted_content,task.available_at,task.created_at").
		Joins("JOIN moderation_revision AS revision ON revision.id = task.revision_id").
		Joins("JOIN moderation_item AS item ON item.id = task.item_id").
		Where("task.batch_id = ? AND task.status = ?", batchID, model.ModerationReviewEmailTaskBatched).
		Order("task.created_at,task.id").
		Limit(boundedLimit(limit)).
		Find(&tasks).Error
	return tasks, err
}

// MarkBatchSent 在同一事务标记批次与其任务已发送。
func (r *repository) MarkBatchSent(ctx context.Context, batchID uint64, messageID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		batch := tx.Model(&model.ModerationReviewEmailBatch{}).
			Where("id = ? AND status = ?", batchID, model.ModerationReviewEmailBatchSending).
			Updates(map[string]any{
				"status":      model.ModerationReviewEmailBatchSent,
				"sent_at":     now,
				"message_id":  messageID,
				"lease_until": nil,
				"locked_by":   nil,
			})
		if batch.Error != nil {
			return batch.Error
		}
		if batch.RowsAffected != 1 {
			return fmt.Errorf("mark moderation email batch sent: updated %d rows", batch.RowsAffected)
		}
		return tx.Model(&model.ModerationReviewEmailTask{}).
			Where("batch_id = ? AND status = ?", batchID, model.ModerationReviewEmailTaskBatched).
			Updates(map[string]any{"status": model.ModerationReviewEmailTaskSent}).Error
	})
}

// MarkBatchRetry 保存稳定 Message-ID、错误摘要和下次重试时间，并释放租约。
func (r *repository) MarkBatchRetry(ctx context.Context, batchID uint64, messageID string, nextAttemptAt time.Time, lastErr string, now time.Time) error {
	lastErr = truncateRunes(lastErr, maxLastErrorRunes)
	result := r.db.WithContext(ctx).Model(&model.ModerationReviewEmailBatch{}).
		Where("id = ? AND status = ?", batchID, model.ModerationReviewEmailBatchSending).
		Updates(map[string]any{
			"status":          model.ModerationReviewEmailBatchPending,
			"attempts":        gorm.Expr("attempts + 1"),
			"next_attempt_at": nextAttemptAt,
			"message_id":      messageID,
			"last_error":      lastErr,
			"lease_until":     nil,
			"locked_by":       nil,
			"updated_at":      now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("mark moderation email batch retry: updated %d rows", result.RowsAffected)
	}
	return nil
}
