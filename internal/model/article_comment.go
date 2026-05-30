package model

type ArticleComment struct {
	Base
	ArticleID uint   `gorm:"not null;index;comment:文章ID" json:"article_id"`
	UserID    uint   `gorm:"not null;comment:评论者用户ID" json:"user_id"`
	Content   string `gorm:"size:2000;not null;comment:评论内容" json:"content"`
}

func (ArticleComment) TableName() string { return "article_comment" }
