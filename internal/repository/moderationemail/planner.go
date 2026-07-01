package moderationemail

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SkipStaleTasks 跳过修订已不再待审的有限批任务。
func (r *repository) SkipStaleTasks(ctx context.Context, limit int, now time.Time) error {
	return r.db.WithContext(ctx).Exec(`UPDATE moderation_review_email_task AS task
JOIN (
  SELECT candidate.id
  FROM moderation_review_email_task AS candidate
  JOIN moderation_revision AS revision ON revision.id = candidate.revision_id
  JOIN moderation_item AS item ON item.id = candidate.item_id
  WHERE candidate.status = ?
    AND (revision.review_status <> ? OR item.pending_revision_id IS NULL OR item.pending_revision_id <> candidate.revision_id)
  ORDER BY candidate.created_at, candidate.id
  LIMIT ?
) AS stale ON stale.id = task.id
SET task.status = ?, task.updated_at = ?
	WHERE task.status = ?`,
		model.ModerationReviewEmailTaskPending, model.ModerationReviewPending,
		boundedLimit(limit),
		model.ModerationReviewEmailTaskSkipped, now,
		model.ModerationReviewEmailTaskPending,
	).Error
}

// OldestPendingTask 返回最早可规划任务；不存在时返回 nil。
func (r *repository) OldestPendingTask(ctx context.Context) (*PendingTask, error) {
	var task PendingTask
	err := r.pendingTasks(ctx).
		Order("task.created_at,task.id").
		Limit(1).
		Take(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// LastSuccessfulSend 返回最近一次成功发送时间；从未发送时返回 nil。
func (r *repository) LastSuccessfulSend(ctx context.Context) (*time.Time, error) {
	var row struct{ SentAt time.Time }
	err := r.db.WithContext(ctx).Model(&model.ModerationReviewEmailBatch{}).
		Select("sent_at").
		Where("status = ? AND sent_at IS NOT NULL", model.ModerationReviewEmailBatchSent).
		Order("sent_at DESC,id DESC").
		Limit(1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row.SentAt, nil
}

// HasOpenBatch 判断是否已有待发或发送中的批次。
func (r *repository) HasOpenBatch(ctx context.Context) (bool, error) {
	var row struct{ ID uint64 }
	err := r.db.WithContext(ctx).Model(&model.ModerationReviewEmailBatch{}).
		Select("id").
		Where("status IN ?", []string{model.ModerationReviewEmailBatchPending, model.ModerationReviewEmailBatchSending}).
		Order("created_at,id").
		Limit(1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

// CreateBatch 锁定最早到期任务，在同一事务创建批次并绑定任务。
func (r *repository) CreateBatch(ctx context.Context, recipient AdminRecipient, limit int, now time.Time) (int, error) {
	created := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tasks []model.ModerationReviewEmailTask
		if err := tx.Table("moderation_review_email_task AS task").
			Select("task.id").
			Joins("JOIN moderation_revision AS revision ON revision.id = task.revision_id").
			Joins("JOIN moderation_item AS item ON item.id = task.item_id").
			Where("task.status = ? AND task.available_at <= ? AND task.next_attempt_at <= ?", model.ModerationReviewEmailTaskPending, now, now).
			Where("revision.review_status = ? AND item.pending_revision_id = task.revision_id", model.ModerationReviewPending).
			Order("task.created_at,task.id").
			Limit(boundedLimit(limit)).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Find(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}

		// 先锁定最早任务，再复查开放批次；并发规划会在同一任务上串行化。
		var openBatch struct{ ID uint64 }
		err := tx.Model(&model.ModerationReviewEmailBatch{}).
			Select("id").
			Where("status IN ?", []string{model.ModerationReviewEmailBatchPending, model.ModerationReviewEmailBatchSending}).
			Order("created_at,id").
			Limit(1).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&openBatch).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		batch := model.ModerationReviewEmailBatch{
			RecipientUserID: recipient.UserID,
			ToEmail:         recipient.Email,
			Subject:         fmt.Sprintf("待审核内容提醒（%d 条）", len(tasks)),
			Status:          model.ModerationReviewEmailBatchPending,
			ItemCount:       len(tasks),
			ScheduledAt:     now,
			NextAttemptAt:   now,
		}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		ids := make([]uint64, 0, len(tasks))
		for _, task := range tasks {
			ids = append(ids, task.ID)
		}
		result := tx.Model(&model.ModerationReviewEmailTask{}).
			Where("id IN ? AND status = ?", ids, model.ModerationReviewEmailTaskPending).
			Updates(map[string]any{"batch_id": batch.ID, "status": model.ModerationReviewEmailTaskBatched})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return fmt.Errorf("bind moderation email tasks: expected %d rows, updated %d", len(ids), result.RowsAffected)
		}
		created = len(ids)
		return nil
	})
	return created, err
}

func (r *repository) pendingTasks(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table("moderation_review_email_task AS task").
		Select("task.id,task.revision_id,task.item_id,item.content_type,item.author_id,revision.submitted_content,task.available_at,task.created_at").
		Joins("JOIN moderation_revision AS revision ON revision.id = task.revision_id").
		Joins("JOIN moderation_item AS item ON item.id = task.item_id").
		Where("task.status = ? AND revision.review_status = ? AND item.pending_revision_id = task.revision_id", model.ModerationReviewEmailTaskPending, model.ModerationReviewPending)
}
