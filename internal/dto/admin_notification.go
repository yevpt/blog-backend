package dto

import "time"

// AdminNotificationListReq 管理端邮件任务/批次的分页查询参数。
type AdminNotificationListReq struct {
	// Page 页码，从 1 开始。
	Page int `form:"page" binding:"omitempty,min=1" example:"1"`
	// PageSize 每页数量，最大 50。
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50" example:"10"`
	// Status 状态过滤，留空表示全部。
	Status string `form:"status" example:"pending"`
}

// AdminEmailTaskResp 管理端邮件任务条目。
type AdminEmailTaskResp struct {
	ID              uint      `json:"id" example:"1"`
	EventID         uint      `json:"event_id" example:"10"`
	RecipientUserID uint      `json:"recipient_user_id" example:"5"`
	ActorUserID     *uint     `json:"actor_user_id,omitempty" example:"2"`
	ToEmail         string    `json:"to_email" example:"owner@example.com"`
	EventType       string    `json:"event_type" example:"comment_created"`
	Purpose         string    `json:"purpose" example:"notification"`
	Status          string    `json:"status" example:"pending"`
	Attempts        int       `json:"attempts" example:"0"`
	BatchID         *uint     `json:"batch_id,omitempty" example:"3"`
	CreatedAt       time.Time `json:"created_at"`
}

// AdminEmailTaskPageResp 邮件任务分页响应。
type AdminEmailTaskPageResp struct {
	Total    int64                `json:"total" example:"42"`
	Page     int                  `json:"page" example:"1"`
	PageSize int                  `json:"page_size" example:"10"`
	List     []AdminEmailTaskResp `json:"list"`
}

// AdminEmailBatchResp 管理端邮件批次条目。
type AdminEmailBatchResp struct {
	ID              uint       `json:"id" example:"1"`
	RecipientUserID uint       `json:"recipient_user_id" example:"5"`
	ToEmail         string     `json:"to_email" example:"owner@example.com"`
	Purpose         string     `json:"purpose" example:"notification"`
	Subject         string     `json:"subject" example:"你有 2 条新通知"`
	Status          string     `json:"status" example:"pending"`
	ItemCount       int        `json:"item_count" example:"2"`
	Attempts        int        `json:"attempts" example:"0"`
	ScheduledAt     time.Time  `json:"scheduled_at"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// AdminEmailBatchPageResp 邮件批次分页响应。
type AdminEmailBatchPageResp struct {
	Total    int64                 `json:"total" example:"12"`
	Page     int                   `json:"page" example:"1"`
	PageSize int                   `json:"page_size" example:"10"`
	List     []AdminEmailBatchResp `json:"list"`
}

// AdminQuotaPolicyResp purpose 额度策略。
type AdminQuotaPolicyResp struct {
	ID           uint   `json:"id" example:"1"`
	Purpose      string `json:"purpose" example:"notification"`
	DailyLimit   int    `json:"daily_limit" example:"150"`
	ReservedMin  int    `json:"reserved_min" example:"0"`
	Priority     int    `json:"priority" example:"100"`
	MaxPerMinute int    `json:"max_per_minute" example:"5"`
	MaxPerHour   int    `json:"max_per_hour" example:"80"`
	Enabled      bool   `json:"enabled" example:"true"`
}

// AdminRoleQuotaPolicyResp 角色额度策略。
type AdminRoleQuotaPolicyResp struct {
	ID         uint   `json:"id" example:"1"`
	Role       string `json:"role" example:"normal"`
	ScopeType  string `json:"scope_type" example:"actor"`
	DailyLimit int    `json:"daily_limit" example:"30"`
	MaxPerHour int    `json:"max_per_hour" example:"0"`
	Enabled    bool   `json:"enabled" example:"true"`
}

// AdminQuotaListResp 额度策略汇总响应。
type AdminQuotaListResp struct {
	Purposes []AdminQuotaPolicyResp     `json:"purposes"`
	Roles    []AdminRoleQuotaPolicyResp `json:"roles"`
}

// AdminUpdateQuotaReq 调整 purpose 额度策略；所有数值非负且有上限保护。
type AdminUpdateQuotaReq struct {
	DailyLimit   int   `json:"daily_limit" binding:"min=0,max=100000" example:"150"`
	ReservedMin  int   `json:"reserved_min" binding:"min=0,max=100000" example:"0"`
	Priority     int   `json:"priority" binding:"min=0,max=1000" example:"100"`
	MaxPerMinute int   `json:"max_per_minute" binding:"min=0,max=10000" example:"5"`
	MaxPerHour   int   `json:"max_per_hour" binding:"min=0,max=100000" example:"80"`
	Enabled      *bool `json:"enabled" example:"true"`
}

// AdminUpdateRoleQuotaReq 调整角色额度策略。
type AdminUpdateRoleQuotaReq struct {
	DailyLimit int   `json:"daily_limit" binding:"min=0,max=100000" example:"30"`
	MaxPerHour int   `json:"max_per_hour" binding:"min=0,max=100000" example:"0"`
	Enabled    *bool `json:"enabled" example:"true"`
}

// AdminBatchRetryResp 重试批次结果。
type AdminBatchRetryResp struct {
	ID     uint   `json:"id" example:"1"`
	Status string `json:"status" example:"pending"`
}
