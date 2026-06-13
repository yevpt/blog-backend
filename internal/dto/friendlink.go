package dto

import "time"

// FriendLinkListReq 友情链接分页查询请求。
type FriendLinkListReq struct {
	// Page 页码，从 1 开始。
	Page int `form:"page" example:"1"`
	// PageSize 每页数量，最大 50。
	PageSize int `form:"page_size" example:"10"`
	// Status 状态过滤，仅管理列表使用：0 隐藏，1 显示。
	Status *uint8 `form:"status" example:"1"`
}

// FriendLinkCreateReq 新增友情链接请求。
type FriendLinkCreateReq struct {
	// Name 网站名称。
	Name string `json:"name" binding:"required" example:"VPT"`
	// Description 网站描述。
	Description *string `json:"description,omitempty" example:"个人博客"`
	// Email 站长邮箱。
	Email *string `json:"email,omitempty" example:"hello@example.com"`
	// Phone 联系电话。
	Phone *string `json:"phone,omitempty" example:"13800138000"`
	// Site 网站 URL。
	Site string `json:"site" binding:"required" example:"https://example.com"`
	// AvatarUrl 网站头像或 Logo 地址，可以是外链或对象存储 key。
	AvatarUrl *string `json:"avatar_url,omitempty" example:"friend-links/vpt.png"`
	// Seq 排序值，越小越靠前；0 是有效值，因此用指针区分未传。
	Seq *uint `json:"seq" binding:"required" example:"0"`
	// Status 状态：0 隐藏，1 显示；未传默认显示。
	Status *uint8 `json:"status,omitempty" example:"1"`
}

// FriendLinkUpdateReq 修改友情链接请求；未传字段保持原值。
type FriendLinkUpdateReq struct {
	// Name 网站名称。
	Name *string `json:"name,omitempty" example:"VPT"`
	// Description 网站描述；传空字符串表示清空。
	Description *string `json:"description,omitempty" example:"个人博客"`
	// Email 站长邮箱；传空字符串表示清空。
	Email *string `json:"email,omitempty" example:"hello@example.com"`
	// Phone 联系电话；传空字符串表示清空。
	Phone *string `json:"phone,omitempty" example:"13800138000"`
	// Site 网站 URL。
	Site *string `json:"site,omitempty" example:"https://example.com"`
	// AvatarUrl 网站头像或 Logo 地址；传空字符串表示清空。
	AvatarUrl *string `json:"avatar_url,omitempty" example:"friend-links/vpt.png"`
	// Seq 排序值，越小越靠前。
	Seq *uint `json:"seq,omitempty" example:"0"`
	// Status 状态：0 隐藏，1 显示。
	Status *uint8 `json:"status,omitempty" example:"1"`
}

// FriendLinkItemResp 友情链接详情响应。
type FriendLinkItemResp struct {
	// ID 友情链接 ID。
	ID uint `json:"id" example:"1"`
	// Name 网站名称。
	Name string `json:"name" example:"VPT"`
	// Description 网站描述。
	Description *string `json:"description,omitempty" example:"个人博客"`
	// Email 站长邮箱。
	Email *string `json:"email,omitempty" example:"hello@example.com"`
	// Phone 联系电话。
	Phone *string `json:"phone,omitempty" example:"13800138000"`
	// Site 网站 URL。
	Site string `json:"site" example:"https://example.com"`
	// AvatarUrl 网站头像或 Logo 的可访问 URL。
	AvatarUrl *string `json:"avatar_url,omitempty" example:"https://cdn.example.com/blog/friend-links/vpt.png"`
	// Seq 排序值，越小越靠前。
	Seq uint `json:"seq" example:"0"`
	// Status 状态：0 隐藏，1 显示。
	Status uint8 `json:"status" example:"1"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// FriendLinkPageResp 友情链接分页响应。
type FriendLinkPageResp struct {
	// Total 总记录数。
	Total int64 `json:"total" example:"20"`
	// Pages 总页数。
	Pages int `json:"pages" example:"2"`
	// Page 当前页码。
	Page int `json:"page" example:"1"`
	// PageSize 每页数量。
	PageSize int `json:"page_size" example:"10"`
	// List 友情链接列表。
	List []FriendLinkItemResp `json:"list"`
}
