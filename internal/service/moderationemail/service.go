// Package moderationemail 规划并发送待审核内容摘要邮件。
package moderationemail

import (
	"context"
	"time"

	moderationemailrepo "github.com/vpt/blog-backend/internal/repository/moderationemail"
)

// Repository 定义审核邮件规划所需的最小持久化能力。
type Repository interface {
	SkipStaleTasks(ctx context.Context, limit int, now time.Time) error
	HasOpenBatch(ctx context.Context) (bool, error)
	OldestPendingTask(ctx context.Context) (*moderationemailrepo.PendingTask, error)
	LastSuccessfulSend(ctx context.Context) (*time.Time, error)
	CreateBatch(ctx context.Context, recipient moderationemailrepo.AdminRecipient, limit int, now time.Time) (int, error)
}

// Directory 查询审核邮件的管理员接收人。
type Directory interface {
	LoadAdminRecipient(ctx context.Context, userID uint) (moderationemailrepo.AdminRecipient, error)
}

// Config 定义审核邮件规划的接收人和最小发送间隔。
type Config struct {
	RecipientUserID uint
	MinInterval     time.Duration
}

// Planner 按聚合窗口与冷却间隔创建审核邮件批次。
type Planner struct {
	repo      Repository
	directory Directory
	cfg       Config
	now       func() time.Time
}

// NewPlanner 创建使用注入时钟的审核邮件规划器。
func NewPlanner(repo Repository, directory Directory, cfg Config, now func() time.Time) *Planner {
	return &Planner{repo: repo, directory: directory, cfg: cfg, now: now}
}
