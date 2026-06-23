package dto

import "time"

// NotificationListReq 站内通知分页查询参数。
type NotificationListReq struct {
	// Page 页码，从 1 开始。
	Page int `form:"page" binding:"omitempty,min=1" example:"1"`
	// PageSize 每页数量，最大 50。
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50" example:"10"`
	// UnreadOnly 为 true 时只返回未读通知。
	UnreadOnly bool `form:"unread_only" example:"false"`
}

// NotificationReadAllReq 批量已读请求；ids 为空且 all 为 true 表示全部已读。
type NotificationReadAllReq struct {
	// IDs 需置为已读的收件箱 ID 列表，最多 100 个。
	IDs []uint `json:"ids" binding:"omitempty,max=100" example:"1,2,3"`
	// All 为 true 且 ids 为空时表示将当前用户全部未读置为已读。
	All bool `json:"all" example:"false"`
}

// NotificationItemResp 单条站内通知响应，字段取自事件快照，不直接暴露 model。
type NotificationItemResp struct {
	// ID 收件箱记录 ID。
	ID uint `json:"id" example:"1"`
	// EventID 关联事件 ID。
	EventID uint `json:"event_id" example:"10"`
	// Type 事件类型，如 comment_created。
	Type string `json:"type" example:"comment_created"`
	// Title 事件标题快照。
	Title string `json:"title" example:"VPT 评论了你的碎语"`
	// ContentExcerpt 内容摘要快照。
	ContentExcerpt string `json:"content_excerpt" example:"写得真好"`
	// IsRead 是否已读。
	IsRead bool `json:"is_read" example:"false"`
	// ReadAt 已读时间，未读为空。
	ReadAt *time.Time `json:"read_at,omitempty"`
	// CreatedAt 通知产生时间。
	CreatedAt time.Time `json:"created_at"`
	// ActorUserID 操作人用户 ID，系统通知为空。
	ActorUserID *uint `json:"actor_user_id,omitempty" example:"2"`
	// SourceType 直接对象类型，如 comment。
	SourceType string `json:"source_type" example:"comment"`
	// SourceID 直接对象 ID。
	SourceID uint `json:"source_id" example:"99"`
	// RootType 根对象类型，如 moment。
	RootType string `json:"root_type" example:"moment"`
	// RootID 根对象 ID。
	RootID uint `json:"root_id" example:"12"`
	// RootTitle 根对象展示标题快照（文章标题/碎语摘要），无根对象或已删除时为空。
	RootTitle *string `json:"root_title,omitempty" example:"我的第一篇文章"`
	// Metadata 跳转与扩展信息的原始 JSON，前端按需解析。
	Metadata *string `json:"metadata,omitempty"`
}

// NotificationPageResp 站内通知分页响应。
type NotificationPageResp struct {
	// Total 满足条件的通知总数。
	Total int64 `json:"total" example:"42"`
	// Page 当前页码。
	Page int `json:"page" example:"1"`
	// PageSize 每页数量。
	PageSize int `json:"page_size" example:"10"`
	// List 当前页通知列表。
	List []NotificationItemResp `json:"list"`
}

// NotificationUnreadCountResp 未读数量响应。
type NotificationUnreadCountResp struct {
	// Count 未读通知数量。
	Count int64 `json:"count" example:"3"`
}

// NotificationReadResp 已读操作结果。
type NotificationReadResp struct {
	// Updated 实际被置为已读的通知数量。
	Updated int64 `json:"updated" example:"3"`
}
