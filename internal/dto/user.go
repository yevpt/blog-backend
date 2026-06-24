package dto

import "time"

// UserDetailResp 当前登录用户详情响应
type UserDetailResp struct {
	ID            uint                 `json:"id"`
	Username      string               `json:"username"`
	Nickname      *string              `json:"nickname,omitempty"`
	Email         *string              `json:"email,omitempty"`
	EmailVerified bool                 `json:"email_verified"`
	Phone         *string              `json:"phone,omitempty"`
	PasswordSet   bool                 `json:"password_set"`
	Site          *string              `json:"site,omitempty"`
	AvatarUrl     *string              `json:"avatar_url,omitempty"`
	Mark          *string              `json:"mark,omitempty"`
	Status        uint8                `json:"status"`
	LastLoginAt   *time.Time           `json:"last_login_at,omitempty"`
	Roles         []string             `json:"roles"`
	Meta          *UserMetaResp        `json:"meta,omitempty"`
	Setting       *UserSettingResp     `json:"setting,omitempty"`
	SocialLinks   []UserSocialLinkResp `json:"social_links,omitempty"`
}

// UserMetaResp 用户扩展资料响应
type UserMetaResp struct {
	Name             *string    `json:"name,omitempty"`
	Description      *string    `json:"description,omitempty"`
	SubEmail         *string    `json:"sub_email,omitempty"`
	SubEmailVerified bool       `json:"sub_email_verified"`
	Gender           *uint8     `json:"gender,omitempty"`
	Birthday         *time.Time `json:"birthday,omitempty"`
	Country          *string    `json:"country,omitempty"`
	Province         *string    `json:"province,omitempty"`
	City             *string    `json:"city,omitempty"`
	Address          *string    `json:"address,omitempty"`
}

// UserSettingResp 用户偏好设置响应
type UserSettingResp struct {
	MailShow     uint8 `json:"mail_show"`
	MailReceive  uint8 `json:"mail_receive"`
	DarkMode     uint8 `json:"dark_mode"`
	ReceiveMail  bool  `json:"receive_mail"`
	ShowName     bool  `json:"show_name"`
	ShowAge      bool  `json:"show_age"`
	ShowPhone    bool  `json:"show_phone"`
	ShowQq       bool  `json:"show_qq"`
	ShowWechat   bool  `json:"show_wechat"`
	ShowZhihu    bool  `json:"show_zhihu"`
	ShowSina     bool  `json:"show_sina"`
	ShowBili     bool  `json:"show_bili"`
	ShowPosition bool  `json:"show_position"`
}

// UserSocialLinkResp 用户社交链接响应
type UserSocialLinkResp struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

// UserListReq 获取用户列表请求
type UserListReq struct {
	Page     int `form:"page" binding:"omitempty,min=1" example:"1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50" example:"10"`
}

const (
	// UserLikedContentFilterArticle 表示只查询文章点赞。
	UserLikedContentFilterArticle = "article"
	// UserLikedContentFilterComment 表示查询评论与回复点赞。
	UserLikedContentFilterComment = "comment"
	// UserLikedContentFilterGuestbook 表示只查询留言点赞。
	UserLikedContentFilterGuestbook = "guestbook"
	// UserLikedContentFilterMoment 表示只查询碎语点赞。
	UserLikedContentFilterMoment = "moment"

	// UserLikedContentKindArticle 表示点赞目标为文章。
	UserLikedContentKindArticle = "article"
	// UserLikedContentKindComment 表示点赞目标为一级评论。
	UserLikedContentKindComment = "comment"
	// UserLikedContentKindReply 表示点赞目标为回复。
	UserLikedContentKindReply = "reply"
	// UserLikedContentKindGuestbook 表示点赞目标为留言。
	UserLikedContentKindGuestbook = "guestbook"
	// UserLikedContentKindMoment 表示点赞目标为碎语。
	UserLikedContentKindMoment = "moment"
)

// UserLikedContentListReq 用户点赞内容分页查询参数。
type UserLikedContentListReq struct {
	// Page 页码，从 1 开始。
	Page int `form:"page" binding:"omitempty,min=1" example:"1"`
	// PageSize 每页数量，默认 20，最大 50。
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50" example:"20"`
	// Type 筛选类型；comment 包含评论与回复。
	Type string `form:"type" binding:"omitempty,oneof=article comment guestbook moment" example:"comment"`
}

// UserLikedContentAuthorResp 点赞内容原作者摘要。
type UserLikedContentAuthorResp struct {
	ID        uint     `json:"id" example:"1"`
	Username  string   `json:"username,omitempty" example:"vpt"`
	Nickname  *string  `json:"nickname,omitempty" example:"VPT"`
	AvatarUrl *string  `json:"avatar_url,omitempty" example:"https://cdn.example.com/avatar.png"`
	Site      *string  `json:"site,omitempty" example:"https://yevpt.com"`
	Mark      *string  `json:"mark,omitempty" example:"博主"`
	Roles     []string `json:"roles"`
}

// UserLikedContentObjectResp 点赞内容、父级或根对象摘要。
type UserLikedContentObjectResp struct {
	ID          uint    `json:"id" example:"1"`
	Kind        string  `json:"kind,omitempty" example:"article"`
	Title       *string `json:"title,omitempty" example:"React Aria 组件实践"`
	Excerpt     string  `json:"excerpt,omitempty" example:"内容摘要"`
	CoverImgUrl *string `json:"cover_img_url,omitempty" example:"https://cdn.example.com/cover.jpg"`
	Deleted     bool    `json:"deleted" example:"false"`
}

// UserLikedContentStatsResp 点赞内容的轻量互动统计。
type UserLikedContentStatsResp struct {
	LikeCount    *int64 `json:"like_count,omitempty" example:"3"`
	CommentCount *int64 `json:"comment_count,omitempty" example:"2"`
}

// UserLikedContentItemResp 用户点赞内容列表项。
type UserLikedContentItemResp struct {
	ID      uint                        `json:"id" example:"1"`
	LikedAt time.Time                   `json:"liked_at"`
	Kind    string                      `json:"kind" example:"reply"`
	Filter  string                      `json:"filter" example:"comment"`
	Author  *UserLikedContentAuthorResp `json:"author,omitempty"`
	Content UserLikedContentObjectResp  `json:"content"`
	Parent  *UserLikedContentObjectResp `json:"parent,omitempty"`
	Root    *UserLikedContentObjectResp `json:"root,omitempty"`
	Stats   *UserLikedContentStatsResp  `json:"stats,omitempty"`
}

// UserLikedContentPageResp 用户点赞内容分页响应。
type UserLikedContentPageResp struct {
	Total    int64                      `json:"total" example:"100"`
	Pages    int                        `json:"pages" example:"5"`
	Page     int                        `json:"page" example:"1"`
	PageSize int                        `json:"page_size" example:"20"`
	List     []UserLikedContentItemResp `json:"list"`
}

// UserListItemResp 用户列表项响应
type UserListItemResp struct {
	ID          uint       `json:"id" example:"1"`
	Nickname    *string    `json:"nickname,omitempty" example:"Yevpt"`
	AvatarUrl   *string    `json:"avatar_url,omitempty" example:"https://cdn.example.com/avatar.png"`
	Mark        *string    `json:"mark,omitempty" example:"博主"`
	Roles       []string   `json:"roles"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// UserPageResp 用户分页响应
type UserPageResp struct {
	Total    int64              `json:"total" example:"100"`
	Pages    int                `json:"pages" example:"10"`
	Page     int                `json:"page" example:"1"`
	PageSize int                `json:"page_size" example:"10"`
	List     []UserListItemResp `json:"list"`
}

// UserUpdateReq 更新当前用户信息请求
type UserUpdateReq struct {
	Nickname *string `json:"nickname,omitempty" binding:"omitempty,max=30" example:"Yevpt"`
	Mark     *string `json:"mark,omitempty" binding:"omitempty,max=30" example:"博主"`
}

// UserPublicProfileResp GET /users/:id 某用户的公开详情
type UserPublicProfileResp struct {
	ID           uint                 `json:"id"`
	Nickname     string               `json:"nickname"`
	AvatarUrl    *string              `json:"avatar_url"`
	Mark         *string              `json:"mark"`
	Description  *string              `json:"description"`
	LastLoginAt  *time.Time           `json:"last_login_at"`
	RegisterAt   time.Time            `json:"register_at"`
	Roles        []string             `json:"roles"`
	DisplayEmail *string              `json:"display_email"`
	Site         *string              `json:"site"`
	SocialLinks  []UserSocialLinkResp `json:"social_links"`
	Gender       *string              `json:"gender"`
	Birthday     *string              `json:"birthday"`
}

// UpdateProfileReq PATCH /users/me/profile
type UpdateProfileReq struct {
	Nickname    *string `json:"nickname" binding:"omitempty,max=30"`
	Mark        *string `json:"mark" binding:"omitempty,max=30"`
	Description *string `json:"description" binding:"omitempty,max=200"`
	Site        *string `json:"site" binding:"omitempty,max=200"`
}

// UpdateMetaReq PATCH /users/me/meta
// nil = 不更新；空字符串 "" = 清除该字段
type UpdateMetaReq struct {
	Gender   *string `json:"gender"`
	Birthday *string `json:"birthday"`
	Phone    *string `json:"phone"`
}

// UpdateSocialLinkReq PATCH /users/me/social/:platform
type UpdateSocialLinkReq struct {
	URL *string `json:"url"` // null 或 "" = 删除该平台链接
}

// UpdateUsernameReq PATCH /users/me/username
type UpdateUsernameReq struct {
	Username string `json:"username" binding:"required,min=3,max=155"`
}

// UpdatePasswordReq PATCH /users/me/password
type UpdatePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required,min=6"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// SetInitialPasswordReq PATCH /users/me/password/initial
type SetInitialPasswordReq struct {
	NewPassword string `json:"new_password" binding:"required,min=8"`
	Code        string `json:"code" binding:"required,len=6"`
}

// SendAccountEmailCodeReq POST /users/me/email/code
type SendAccountEmailCodeReq struct {
	Email        string `json:"email" binding:"required,email"`
	CaptchaToken string `json:"captcha_token" binding:"required"`
}

// UpdateEmailReq PATCH /users/me/email
type UpdateEmailReq struct {
	Target string `json:"target" binding:"required,oneof=main sub"`
	Email  string `json:"email" binding:"required,email"`
	Code   string `json:"code" binding:"required,len=6"`
}

// EmailDisplayReq PATCH /users/me/email/display
type EmailDisplayReq struct {
	Display string `json:"display" binding:"required,oneof=main sub none"`
}
