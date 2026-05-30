package model

type MomentComment struct {
	Base
	MomentID uint   `gorm:"not null;index;comment:说说ID" json:"moment_id"`
	UserID   uint   `gorm:"not null;comment:评论者用户ID" json:"user_id"`
	Content  string `gorm:"size:2000;not null;comment:评论内容" json:"content"`
}

func (MomentComment) TableName() string { return "moment_comment" }
