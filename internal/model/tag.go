package model

type Tag struct {
	Base
	Name        string  `gorm:"size:40;not null;comment:标签名" json:"name"`
	URL         *string `gorm:"size:200;comment:标签路由别名" json:"url"`
	Icon        *string `gorm:"size:200;comment:图标URL" json:"icon"`
	Description *string `gorm:"size:500;comment:标签描述" json:"description"`
	CoverImgUrl *string `gorm:"size:200;comment:封面图URL" json:"cover_img_url"`
	Seq         uint    `gorm:"type:int;default:0;comment:排序" json:"seq"`
}

func (Tag) TableName() string { return "tag" }

type ArticleTag struct {
	ID        uint `gorm:"primarykey" json:"id"`
	ArticleID uint `gorm:"not null;uniqueIndex:idx_article_tag,priority:1;index;comment:文章ID" json:"article_id"`
	TagID     uint `gorm:"not null;uniqueIndex:idx_article_tag,priority:2;comment:标签ID" json:"tag_id"`
}

func (ArticleTag) TableName() string { return "article_tag" }
