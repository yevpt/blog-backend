package model

type Category struct {
	Base
	ParentID    *uint   `json:"parent_id"`
	Name        string  `gorm:"size:40;not null" json:"name"`
	URL         *string `gorm:"size:200" json:"url"`
	Icon        *string `gorm:"size:200" json:"icon"`
	Description *string `gorm:"size:500" json:"description"`
	CoverImgUrl *string `gorm:"size:200" json:"cover_img_url"`
	Seq         uint    `gorm:"type:int;default:0" json:"seq"`
}

func (Category) TableName() string { return "category" }

type ArticleCategory struct {
	ID         uint `gorm:"primarykey" json:"id"`
	ArticleID  uint `gorm:"not null;uniqueIndex:idx_article_category,priority:1;index" json:"article_id"`
	CategoryID uint `gorm:"not null;uniqueIndex:idx_article_category,priority:2" json:"category_id"`
}

func (ArticleCategory) TableName() string { return "article_category" }
