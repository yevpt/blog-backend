// Package notification 提供通知系统 v2 的数据访问层。
//
// 所有可靠状态都落在 MySQL：事件、收件箱、邮件任务、邮件批次、额度用量。
// worker 领取任务采用「条件更新 + 租约回读」模式，不依赖 MySQL 8 的
// FOR UPDATE SKIP LOCKED，因此线上低版本 MySQL 也能安全抢占与恢复。
package notification

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// 事件分发状态。
const (
	EventStatusPending    = "pending"    // 待分发
	EventStatusProcessing = "processing" // 已被 worker 领取，处理中
	EventStatusDone       = "done"       // 已分发完成
	EventStatusFailed     = "failed"     // 分发失败，等待重试或人工介入
)

// 邮件任务状态。
const (
	EmailTaskStatusPending  = "pending"  // 待聚合
	EmailTaskStatusBatched  = "batched"  // 已归入批次
	EmailTaskStatusSent     = "sent"     // 已随批次发送
	EmailTaskStatusDeferred = "deferred" // 因额度不足延后
	EmailTaskStatusFailed   = "failed"   // 处理失败
	EmailTaskStatusSkipped  = "skipped"  // 规则判定无需发送
)

// 邮件批次状态。
const (
	EmailBatchStatusPending  = "pending"  // 待发送
	EmailBatchStatusSending  = "sending"  // 发送中
	EmailBatchStatusSent     = "sent"     // 已发送
	EmailBatchStatusDeferred = "deferred" // 因额度不足延后
	EmailBatchStatusFailed   = "failed"   // 发送失败
)

// InboxAggregate 收件箱条目与其事件快照的聚合，供 service 转换为 DTO。
type InboxAggregate struct {
	Inbox model.NotificationInbox // 收件状态
	Event model.NotificationEvent // 事件事实与展示快照
}

// InboxPage 收件箱分页结果，保持 repository 不返回 dto。
type InboxPage struct {
	Total int64            // 满足条件的总条数
	Items []InboxAggregate // 当前页条目
}

// QuotaUsageKey 唯一定位一条额度用量记录，对应 uk_quota_usage 唯一约束。
type QuotaUsageKey struct {
	QuotaDate   time.Time // 统计日期
	ScopeType   string    // 维度 site/purpose/actor/recipient
	ScopeID     uint      // 全站为 0，用户维度为 user_id
	Purpose     string    // 邮件用途
	WindowType  string    // 窗口类型 day/hour/minute
	WindowStart time.Time // 统计窗口开始
}

// EventRepository 通知事件的写入与 worker 领取。
type EventRepository interface {
	// CreateEvent 写入一条待分发事件。
	CreateEvent(ctx context.Context, event *model.NotificationEvent) error
	// LeasePendingEvents 领取可处理事件：抢占 pending 与租约过期的 processing 行，返回本 worker 持有的事件。
	LeasePendingEvents(ctx context.Context, workerID string, leaseSeconds int, limit int) ([]model.NotificationEvent, error)
	// MarkEventDone 标记事件分发完成并释放租约。
	MarkEventDone(ctx context.Context, id uint) error
	// MarkEventRetry 分发失败时回退为 pending，设置下次处理时间与错误信息并释放租约。
	MarkEventRetry(ctx context.Context, id uint, nextProcessAt time.Time, lastErr string) error
}

// InboxRepository 站内收件箱的投递与读取。
type InboxRepository interface {
	// CreateInbox 幂等投递站内通知，命中唯一约束时不重复插入，返回是否新建。
	CreateInbox(ctx context.Context, inbox *model.NotificationInbox) (created bool, err error)
	// ListInbox 分页查询某用户的收件箱，unreadOnly 为 true 时只返回未读。
	ListInbox(ctx context.Context, recipientID uint, unreadOnly bool, page int, pageSize int) (*InboxPage, error)
	// CountUnread 统计某用户的未读数量。
	CountUnread(ctx context.Context, recipientID uint) (int64, error)
	// MarkInboxRead 将某用户名下的单条通知置为已读，返回受影响行数。
	MarkInboxRead(ctx context.Context, recipientID uint, id uint) (int64, error)
	// MarkAllInboxRead 批量已读；ids 为空表示该用户全部未读，返回受影响行数。
	MarkAllInboxRead(ctx context.Context, recipientID uint, ids []uint) (int64, error)
	// DeleteInbox 软删除某用户名下的单条通知，返回受影响行数。
	DeleteInbox(ctx context.Context, recipientID uint, id uint) (int64, error)
}

// EmailTaskRepository 邮件任务的入队与 worker 领取。
type EmailTaskRepository interface {
	// CreateEmailTask 幂等入队邮件任务，命中 idempotency_key 唯一约束时不重复，返回是否新建。
	CreateEmailTask(ctx context.Context, task *model.NotificationEmailTask) (created bool, err error)
	// LeaseEmailTasks 领取可聚合的邮件任务，抢占 pending 与租约过期行。
	LeaseEmailTasks(ctx context.Context, workerID string, leaseSeconds int, limit int) ([]model.NotificationEmailTask, error)
	// DeferEmailTasks 将任务标记为 deferred 并设置下次处理时间，同时释放租约。
	DeferEmailTasks(ctx context.Context, ids []uint, nextAttemptAt time.Time) error
	// ReleaseEmailTasks 释放任务租约并保持 pending，供未到窗口或超出批次容量的任务下次重领。
	ReleaseEmailTasks(ctx context.Context, ids []uint) error
}

// EmailBatchRepository 邮件批次的生成、领取与发送结果落库。
type EmailBatchRepository interface {
	// CreateEmailBatchWithItems 在事务内创建批次、批次条目，并把任务标记为 batched。
	CreateEmailBatchWithItems(ctx context.Context, batch *model.NotificationEmailBatch, taskIDs []uint) error
	// LeaseEmailBatches 领取到点待发送的批次，抢占 pending 与租约过期行。
	LeaseEmailBatches(ctx context.Context, workerID string, leaseSeconds int, limit int) ([]model.NotificationEmailBatch, error)
	// MarkBatchSent 标记批次及其任务发送成功并记录 message_id。
	MarkBatchSent(ctx context.Context, batchID uint, messageID string) error
	// MarkBatchRetry 发送失败时回退批次为 pending，设置下次发送时间与错误信息。
	MarkBatchRetry(ctx context.Context, batchID uint, scheduledAt time.Time, lastErr string) error
}

// PreferenceRepository 用户通知偏好读取。
type PreferenceRepository interface {
	// GetPreference 读取用户对某事件类型的偏好；优先精确匹配，其次回退到 `*` 默认，均无则返回 nil。
	GetPreference(ctx context.Context, userID uint, eventType string) (*model.NotificationPreference, error)
}

// QuotaRepository 邮件额度策略读取与用量原子预留。
type QuotaRepository interface {
	// GetQuotaPolicies 读取全部 purpose 额度策略。
	GetQuotaPolicies(ctx context.Context) ([]model.EmailQuotaPolicy, error)
	// GetRoleQuotaPolicies 读取全部角色额度策略。
	GetRoleQuotaPolicies(ctx context.Context) ([]model.EmailRoleQuotaPolicy, error)
	// ReserveQuota 原子占用一次额度：用量未达 limit 时自增并返回 true，已达上限返回 false。
	ReserveQuota(ctx context.Context, key QuotaUsageKey, limit int) (bool, error)
	// GetUsage 读取某额度键当前已用计数，无记录返回 0。
	GetUsage(ctx context.Context, key QuotaUsageKey) (int, error)
}

// Repository 聚合通知系统全部数据访问能力，便于在 router 统一构造与注入。
type Repository interface {
	EventRepository
	InboxRepository
	EmailTaskRepository
	EmailBatchRepository
	PreferenceRepository
	QuotaRepository
}

type repo struct {
	db *gorm.DB
}

// NewRepository 创建通知仓储实例。
func NewRepository(db *gorm.DB) Repository {
	return &repo{db: db}
}

// leaseUntilFrom 由租约秒数计算到期时间，秒数非正时退化为当前时间。
func leaseUntilFrom(now time.Time, leaseSeconds int) time.Time {
	if leaseSeconds <= 0 {
		return now
	}
	return now.Add(time.Duration(leaseSeconds) * time.Second)
}
