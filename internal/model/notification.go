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
	Type           string     `gorm:"size:40;not null;comment:事件类型 comment_created/reply_created/article_liked/system_notice" json:"type"`
	ActorUserID    *uint      `gorm:"index:idx_event_actor;comment:操作人，系统消息可为空" json:"actor_user_id"`
	SourceType     string     `gorm:"size:30;not null;index:idx_event_source,priority:1;comment:直接对象类型 comment/reply/article" json:"source_type"`
	SourceID       uint       `gorm:"index:idx_event_source,priority:2;comment:直接对象ID" json:"source_id"`
	RootType       string     `gorm:"size:30;not null;index:idx_event_root,priority:1;comment:根对象类型 moment/guestbook/article" json:"root_type"`
	RootID         uint       `gorm:"index:idx_event_root,priority:2;comment:根对象ID" json:"root_id"`
	Title          string     `gorm:"size:120;comment:事件标题快照" json:"title"`
	ContentExcerpt string     `gorm:"size:500;comment:内容摘要快照" json:"content_excerpt"`
	MetadataJSON   *string    `gorm:"type:json;comment:跳转、额外文案、对象快照等扩展信息" json:"metadata_json"`
	DispatchStatus string     `gorm:"size:20;not null;default:pending;index:idx_event_dispatch,priority:1;comment:分发状态 pending/processing/done/failed" json:"dispatch_status"`
	Attempts       int        `gorm:"not null;default:0;comment:分发尝试次数" json:"attempts"`
	NextProcessAt  time.Time  `gorm:"index:idx_event_dispatch,priority:2;comment:下次可处理时间" json:"next_process_at"`
	LeaseUntil     *time.Time `gorm:"type:datetime(6);index:idx_event_dispatch,priority:3;comment:worker 租约到期时间" json:"lease_until"`
	LockedBy       *string    `gorm:"size:80;comment:领取该事件的 worker 标识" json:"locked_by"`
	LastError      *string    `gorm:"size:1000;comment:最近一次分发错误" json:"last_error"`
}

func (NotificationEvent) TableName() string { return "notification_event" }

// NotificationInbox 记录某个用户是否收到某个事件，以及是否已读、是否删除。
// 同一事件可投递给多个用户，唯一约束保证幂等投递。
type NotificationInbox struct {
	Base
	EventID         uint       `gorm:"not null;uniqueIndex:uk_inbox_recipient_event,priority:2;comment:事件ID" json:"event_id"`
	RecipientUserID uint       `gorm:"not null;uniqueIndex:uk_inbox_recipient_event,priority:1;index:idx_inbox_recipient_read,priority:1;comment:接收人用户ID" json:"recipient_user_id"`
	IsRead          bool       `gorm:"type:tinyint;not null;default:0;index:idx_inbox_recipient_read,priority:2;comment:是否已读" json:"is_read"`
	ReadAt          *time.Time `gorm:"comment:已读时间" json:"read_at"`
	DeliveredAt     time.Time  `gorm:"comment:投递时间" json:"delivered_at"`
}

func (NotificationInbox) TableName() string { return "notification_inbox" }

// NotificationPreference 记录用户的细粒度通知偏好。
// 总开关沿用 user_setting.receive_mail，此表只描述更细的分类与摘要策略。
type NotificationPreference struct {
	timestamps
	UserID          uint    `gorm:"not null;uniqueIndex:uk_preference_user_event,priority:1;comment:用户ID" json:"user_id"`
	EventType       string  `gorm:"size:40;not null;default:'*';uniqueIndex:uk_preference_user_event,priority:2;comment:事件类型，* 表示默认" json:"event_type"`
	InAppEnabled    bool    `gorm:"type:tinyint;not null;default:1;comment:是否接收站内通知" json:"in_app_enabled"`
	EmailEnabled    bool    `gorm:"type:tinyint;not null;default:1;comment:是否接收邮件通知" json:"email_enabled"`
	EmailDigestMode string  `gorm:"size:20;not null;default:digest;comment:邮件摘要模式 off/digest/immediate_digest" json:"email_digest_mode"`
	QuietStart      *string `gorm:"size:5;comment:静默时段开始 HH:mm" json:"quiet_start"`
	QuietEnd        *string `gorm:"size:5;comment:静默时段结束 HH:mm" json:"quiet_end"`
}

func (NotificationPreference) TableName() string { return "notification_preference" }

// NotificationEmailTask 是邮件队列的最小单位，由 dispatcher 创建、planner 聚合。
// 一条 task 不是一封邮件，多条 task 由 planner 合成一个 batch。
type NotificationEmailTask struct {
	Base
	EventID         uint       `gorm:"not null;comment:事件ID" json:"event_id"`
	RecipientUserID uint       `gorm:"not null;index:idx_email_task_recipient;comment:接收人用户ID" json:"recipient_user_id"`
	ActorUserID     *uint      `gorm:"index:idx_email_task_actor;comment:操作人用户ID，用于 actor 限额" json:"actor_user_id"`
	ToEmail         string     `gorm:"size:155;not null;comment:发送目标邮箱快照" json:"to_email"`
	EventType       string     `gorm:"size:40;not null;comment:事件类型快照" json:"event_type"`
	Purpose         string     `gorm:"size:40;not null;comment:邮件用途 notification 等" json:"purpose"`
	Priority        int        `gorm:"not null;default:0;index:idx_email_task_pick,priority:4;comment:优先级，数字越小越优先" json:"priority"`
	Status          string     `gorm:"size:20;not null;default:pending;index:idx_email_task_pick,priority:1;comment:状态 pending/batched/sent/deferred/failed/skipped" json:"status"`
	AvailableAt     time.Time  `gorm:"index:idx_email_task_pick,priority:3;comment:最早可聚合时间" json:"available_at"`
	NextAttemptAt   time.Time  `gorm:"index:idx_email_task_pick,priority:2;comment:下次处理时间" json:"next_attempt_at"`
	Attempts        int        `gorm:"not null;default:0;comment:尝试次数" json:"attempts"`
	BatchID         *uint      `gorm:"comment:已归属的邮件批次ID" json:"batch_id"`
	LeaseUntil      *time.Time `gorm:"type:datetime(6);comment:worker 租约到期时间" json:"lease_until"`
	LockedBy        *string    `gorm:"size:80;comment:领取该任务的 worker 标识" json:"locked_by"`
	IdempotencyKey  string     `gorm:"size:120;not null;uniqueIndex:uk_email_task_idempotency;comment:幂等键，防止重复入队" json:"idempotency_key"`
	LastError       *string    `gorm:"size:1000;comment:最近一次处理错误" json:"last_error"`
}

func (NotificationEmailTask) TableName() string { return "notification_email_task" }

// NotificationEmailBatch 是最终要发送的一封邮件，包含多条 task。
type NotificationEmailBatch struct {
	Base
	RecipientUserID uint       `gorm:"not null;index:idx_email_batch_recipient;comment:接收人用户ID" json:"recipient_user_id"`
	ToEmail         string     `gorm:"size:155;not null;comment:收件邮箱快照" json:"to_email"`
	Purpose         string     `gorm:"size:40;not null;comment:邮件用途" json:"purpose"`
	Subject         string     `gorm:"size:180;not null;comment:邮件标题" json:"subject"`
	Status          string     `gorm:"size:20;not null;default:pending;index:idx_email_batch_pick,priority:1;comment:状态 pending/sending/sent/deferred/failed" json:"status"`
	ItemCount       int        `gorm:"not null;default:0;comment:包含任务数" json:"item_count"`
	ScheduledAt     time.Time  `gorm:"index:idx_email_batch_pick,priority:2;comment:计划发送时间" json:"scheduled_at"`
	SentAt          *time.Time `gorm:"comment:实际发送时间" json:"sent_at"`
	Attempts        int        `gorm:"not null;default:0;comment:尝试次数" json:"attempts"`
	LeaseUntil      *time.Time `gorm:"type:datetime(6);index:idx_email_batch_pick,priority:3;comment:worker 租约到期时间" json:"lease_until"`
	LockedBy        *string    `gorm:"size:80;comment:领取该批次的 worker 标识" json:"locked_by"`
	MessageID       *string    `gorm:"size:120;comment:邮件 Message-ID 或内部幂等 ID" json:"message_id"`
	LastError       *string    `gorm:"size:1000;comment:最近一次发送错误" json:"last_error"`
}

func (NotificationEmailBatch) TableName() string { return "notification_email_batch" }

// NotificationEmailBatchItem 连接 batch 与 task，保证一个 task 只属于一个 batch。
type NotificationEmailBatchItem struct {
	ID      uint `json:"id" gorm:"primarykey"`
	BatchID uint `gorm:"not null;uniqueIndex:uk_batch_item,priority:1;comment:邮件批次ID" json:"batch_id"`
	TaskID  uint `gorm:"not null;uniqueIndex:uk_batch_item,priority:2;uniqueIndex:uk_batch_task;comment:邮件任务ID" json:"task_id"`
}

func (NotificationEmailBatchItem) TableName() string { return "notification_email_batch_item" }

// EmailQuotaPolicy 按 purpose 配置邮件额度与频率，purpose 可扩展不写死。
type EmailQuotaPolicy struct {
	timestamps
	Purpose      string `gorm:"size:40;not null;uniqueIndex:uk_email_quota_policy_purpose;comment:邮件用途 register_code/password_reset/security/notification/admin_notice" json:"purpose"`
	DailyLimit   int    `gorm:"not null;default:0;comment:该 purpose 每日上限" json:"daily_limit"`
	ReservedMin  int    `gorm:"not null;default:0;comment:该 purpose 每日保底份额" json:"reserved_min"`
	Priority     int    `gorm:"not null;default:0;comment:全局优先级，数字越小越优先" json:"priority"`
	MaxPerMinute int    `gorm:"not null;default:0;comment:该 purpose 每分钟上限" json:"max_per_minute"`
	MaxPerHour   int    `gorm:"not null;default:0;comment:该 purpose 每小时上限" json:"max_per_hour"`
	Enabled      bool   `gorm:"type:tinyint;not null;default:1;comment:是否启用" json:"enabled"`
}

func (EmailQuotaPolicy) TableName() string { return "email_quota_policy" }

// EmailRoleQuotaPolicy 按角色限制操作人与接收人，管理员也必须有限额，只是更高。
type EmailRoleQuotaPolicy struct {
	timestamps
	Role       string `gorm:"size:30;not null;uniqueIndex:uk_role_quota,priority:1;comment:角色 normal/vip/admin" json:"role"`
	ScopeType  string `gorm:"size:20;not null;uniqueIndex:uk_role_quota,priority:2;comment:限额维度 actor/recipient" json:"scope_type"`
	DailyLimit int    `gorm:"not null;default:0;comment:每日上限" json:"daily_limit"`
	MaxPerHour int    `gorm:"not null;default:0;comment:每小时上限" json:"max_per_hour"`
	Enabled    bool   `gorm:"type:tinyint;not null;default:1;comment:是否启用" json:"enabled"`
}

func (EmailRoleQuotaPolicy) TableName() string { return "email_role_quota_policy" }

// EmailQuotaUsage 记录实际消耗，所有限额判断都以此表为准，服务重启后仍可继续限制。
type EmailQuotaUsage struct {
	timestamps
	QuotaDate   time.Time `gorm:"type:date;not null;comment:统计日期" json:"quota_date"`
	ScopeType   string    `gorm:"size:20;not null;uniqueIndex:uk_quota_usage,priority:1;comment:维度 site/purpose/actor/recipient" json:"scope_type"`
	ScopeID     uint      `gorm:"not null;default:0;uniqueIndex:uk_quota_usage,priority:2;comment:全站为0，用户维度为 user_id" json:"scope_id"`
	Purpose     string    `gorm:"size:40;not null;uniqueIndex:uk_quota_usage,priority:3;comment:邮件用途" json:"purpose"`
	WindowType  string    `gorm:"size:20;not null;uniqueIndex:uk_quota_usage,priority:4;comment:窗口类型 day/hour/minute" json:"window_type"`
	WindowStart time.Time `gorm:"not null;uniqueIndex:uk_quota_usage,priority:5;comment:统计窗口开始" json:"window_start"`
	UsedCount   int       `gorm:"not null;default:0;comment:已用数量" json:"used_count"`
}

func (EmailQuotaUsage) TableName() string { return "email_quota_usage" }

// EmailSendLog 记录每次真实发送尝试，仅追加，无更新与软删除。
type EmailSendLog struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	BatchID   *uint     `gorm:"comment:通知邮件批次ID" json:"batch_id"`
	Purpose   string    `gorm:"size:40;not null;comment:邮件用途" json:"purpose"`
	ToEmail   string    `gorm:"size:155;not null;comment:收件邮箱" json:"to_email"`
	Status    string    `gorm:"size:20;not null;comment:发送结果 success/failed" json:"status"`
	Provider  string    `gorm:"size:40;comment:邮件供应商" json:"provider"`
	MessageID *string   `gorm:"size:120;comment:邮件 Message-ID" json:"message_id"`
	Error     *string   `gorm:"size:1000;comment:发送错误" json:"error"`
	CreatedAt time.Time `json:"created_at"`
}

func (EmailSendLog) TableName() string { return "email_send_log" }
