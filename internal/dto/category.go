package dto

// CategoryCreateReq 新增分类请求。
type CategoryCreateReq struct {
	// ParentID 父分类 ID，当前仅预留，不参与层级逻辑。
	ParentID *uint `json:"parent_id,omitempty" example:"1"`
	// Name 分类名称。
	Name string `json:"name" binding:"required" example:"编程"`
	// URL 分类路由别名。
	URL *string `json:"url,omitempty" example:"programming"`
	// Icon 图标地址或对象 key。
	Icon string `json:"icon" binding:"required" example:"icons/programming.svg"`
	// Description 分类描述。
	Description string `json:"description" binding:"required" example:"编程学习与工程实践"`
	// CoverImgUrl 封面图地址或对象 key。
	CoverImgUrl string `json:"cover_img_url" binding:"required" example:"covers/programming.jpg"`
	// Seq 排序值，越小越靠前；0 是有效值，因此用指针区分未传。
	Seq *uint `json:"seq" binding:"required" example:"0"`
}

// CategoryUpdateReq 修改分类请求；未传字段保持原值。
type CategoryUpdateReq struct {
	// ParentID 父分类 ID，当前仅预留，不参与层级逻辑。
	ParentID *uint `json:"parent_id,omitempty" example:"1"`
	// Name 分类名称。
	Name *string `json:"name,omitempty" example:"编程"`
	// URL 分类路由别名；传空字符串表示清空。
	URL *string `json:"url,omitempty" example:"programming"`
	// Icon 图标地址或对象 key；传空字符串表示清空。
	Icon *string `json:"icon,omitempty" example:"icons/programming.svg"`
	// Description 分类描述；传空字符串表示清空。
	Description *string `json:"description,omitempty" example:"编程学习与工程实践"`
	// CoverImgUrl 封面图地址或对象 key；传空字符串表示清空。
	CoverImgUrl *string `json:"cover_img_url,omitempty" example:"covers/programming.jpg"`
	// Seq 排序值，越小越靠前。
	Seq *uint `json:"seq,omitempty" example:"0"`
}

// CategoryArticlesReq 批量维护分类下文章请求。
type CategoryArticlesReq struct {
	// ArticleIDs 文章 ID 列表；单篇文章也使用一个元素的数组。
	ArticleIDs []uint `json:"article_ids" binding:"required" example:"1"`
}

// CategoryArticlesResp 批量维护分类下文章响应。
type CategoryArticlesResp struct {
	// CategoryID 分类 ID。
	CategoryID uint `json:"category_id" example:"1"`
	// ArticleIDs 本次请求归一化后的文章 ID 列表。
	ArticleIDs []uint `json:"article_ids"`
	// AffectedCount 实际影响的关联数量。
	AffectedCount int64 `json:"affected_count" example:"2"`
}

// CategoryItemResp 分类详情响应，含分类元数据及其下的公开文章数量。
type CategoryItemResp struct {
	// ID 分类 ID。
	ID uint `json:"id" example:"1"`
	// ParentID 父分类 ID，当前仅预留。
	ParentID *uint `json:"parent_id,omitempty" example:"1"`
	// Name 分类名称。
	Name string `json:"name" example:"编程"`
	// URL 分类路由别名。
	URL *string `json:"url,omitempty" example:"programming"`
	// Icon 图标 URL 或对象 key。
	Icon *string `json:"icon,omitempty"`
	// Description 分类描述。
	Description *string `json:"description,omitempty"`
	// CoverImgUrl 封面图地址或对象 key。
	CoverImgUrl *string `json:"cover_img_url,omitempty"`
	// Seq 排序值，越小越靠前。
	Seq uint `json:"seq" example:"0"`
	// ArticleCount 该分类下的公开文章数量。
	ArticleCount int64 `json:"article_count" example:"12"`
}

// CategoryTabItem 分类 Tab 列表项响应，含分类元数据及其下的公开文章数量。
type CategoryTabItem struct {
	// ID 分类 ID。
	ID uint `json:"id" example:"1"`
	// Name 分类名称。
	Name string `json:"name" example:"编程"`
	// URL 分类路由别名。
	URL *string `json:"url,omitempty" example:"programming"`
	// Icon 图标 URL。
	Icon *string `json:"icon,omitempty"`
	// Description 分类描述。
	Description *string `json:"description,omitempty"`
	// CoverImgUrl 封面图地址。
	CoverImgUrl *string `json:"cover_img_url,omitempty"`
	// Seq 排序值，越小越靠前。
	Seq uint `json:"seq" example:"0"`
	// ArticleCount 该分类下的公开文章数量。
	ArticleCount int64 `json:"article_count" example:"12"`
}

// CategoryTabsResp 分类 Tab 列表响应。
type CategoryTabsResp struct {
	// List 分类列表，按 seq ASC、文章数量 DESC 排序。
	List []CategoryTabItem `json:"list"`
}
