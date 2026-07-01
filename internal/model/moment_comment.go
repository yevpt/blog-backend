package model

type MomentComment struct {
	Base
	MomentID uint   `gorm:"not null;index" json:"moment_id"`
	UserID   uint   `gorm:"not null" json:"user_id"`
	Content  string `gorm:"size:2000;not null" json:"content"`
}

func (MomentComment) TableName() string { return "moment_comment" }
