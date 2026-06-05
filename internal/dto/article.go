package dto

import "time"

// ArticleListReq 文章分页查询参数。
type ArticleListReq struct {
	// Page 页码，从 1 开始。
	Page int `form:"page" binding:"omitempty,min=1" example:"1"`
	// PageSize 每页数量，最大 50。
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50" example:"10"`
	// Recommend 是否只查询推荐文章。
	Recommend *bool `form:"recommend" example:"true"`
	// CategoryID 分类 ID 过滤条件。
	CategoryID *uint `form:"category_id" example:"1"`
	// TagID 标签 ID 过滤条件。
	TagID *uint `form:"tag_id" example:"1"`
}

// ArticleSaveReq 新增或更新文章请求。
type ArticleSaveReq struct {
	// ID 文章 ID，为空或 0 表示新增。
	ID *uint `json:"id" example:"1"`
	// Title 文章标题。
	Title string `json:"title" binding:"required" example:"文章标题"`
	// CoverImgUrl 封面图地址。
	CoverImgUrl *string `json:"cover_img_url" example:"https://example.com/cover.jpg"`
	// ShortContent 文章摘要。
	ShortContent *string `json:"short_content" example:"摘要"`
	// Content Markdown 正文内容。
	Content string `json:"content" binding:"required" example:"Markdown 正文"`
	// Status 文章状态：0 隐藏，1 公开，2 加密。
	Status uint8 `json:"status" binding:"oneof=0 1 2" example:"1"`
	// CommentStatus 评论状态：0 关闭，1 开启。
	CommentStatus uint8 `json:"comment_status" binding:"oneof=0 1" example:"1"`
	// Password 加密文章密码。
	Password *string `json:"password" example:"secret"`
	// CategoryIDs 分类 ID 列表，至少一个；当前每篇文章只归属第一个有效分类。
	CategoryIDs []uint `json:"category_ids" binding:"required,min=1" example:"1"`
	// TagIDs 标签 ID 列表。
	TagIDs []uint `json:"tag_ids" example:"1"`
	// MusicIDs 音乐 ID 列表。
	MusicIDs []uint `json:"music_ids" example:"1"`
	// Recommend 是否推荐文章。
	Recommend bool `json:"recommend" example:"false"`
	// RecommendSeq 推荐排序值。
	RecommendSeq uint `json:"recommend_seq" example:"10"`
}

// ArticleRelationResp 文章分类、标签、音乐等轻量关联响应。
type ArticleRelationResp struct {
	// ID 关联资源 ID。
	ID uint `json:"id" example:"1"`
	// Name 关联资源名称。
	Name string `json:"name" example:"Go"`
	// URL 关联资源访问地址或别名。
	URL *string `json:"url,omitempty" example:"go"`
	// Icon 关联资源图标。
	Icon *string `json:"icon,omitempty"`
	// Description 关联资源描述。
	Description *string `json:"description,omitempty"`
	// CoverImgUrl 关联资源封面图地址。
	CoverImgUrl *string `json:"cover_img_url,omitempty"`
}

// ArticleMusicResp 文章关联音乐响应。
type ArticleMusicResp struct {
	// ID 音乐 ID。
	ID uint `json:"id" example:"1"`
	// Name 音乐名称。
	Name string `json:"name" example:"Song"`
	// Singer 歌手名称。
	Singer string `json:"singer" example:"Singer"`
	// Album 专辑名称。
	Album string `json:"album" example:"Album"`
	// URL 音乐播放地址。
	URL *string `json:"url,omitempty"`
	// CoverImgUrl 音乐封面图地址。
	CoverImgUrl *string `json:"cover_img_url,omitempty"`
	// Duration 音乐时长，单位为秒。
	Duration uint16 `json:"duration" example:"240"`
}

// ArticleListItemResp 文章列表项响应，不包含正文。
type ArticleListItemResp struct {
	// ID 文章 ID。
	ID uint `json:"id" example:"1"`
	// Title 文章标题。
	Title string `json:"title" example:"文章标题"`
	// CoverImgUrl 封面图地址。
	CoverImgUrl *string `json:"cover_img_url,omitempty"`
	// ShortContent 文章摘要。
	ShortContent *string `json:"short_content,omitempty"`
	// UserID 作者用户 ID。
	UserID uint `json:"user_id" example:"1"`
	// Status 文章状态。
	Status uint8 `json:"status" example:"1"`
	// CommentStatus 评论状态。
	CommentStatus uint8 `json:"comment_status" example:"1"`
	// ReadCount 阅读数量。
	ReadCount uint `json:"read_count" example:"20"`
	// LikeCount 点赞数量。
	LikeCount int64 `json:"like_count" example:"3"`
	// CommentCount 评论数量。
	CommentCount int64 `json:"comment_count" example:"2"`
	// IsLiked 当前用户是否已点赞；未登录时恒为 false。
	IsLiked bool `json:"is_liked" example:"false"`
	// IsRecommended 是否为推荐文章。
	IsRecommended bool `json:"is_recommended" example:"true"`
	// Category 文章所属分类（每篇文章归属一个分类）。
	Category *ArticleRelationResp `json:"category,omitempty"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// ArticleDetailResp 文章详情响应。
type ArticleDetailResp struct {
	// ArticleListItemResp 复用文章列表项的基础字段。
	ArticleListItemResp
	// Content 文章正文。
	Content string `json:"content,omitempty"`
	// Passworded 是否为加密文章。
	Passworded bool `json:"passworded" example:"false"`
	// IsLiked 当前用户是否已点赞。
	IsLiked bool `json:"is_liked" example:"false"`
	// CategoryIDs 分类 ID 列表。
	CategoryIDs []uint `json:"category_ids"`
	// Categories 分类列表。
	Categories []ArticleRelationResp `json:"categories"`
	// TagIDs 标签 ID 列表。
	TagIDs []uint `json:"tag_ids"`
	// Tags 标签列表。
	Tags []ArticleRelationResp `json:"tags"`
	// MusicIDs 音乐 ID 列表。
	MusicIDs []uint `json:"music_ids"`
	// Music 音乐列表。
	Music []ArticleMusicResp `json:"music"`
	// RecommendSeq 推荐排序值。
	RecommendSeq *uint `json:"recommend_seq,omitempty" example:"10"`
}

// ArticlePageResp 文章分页响应。
type ArticlePageResp struct {
	// Total 总记录数。
	Total int64 `json:"total" example:"100"`
	// Pages 总页数。
	Pages int `json:"pages" example:"10"`
	// Page 当前页码。
	Page int `json:"page" example:"1"`
	// PageSize 每页数量。
	PageSize int `json:"page_size" example:"10"`
	// List 文章列表。
	List []ArticleListItemResp `json:"list"`
}

// ArticleIDsResp 文章 ID 列表响应。
type ArticleIDsResp struct {
	// IDs 文章 ID 列表。
	IDs []uint `json:"ids" example:"1,2,3"`
}

// ArticleLikeResp 文章点赞状态响应。
type ArticleLikeResp struct {
	// IsLiked 当前用户是否已点赞。
	IsLiked bool `json:"is_liked" example:"true"`
	// LikeCount 点赞数量。
	LikeCount int64 `json:"like_count" example:"3"`
}

// ArticleReadResp 阅读数响应。
type ArticleReadResp struct {
	// ID 文章 ID。
	ID uint `json:"id" example:"1"`
	// ReadCount 阅读数量。
	ReadCount uint `json:"read_count" example:"21"`
}
