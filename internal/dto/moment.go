package dto

import "time"

// MomentListReq 碎语分页查询参数。
type MomentListReq struct {
	// UserID 作者用户 ID；省略时查询所有公开碎语。
	UserID *uint `form:"user_id" binding:"omitempty,min=1" example:"1"`
	// RoleID 作者角色 ID；传入后查询该角色下所有用户的公开碎语。
	RoleID *uint `form:"role_id" binding:"omitempty,min=1" example:"2"`
	// Page 页码，从 1 开始。
	Page int `form:"page" binding:"omitempty,min=1" example:"1"`
	// PageSize 每页数量，默认 10，最大 50。
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50" example:"10"`
}

// MomentMediaReq 碎语图片保存请求。
type MomentMediaReq struct {
	// Name 图片原始文件名。
	Name string `json:"name" example:"cat.jpg"`
	// FileType 图片文件类型或扩展名。
	FileType string `json:"file_type" example:"jpg"`
	// URL 图片对象 key 或可访问 URL。
	URL string `json:"url" binding:"required,max=1000" example:"moments/cat.jpg"`
	// Size 图片大小，单位字节。
	Size uint `json:"size" example:"1024"`
	// Seq 图片排序值，越小越靠前。
	Seq uint `json:"seq" example:"1"`
}

// MomentSaveReq 新增或更新碎语请求。
type MomentSaveReq struct {
	// ID 碎语 ID，为空或 0 表示新增。
	ID *uint `json:"id" example:"1"`
	// UserID 作者用户 ID；管理员可传入代管作者，普通用户会被强制设置为当前登录用户。
	UserID *uint `json:"user_id" example:"1"`
	// Content 碎语正文，去除首尾空白后不能为空，最多 800 字符。
	Content string `json:"content" binding:"required,max=800" example:"今天的风很温柔"`
	// Status 状态：0 隐藏，1 公开。
	Status uint8 `json:"status" binding:"oneof=0 1" example:"1"`
	// CommentStatus 评论状态：0 关闭，1 开启。
	CommentStatus uint8 `json:"comment_status" binding:"oneof=0 1" example:"1"`
	// Images 图片列表，会整体替换该碎语原有图片。
	Images []MomentMediaReq `json:"images"`
}

// MomentUserResp 碎语作者摘要。
type MomentUserResp struct {
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
	Mark *string `json:"mark,omitempty" example:"博主"`
}

// MomentMediaResp 碎语图片响应。
type MomentMediaResp struct {
	// ID 图片 ID。
	ID uint `json:"id" example:"1"`
	// Name 图片原始文件名。
	Name string `json:"name" example:"cat.jpg"`
	// FileType 图片文件类型或扩展名。
	FileType string `json:"file_type" example:"jpg"`
	// URL 图片对象 key 或原始 URL。
	URL string `json:"url" example:"moments/cat.jpg"`
	// AccessURL 可直接访问的图片地址。
	AccessURL string `json:"access_url" example:"https://cdn.example.com/moments/cat.jpg"`
	// Size 图片大小，单位字节。
	Size uint `json:"size" example:"1024"`
	// Seq 图片排序值。
	Seq uint `json:"seq" example:"1"`
}

// MomentItemResp 碎语响应。
type MomentItemResp struct {
	// ID 碎语 ID。
	ID uint `json:"id" example:"1"`
	// UserID 作者用户 ID。
	UserID uint `json:"user_id" example:"1"`
	// Content 碎语正文。
	Content string `json:"content" example:"今天的风很温柔"`
	// Status 状态：0 隐藏，1 公开。
	Status uint8 `json:"status" example:"1"`
	// CommentStatus 评论状态：0 关闭，1 开启。
	CommentStatus uint8 `json:"comment_status" example:"1"`
	// ReadCount 阅读数量。
	ReadCount uint `json:"read_count" example:"20"`
	// IsTop 是否置顶。
	IsTop bool `json:"is_top" example:"false"`
	// LikeCount 点赞数量。
	LikeCount int64 `json:"like_count" example:"3"`
	// CommentCount 评论数量。
	CommentCount int64 `json:"comment_count" example:"2"`
	// IsLiked 当前用户是否已点赞。
	IsLiked bool `json:"is_liked" example:"false"`
	// User 作者摘要。
	User *MomentUserResp `json:"user,omitempty"`
	// Images 图片列表。
	Images []MomentMediaResp `json:"images"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// MomentPageResp 碎语分页响应。
type MomentPageResp struct {
	// Total 总记录数。
	Total int64 `json:"total" example:"100"`
	// Pages 总页数。
	Pages int `json:"pages" example:"10"`
	// Page 当前页码。
	Page int `json:"page" example:"1"`
	// PageSize 每页数量。
	PageSize int `json:"page_size" example:"10"`
	// List 碎语列表。
	List []MomentItemResp `json:"list"`
}

// MomentLikeResp 碎语点赞状态响应。
type MomentLikeResp struct {
	// IsLiked 当前用户是否已点赞。
	IsLiked bool `json:"is_liked" example:"true"`
	// LikeCount 点赞数量。
	LikeCount int64 `json:"like_count" example:"3"`
}

// MomentViewResp 阅读数响应。
type MomentViewResp struct {
	// ID 碎语 ID。
	ID uint `json:"id" example:"1"`
	// ViewCount 阅读数量。
	ViewCount uint `json:"view_count" example:"21"`
}

// MomentDeleteResp 碎语删除响应。
type MomentDeleteResp struct {
	// ID 被删除的碎语 ID。
	ID uint `json:"id" example:"1"`
}

// MomentTopResp 碎语置顶状态响应。
type MomentTopResp struct {
	// ID 碎语 ID。
	ID uint `json:"id" example:"1"`
	// IsTop 是否置顶。
	IsTop bool `json:"is_top" example:"true"`
}
