package model

type Category struct {
	Base
	ParentID    *uint   `gorm:"comment:父分类ID，NULL 表示顶级分类" json:"parent_id"`
	Name        string  `gorm:"size:40;not null;comment:分类名称" json:"name"`
	URL         *string `gorm:"size:200;comment:分类路由别名" json:"url"`
	Icon        *string `gorm:"size:200;comment:图标URL" json:"icon"`
	Description *string `gorm:"size:500;comment:分类描述" json:"description"`
	CoverImgUrl *string `gorm:"size:200;comment:封面图URL" json:"cover_img_url"`
	Seq         uint    `gorm:"type:int;default:0;comment:排序" json:"seq"`
}

func (Category) TableName() string { return "category" }

type ArticleCategory struct {
	ID         uint `gorm:"primarykey" json:"id"`
	ArticleID  uint `gorm:"not null;uniqueIndex:idx_article_category,priority:1;index;comment:文章ID" json:"article_id"`
	CategoryID uint `gorm:"not null;uniqueIndex:idx_article_category,priority:2;comment:分类ID" json:"category_id"`
}

func (ArticleCategory) TableName() string { return "article_category" }
