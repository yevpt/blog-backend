package model

import "time"

const (
	// ModerationReviewEmailTaskPending 表示任务等待聚合。
	ModerationReviewEmailTaskPending = "pending"
	// ModerationReviewEmailTaskBatched 表示任务已归入发送批次。
	ModerationReviewEmailTaskBatched = "batched"
	// ModerationReviewEmailTaskSent 表示任务所属邮件已发送。
	ModerationReviewEmailTaskSent = "sent"
	// ModerationReviewEmailTaskSkipped 表示修订已失效，无需提醒。
	ModerationReviewEmailTaskSkipped = "skipped"

	// ModerationReviewEmailBatchPending 表示批次等待发送。
	ModerationReviewEmailBatchPending = "pending"
	// ModerationReviewEmailBatchSending 表示批次已被 worker 租用。
	ModerationReviewEmailBatchSending = "sending"
	// ModerationReviewEmailBatchSent 表示批次发送成功。
	ModerationReviewEmailBatchSent = "sent"
	// ModerationReviewEmailBatchFailed 表示批次已停止重试。
	ModerationReviewEmailBatchFailed = "failed"
)

// ModerationReviewEmailTask 记录一个待审核修订的邮件聚合状态。
type ModerationReviewEmailTask struct {
	ID            uint64    `gorm:"primaryKey"`
	RevisionID    uint64    `gorm:"not null;uniqueIndex:uk_moderation_review_email_revision"`
	ItemID        uint64    `gorm:"not null;index:idx_moderation_review_email_task_item"`
	Status        string    `gorm:"size:16;not null;index:idx_moderation_review_email_task_pick,priority:1"`
	AvailableAt   time.Time `gorm:"type:datetime(3);not null"`
	NextAttemptAt time.Time `gorm:"type:datetime(3);not null;index:idx_moderation_review_email_task_pick,priority:2"`
	BatchID       *uint64   `gorm:"index:idx_moderation_review_email_task_batch"`
	CreatedAt     time.Time `gorm:"type:datetime(3);not null"`
	UpdatedAt     time.Time `gorm:"type:datetime(3);not null"`
}

// TableName 返回审核邮件任务表名。
func (ModerationReviewEmailTask) TableName() string { return "moderation_review_email_task" }

// ModerationReviewEmailBatch 记录一封可租用、可重试的审核摘要邮件。
type ModerationReviewEmailBatch struct {
	ID              uint64     `gorm:"primaryKey"`
	RecipientUserID uint       `gorm:"not null;index:idx_moderation_review_email_batch_recipient"`
	ToEmail         string     `gorm:"size:155;not null"`
	Subject         string     `gorm:"size:180;not null"`
	Status          string     `gorm:"size:16;not null;index:idx_moderation_review_email_batch_pick,priority:1"`
	ItemCount       int        `gorm:"not null;default:0"`
	ScheduledAt     time.Time  `gorm:"type:datetime(3);not null"`
	SentAt          *time.Time `gorm:"type:datetime(3)"`
	Attempts        int        `gorm:"not null;default:0"`
	NextAttemptAt   time.Time  `gorm:"type:datetime(3);not null;index:idx_moderation_review_email_batch_pick,priority:2"`
	LeaseUntil      *time.Time `gorm:"type:datetime(3);index:idx_moderation_review_email_batch_pick,priority:3"`
	LockedBy        *string    `gorm:"size:80"`
	MessageID       *string    `gorm:"size:120"`
	LastError       *string    `gorm:"size:1000"`
	CreatedAt       time.Time  `gorm:"type:datetime(3);not null"`
	UpdatedAt       time.Time  `gorm:"type:datetime(3);not null"`
}

// TableName 返回审核邮件批次表名。
func (ModerationReviewEmailBatch) TableName() string { return "moderation_review_email_batch" }
