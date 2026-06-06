package dto

import "time"

// CommentListReq 评论分页查询参数。
type CommentListReq struct {
	// Page 页码，从 1 开始。
	Page int `form:"page" binding:"omitempty,min=1" example:"1"`
	// PageSize 每页数量，最大 50。
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50" example:"10"`
}

// CommentDeleteReq 删除一级评论请求。
type CommentDeleteReq struct {
	// TargetType 评论目标类型：article、moment、guestbook。
	TargetType string `form:"target_type" binding:"required,oneof=article moment guestbook" example:"article"`
}

// CommentCreateReq 新增一级评论请求。
type CommentCreateReq struct {
	// Content 评论内容，去除首尾空白后不能为空，最多 2000 字符。
	Content string `json:"content" binding:"required,max=2000" example:"写得真好"`
}

// CommentReplyCreateReq 新增评论回复请求。
type CommentReplyCreateReq struct {
	// ParentReplyID 上级回复 ID；0 表示直接回复一级评论。
	ParentReplyID uint `json:"parent_reply_id" example:"0"`
	// Content 回复内容，去除首尾空白后不能为空，最多 2000 字符。
	Content string `json:"content" binding:"required,max=2000" example:"收到"`
}

// CommentReplyListReq 评论回复分页查询参数。
type CommentReplyListReq struct {
	// Page 页码，从 1 开始。
	Page int `form:"page" binding:"omitempty,min=1" example:"1"`
	// PageSize 每页数量，最大 50。
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50" example:"10"`
}

// CommentUserResp 评论用户摘要。
type CommentUserResp struct {
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

// CommentReplyResp 评论回复响应。
type CommentReplyResp struct {
	// ID 回复 ID。
	ID uint `json:"id" example:"1"`
	// TargetType 评论目标类型。
	TargetType string `json:"target_type" example:"article"`
	// CommentID 一级评论 ID。
	CommentID uint `json:"comment_id" example:"1"`
	// FromUserID 回复者用户 ID。
	FromUserID uint `json:"from_user_id" example:"2"`
	// ToUserID 被回复者用户 ID。
	ToUserID uint `json:"to_user_id" example:"1"`
	// ParentReplyID 上级回复 ID；0 表示直接回复一级评论。
	ParentReplyID uint `json:"parent_reply_id" example:"0"`
	// Content 回复内容。
	Content string `json:"content" example:"收到"`
	// FromUser 回复者用户摘要。
	FromUser *CommentUserResp `json:"from_user,omitempty"`
	// ToUser 被回复者用户摘要。
	ToUser *CommentUserResp `json:"to_user,omitempty"`
	// LikeCount 回复点赞数量。
	LikeCount int64 `json:"like_count" example:"3"`
	// IsLiked 当前用户是否已点赞；未登录时恒为 false。
	IsLiked bool `json:"is_liked" example:"false"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// CommentItemResp 一级评论响应。
type CommentItemResp struct {
	// ID 评论 ID。
	ID uint `json:"id" example:"1"`
	// TargetType 评论目标类型。
	TargetType string `json:"target_type" example:"article"`
	// TargetID 评论目标 ID。
	TargetID uint `json:"target_id" example:"1"`
	// UserID 评论者用户 ID。
	UserID uint `json:"user_id" example:"1"`
	// Content 评论内容。
	Content string `json:"content" example:"写得真好"`
	// User 评论者用户摘要。
	User *CommentUserResp `json:"user,omitempty"`
	// ReplyCount 回复数量。
	ReplyCount int64 `json:"reply_count" example:"3"`
	// LikeCount 评论点赞数量。
	LikeCount int64 `json:"like_count" example:"3"`
	// IsLiked 当前用户是否已点赞；未登录时恒为 false。
	IsLiked bool `json:"is_liked" example:"false"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// CommentPageResp 评论分页响应。
type CommentPageResp struct {
	// Total 总记录数。
	Total int64 `json:"total" example:"100"`
	// Pages 总页数。
	Pages int `json:"pages" example:"10"`
	// Page 当前页码。
	Page int `json:"page" example:"1"`
	// PageSize 每页数量。
	PageSize int `json:"page_size" example:"10"`
	// List 评论列表。
	List []CommentItemResp `json:"list"`
}

// CommentReplyPageResp 评论回复分页响应。
type CommentReplyPageResp struct {
	// Total 总记录数。
	Total int64 `json:"total" example:"100"`
	// Pages 总页数。
	Pages int `json:"pages" example:"10"`
	// Page 当前页码。
	Page int `json:"page" example:"1"`
	// PageSize 每页数量。
	PageSize int `json:"page_size" example:"10"`
	// List 回复列表。
	List []CommentReplyResp `json:"list"`
}

// CommentLikeResp 评论或回复点赞状态响应。
type CommentLikeResp struct {
	// IsLiked 当前用户是否已点赞。
	IsLiked bool `json:"is_liked" example:"true"`
	// LikeCount 点赞数量。
	LikeCount int64 `json:"like_count" example:"3"`
}

// CommentDeleteResp 评论删除响应。
type CommentDeleteResp struct {
	// ID 被删除的评论或回复 ID。
	ID uint `json:"id" example:"1"`
}
