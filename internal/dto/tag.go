package dto

// TagCreateReq 新增标签请求。
type TagCreateReq struct {
	// Name 标签名称。
	Name string `json:"name" binding:"required" example:"Go"`
	// URL 标签路由别名。
	URL *string `json:"url,omitempty" example:"go"`
	// Icon 图标地址或对象 key。
	Icon *string `json:"icon,omitempty" example:"icons/go.svg"`
	// Description 标签描述。
	Description *string `json:"description,omitempty" example:"Go 语言相关内容"`
	// CoverImgUrl 封面图地址或对象 key。
	CoverImgUrl *string `json:"cover_img_url,omitempty" example:"covers/go.jpg"`
	// Seq 排序值，越小越靠前；0 是有效值，因此用指针区分未传。
	Seq *uint `json:"seq" binding:"required" example:"0"`
}

// TagUpdateReq 修改标签请求；未传字段保持原值。
type TagUpdateReq struct {
	// Name 标签名称。
	Name *string `json:"name,omitempty" example:"Go"`
	// URL 标签路由别名；传空字符串表示清空。
	URL *string `json:"url,omitempty" example:"go"`
	// Icon 图标地址或对象 key；传空字符串表示清空。
	Icon *string `json:"icon,omitempty" example:"icons/go.svg"`
	// Description 标签描述；传空字符串表示清空。
	Description *string `json:"description,omitempty" example:"Go 语言相关内容"`
	// CoverImgUrl 封面图地址或对象 key；传空字符串表示清空。
	CoverImgUrl *string `json:"cover_img_url,omitempty" example:"covers/go.jpg"`
	// Seq 排序值，越小越靠前。
	Seq *uint `json:"seq,omitempty" example:"0"`
}

// TagArticlesReq 批量维护标签下文章请求。
type TagArticlesReq struct {
	// ArticleIDs 文章 ID 列表；单篇文章也使用一个元素的数组。
	ArticleIDs []uint `json:"article_ids" binding:"required" example:"1"`
}

// TagArticlesResp 批量维护标签下文章响应。
type TagArticlesResp struct {
	// TagID 标签 ID。
	TagID uint `json:"tag_id" example:"1"`
	// ArticleIDs 本次请求归一化后的文章 ID 列表。
	ArticleIDs []uint `json:"article_ids"`
	// AffectedCount 实际影响的关联数量。
	AffectedCount int64 `json:"affected_count" example:"2"`
}

// TagItemResp 标签详情响应，含标签元数据及其下的公开文章数量。
type TagItemResp struct {
	// ID 标签 ID。
	ID uint `json:"id" example:"1"`
	// Name 标签名称。
	Name string `json:"name" example:"Go"`
	// URL 标签路由别名。
	URL *string `json:"url,omitempty" example:"go"`
	// Icon 图标 URL 或对象 key。
	Icon *string `json:"icon,omitempty"`
	// Description 标签描述。
	Description *string `json:"description,omitempty"`
	// CoverImgUrl 封面图地址或对象 key。
	CoverImgUrl *string `json:"cover_img_url,omitempty"`
	// Seq 排序值，越小越靠前。
	Seq uint `json:"seq" example:"0"`
	// ArticleCount 该标签下的公开文章数量。
	ArticleCount int64 `json:"article_count" example:"12"`
}

// TagListResp 标签列表响应。
type TagListResp struct {
	// List 标签列表，按 seq ASC、文章数量 DESC 排序。
	List []TagItemResp `json:"list"`
}
