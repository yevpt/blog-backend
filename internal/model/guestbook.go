package model

// Guestbook 留言板，记录访客对某个用户的留言
type Guestbook struct {
	Base
	OwnerUserID uint   `gorm:"not null;index" json:"owner_user_id"`
	FromUserID  uint   `gorm:"not null" json:"from_user_id"`
	Content     string `gorm:"size:2000;not null" json:"content"`
}

func (Guestbook) TableName() string { return "guestbook" }
