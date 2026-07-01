package model

type Tag struct {
	Base
	Name        string  `gorm:"size:40;not null" json:"name"`
	URL         *string `gorm:"size:200" json:"url"`
	Icon        *string `gorm:"size:200" json:"icon"`
	Description *string `gorm:"size:500" json:"description"`
	CoverImgUrl *string `gorm:"size:200" json:"cover_img_url"`
	Seq         uint    `gorm:"type:int;default:0" json:"seq"`
}

func (Tag) TableName() string { return "tag" }

type ArticleTag struct {
	ID        uint `gorm:"primarykey" json:"id"`
	ArticleID uint `gorm:"not null;uniqueIndex:idx_article_tag,priority:1;index" json:"article_id"`
	TagID     uint `gorm:"not null;uniqueIndex:idx_article_tag,priority:2" json:"tag_id"`
	Seq       uint `gorm:"type:int;not null" json:"seq"`
}

func (ArticleTag) TableName() string { return "article_tag" }
