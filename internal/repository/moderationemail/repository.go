// Package moderationemail 提供审核摘要邮件的持久化与接收人目录。
package moderationemail

import (
	"context"
	"errors"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// ErrRecipientUnavailable 表示配置用户不能作为审核邮件接收人。
var ErrRecipientUnavailable = errors.New("moderation email recipient unavailable")

// AdminRecipient 是创建批次时保存的接收人快照。
type AdminRecipient struct {
	UserID uint
	Email  string
}

// PendingTask 是邮件模板所需的待审内容快照。
type PendingTask struct {
	ID               uint64
	RevisionID       uint64
	ItemID           uint64
	ContentType      string
	AuthorID         uint64
	SubmittedContent string
	AvailableAt      time.Time
	CreatedAt        time.Time
}

// Repository 定义审核邮件规划与发送所需的持久化能力。
type Repository interface {
	Directory
	OldestPendingTask(ctx context.Context) (*PendingTask, error)
	SkipStaleTasks(ctx context.Context, limit int, now time.Time) error
	LastSuccessfulSend(ctx context.Context) (*time.Time, error)
	HasOpenBatch(ctx context.Context) (bool, error)
	CreateBatch(ctx context.Context, recipient AdminRecipient, limit int, now time.Time) (int, error)
	LeaseBatches(ctx context.Context, workerID string, leaseDuration time.Duration, limit int, now time.Time) ([]model.ModerationReviewEmailBatch, error)
	LoadBatchTasks(ctx context.Context, batchID uint64, limit int) ([]PendingTask, error)
	MarkBatchSent(ctx context.Context, batchID uint64, messageID string, now time.Time) error
	MarkBatchRetry(ctx context.Context, batchID uint64, messageID string, nextAttemptAt time.Time, lastErr string, now time.Time) error
}

// Directory 定义审核邮件接收人查询能力。
type Directory interface {
	LoadAdminRecipient(ctx context.Context, userID uint) (AdminRecipient, error)
}

type repository struct {
	db *gorm.DB
}

const maxLastErrorRunes = 1000

// NewRepository 创建审核邮件仓储。
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// NewDirectory 创建严格的管理员接收人目录。
func NewDirectory(db *gorm.DB) Directory {
	return &repository{db: db}
}

func boundedLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func mysqlDateTime3(value time.Time) time.Time {
	return value.Truncate(time.Millisecond)
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
