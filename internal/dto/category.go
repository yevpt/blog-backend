package dto

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
