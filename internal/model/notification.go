package model

import "time"

// 通知系统 v2 表结构。
//
// 事实源全部落在 MySQL：事件、收件箱、邮件任务、邮件批次、额度用量、发送日志。
// 这些模型只描述持久结构，不直接返回给前端，对外一律走 dto。
//
// 软删除：事件、收件箱、邮件任务、邮件批次内嵌 Base（含 DeletedAt）；
// 偏好、额度策略、用量、发送日志等运营/审计表无需软删除，使用显式时间字段。

// timestamps 是无软删除表的公共时间字段，对应建表的 created_at/updated_at。
type timestamps struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NotificationEvent 记录一件已经发生的通知事实，例如「B 评论了 A 的碎语」。
// 事件不决定邮件如何聚合，聚合属于 planner 的运行时策略。
type NotificationEvent struct {
	Base
	Type           string     `gorm:"size:40;not null" json:"type"`
	ActorUserID    *uint      `gorm:"index:idx_event_actor" json:"actor_user_id"`
	SourceType     string     `gorm:"size:30;not null;index:idx_event_source,priority:1" json:"source_type"`
	SourceID       uint       `gorm:"index:idx_event_source,priority:2" json:"source_id"`
	RootType       string     `gorm:"size:30;not null;index:idx_event_root,priority:1" json:"root_type"`
	RootID         uint       `gorm:"index:idx_event_root,priority:2" json:"root_id"`
	Title          string     `gorm:"size:120" json:"title"`
	ContentExcerpt string     `gorm:"size:500" json:"content_excerpt"`
	MetadataJSON   *string    `gorm:"type:json" json:"metadata_json"`
	DispatchStatus string     `gorm:"size:20;not null;default:pending;index:idx_event_dispatch,priority:1" json:"dispatch_status"`
	Attempts       int        `gorm:"not null;default:0" json:"attempts"`
	NextProcessAt  time.Time  `gorm:"index:idx_event_dispatch,priority:2" json:"next_process_at"`
	LeaseUntil     *time.Time `gorm:"type:datetime(6);index:idx_event_dispatch,priority:3" json:"lease_until"`
	LockedBy       *string    `gorm:"size:80" json:"locked_by"`
	LastError      *string    `gorm:"size:1000" json:"last_error"`
}

func (NotificationEvent) TableName() string { return "notification_event" }

// NotificationInbox 记录某个用户是否收到某个事件，以及是否已读、是否删除。
// 同一事件可投递给多个用户，唯一约束保证幂等投递。
type NotificationInbox struct {
	Base
	EventID         uint       `gorm:"not null;uniqueIndex:uk_inbox_recipient_event,priority:2" json:"event_id"`
	RecipientUserID uint       `gorm:"not null;uniqueIndex:uk_inbox_recipient_event,priority:1;index:idx_inbox_recipient_read,priority:1" json:"recipient_user_id"`
	IsRead          bool       `gorm:"type:tinyint;not null;default:0;index:idx_inbox_recipient_read,priority:2" json:"is_read"`
	ReadAt          *time.Time `json:"read_at"`
	DeliveredAt     time.Time  `json:"delivered_at"`
}

func (NotificationInbox) TableName() string { return "notification_inbox" }

// NotificationPreference 记录用户的细粒度通知偏好。
// 总开关沿用 user_setting.receive_mail，此表只描述更细的分类与摘要策略。
type NotificationPreference struct {
	timestamps
	UserID          uint    `gorm:"not null;uniqueIndex:uk_preference_user_event,priority:1" json:"user_id"`
	EventType       string  `gorm:"size:40;not null;default:'*';uniqueIndex:uk_preference_user_event,priority:2" json:"event_type"`
	InAppEnabled    bool    `gorm:"type:tinyint;not null;default:1" json:"in_app_enabled"`
	EmailEnabled    bool    `gorm:"type:tinyint;not null;default:1" json:"email_enabled"`
	EmailDigestMode string  `gorm:"size:20;not null;default:digest" json:"email_digest_mode"`
	QuietStart      *string `gorm:"size:5" json:"quiet_start"`
	QuietEnd        *string `gorm:"size:5" json:"quiet_end"`
}

func (NotificationPreference) TableName() string { return "notification_preference" }

// NotificationEmailTask 是邮件队列的最小单位，由 dispatcher 创建、planner 聚合。
// 一条 task 不是一封邮件，多条 task 由 planner 合成一个 batch。
type NotificationEmailTask struct {
	Base
	EventID         uint       `gorm:"not null" json:"event_id"`
	RecipientUserID uint       `gorm:"not null;index:idx_email_task_recipient" json:"recipient_user_id"`
	ActorUserID     *uint      `gorm:"index:idx_email_task_actor" json:"actor_user_id"`
	ToEmail         string     `gorm:"size:155;not null" json:"to_email"`
	EventType       string     `gorm:"size:40;not null" json:"event_type"`
	Purpose         string     `gorm:"size:40;not null" json:"purpose"`
	Priority        int        `gorm:"not null;default:0;index:idx_email_task_pick,priority:4" json:"priority"`
	Status          string     `gorm:"size:20;not null;default:pending;index:idx_email_task_pick,priority:1" json:"status"`
	AvailableAt     time.Time  `gorm:"index:idx_email_task_pick,priority:3" json:"available_at"`
	NextAttemptAt   time.Time  `gorm:"index:idx_email_task_pick,priority:2" json:"next_attempt_at"`
	Attempts        int        `gorm:"not null;default:0" json:"attempts"`
	BatchID         *uint      `json:"batch_id"`
	LeaseUntil      *time.Time `gorm:"type:datetime(6)" json:"lease_until"`
	LockedBy        *string    `gorm:"size:80" json:"locked_by"`
	IdempotencyKey  string     `gorm:"size:120;not null;uniqueIndex:uk_email_task_idempotency" json:"idempotency_key"`
	LastError       *string    `gorm:"size:1000" json:"last_error"`
}

func (NotificationEmailTask) TableName() string { return "notification_email_task" }

// NotificationEmailBatch 是最终要发送的一封邮件，包含多条 task。
type NotificationEmailBatch struct {
	Base
	RecipientUserID uint       `gorm:"not null;index:idx_email_batch_recipient" json:"recipient_user_id"`
	ToEmail         string     `gorm:"size:155;not null" json:"to_email"`
	Purpose         string     `gorm:"size:40;not null" json:"purpose"`
	Subject         string     `gorm:"size:180;not null" json:"subject"`
	Status          string     `gorm:"size:20;not null;default:pending;index:idx_email_batch_pick,priority:1" json:"status"`
	ItemCount       int        `gorm:"not null;default:0" json:"item_count"`
	ScheduledAt     time.Time  `gorm:"index:idx_email_batch_pick,priority:2" json:"scheduled_at"`
	SentAt          *time.Time `json:"sent_at"`
	Attempts        int        `gorm:"not null;default:0" json:"attempts"`
	LeaseUntil      *time.Time `gorm:"type:datetime(6);index:idx_email_batch_pick,priority:3" json:"lease_until"`
	LockedBy        *string    `gorm:"size:80" json:"locked_by"`
	MessageID       *string    `gorm:"size:120" json:"message_id"`
	LastError       *string    `gorm:"size:1000" json:"last_error"`
}

func (NotificationEmailBatch) TableName() string { return "notification_email_batch" }

// NotificationEmailBatchItem 连接 batch 与 task，保证一个 task 只属于一个 batch。
type NotificationEmailBatchItem struct {
	ID      uint `json:"id" gorm:"primarykey"`
	BatchID uint `gorm:"not null;uniqueIndex:uk_batch_item,priority:1" json:"batch_id"`
	TaskID  uint `gorm:"not null;uniqueIndex:uk_batch_item,priority:2;uniqueIndex:uk_batch_task" json:"task_id"`
}

func (NotificationEmailBatchItem) TableName() string { return "notification_email_batch_item" }

// EmailQuotaPolicy 按 purpose 配置邮件额度与频率，purpose 可扩展不写死。
type EmailQuotaPolicy struct {
	timestamps
	Purpose      string `gorm:"size:40;not null;uniqueIndex:uk_email_quota_policy_purpose" json:"purpose"`
	DailyLimit   int    `gorm:"not null;default:0" json:"daily_limit"`
	ReservedMin  int    `gorm:"not null;default:0" json:"reserved_min"`
	Priority     int    `gorm:"not null;default:0" json:"priority"`
	MaxPerMinute int    `gorm:"not null;default:0" json:"max_per_minute"`
	MaxPerHour   int    `gorm:"not null;default:0" json:"max_per_hour"`
	Enabled      bool   `gorm:"type:tinyint;not null;default:1" json:"enabled"`
}

func (EmailQuotaPolicy) TableName() string { return "email_quota_policy" }

// EmailRoleQuotaPolicy 按角色限制操作人与接收人，管理员也必须有限额，只是更高。
type EmailRoleQuotaPolicy struct {
	timestamps
	Role       string `gorm:"size:30;not null;uniqueIndex:uk_role_quota,priority:1" json:"role"`
	ScopeType  string `gorm:"size:20;not null;uniqueIndex:uk_role_quota,priority:2" json:"scope_type"`
	DailyLimit int    `gorm:"not null;default:0" json:"daily_limit"`
	MaxPerHour int    `gorm:"not null;default:0" json:"max_per_hour"`
	Enabled    bool   `gorm:"type:tinyint;not null;default:1" json:"enabled"`
}

func (EmailRoleQuotaPolicy) TableName() string { return "email_role_quota_policy" }

// EmailQuotaUsage 记录实际消耗，所有限额判断都以此表为准，服务重启后仍可继续限制。
type EmailQuotaUsage struct {
	timestamps
	QuotaDate   time.Time `gorm:"type:date;not null" json:"quota_date"`
	ScopeType   string    `gorm:"size:20;not null;uniqueIndex:uk_quota_usage,priority:1" json:"scope_type"`
	ScopeID     uint      `gorm:"not null;default:0;uniqueIndex:uk_quota_usage,priority:2" json:"scope_id"`
	Purpose     string    `gorm:"size:40;not null;uniqueIndex:uk_quota_usage,priority:3" json:"purpose"`
	WindowType  string    `gorm:"size:20;not null;uniqueIndex:uk_quota_usage,priority:4" json:"window_type"`
	WindowStart time.Time `gorm:"not null;uniqueIndex:uk_quota_usage,priority:5" json:"window_start"`
	UsedCount   int       `gorm:"not null;default:0" json:"used_count"`
}

func (EmailQuotaUsage) TableName() string { return "email_quota_usage" }

// EmailSendLog 记录每次真实发送尝试，仅追加，无更新与软删除。
type EmailSendLog struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	BatchID   *uint     `json:"batch_id"`
	Purpose   string    `gorm:"size:40;not null" json:"purpose"`
	ToEmail   string    `gorm:"size:155;not null" json:"to_email"`
	Status    string    `gorm:"size:20;not null" json:"status"`
	Provider  string    `gorm:"size:40" json:"provider"`
	MessageID *string   `gorm:"size:120" json:"message_id"`
	Error     *string   `gorm:"size:1000" json:"error"`
	CreatedAt time.Time `json:"created_at"`
}

func (EmailSendLog) TableName() string { return "email_send_log" }
