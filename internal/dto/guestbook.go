package dto

import "time"

// GuestbookListReq 留言板分页查询参数。
type GuestbookListReq struct {
	// OwnerUserID 留言板主人用户 ID；省略时默认查询博主 1 的留言板。
	OwnerUserID uint `form:"owner_user_id" binding:"omitempty,min=1" example:"1"`
	// Page 页码，从 1 开始。
	Page int `form:"page" binding:"omitempty,min=1" example:"1"`
	// PageSize 每页数量，默认 10，最大 50。
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50" example:"10"`
}

// GuestbookCreateReq 发表留言请求。
type GuestbookCreateReq struct {
	// OwnerUserID 留言板主人用户 ID；省略时默认给博主 1 留言。
	OwnerUserID uint `json:"owner_user_id" binding:"omitempty,min=1" example:"1"`
	// Content 留言内容，去除首尾空白后不能为空，最多 2000 字符。
	Content string `json:"content" binding:"required,max=2000" example:"来踩踩，博客很棒"`
}

// GuestbookUserResp 留言用户摘要。
type GuestbookUserResp struct {
	// ID 用户 ID。
	ID uint `json:"id" example:"1"`
	// Username 登录账号。
	Username string `json:"username" example:"vpt"`
	// Nickname 用户昵称。
	Nickname *string `json:"nickname,omitempty" example:"VPT"`
	// AvatarUrl 用户头像地址。
	AvatarUrl *string `json:"avatar_url,omitempty" example:"https://example.com/avatar.png"`
	// Site 用户个人站点。
	Site *string `json:"site,omitempty" example:"https://yevpt.com"`
	// Mark 用户身份标签。
	Mark *string `json:"mark,omitempty" example:"注册会员"`
}

// GuestbookItemResp 留言响应。
type GuestbookItemResp struct {
	// ID 留言 ID。
	ID uint `json:"id" example:"1"`
	// OwnerUserID 留言板主人用户 ID。
	OwnerUserID uint `json:"owner_user_id" example:"1"`
	// FromUserID 留言者用户 ID。
	FromUserID uint `json:"from_user_id" example:"7"`
	// Content 留言内容。
	Content string `json:"content" example:"来踩踩，博客很棒"`
	// User 留言者用户摘要。
	User *GuestbookUserResp `json:"user,omitempty"`
	// LikeCount 点赞数量。
	LikeCount int64 `json:"like_count" example:"3"`
	// IsLiked 当前用户是否已点赞。
	IsLiked bool `json:"is_liked" example:"false"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// GuestbookPageResp 留言分页响应。
type GuestbookPageResp struct {
	// Total 总记录数。
	Total int64 `json:"total" example:"100"`
	// Pages 总页数。
	Pages int `json:"pages" example:"10"`
	// Page 当前页码。
	Page int `json:"page" example:"1"`
	// PageSize 每页数量。
	PageSize int `json:"page_size" example:"10"`
	// List 留言列表。
	List []GuestbookItemResp `json:"list"`
}

// GuestbookLikeResp 留言点赞状态响应。
type GuestbookLikeResp struct {
	// ID 留言 ID。
	ID uint `json:"id" example:"1"`
	// IsLiked 当前用户是否已点赞。
	IsLiked bool `json:"is_liked" example:"true"`
	// LikeCount 点赞数量。
	LikeCount int64 `json:"like_count" example:"3"`
}

// GuestbookDeleteResp 留言删除响应。
type GuestbookDeleteResp struct {
	// ID 被删除的留言 ID。
	ID uint `json:"id" example:"1"`
}
