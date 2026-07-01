package model

type ArticleComment struct {
	Base
	ArticleID uint   `gorm:"not null;index" json:"article_id"`
	UserID    uint   `gorm:"not null" json:"user_id"`
	Content   string `gorm:"size:2000;not null" json:"content"`
}

func (ArticleComment) TableName() string { return "article_comment" }
