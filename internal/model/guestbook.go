package model

// Guestbook 留言板，记录访客对某个用户的留言
type Guestbook struct {
	Base
	OwnerUserID uint   `gorm:"not null;index;comment:被留言的用户ID" json:"owner_user_id"`
	FromUserID  uint   `gorm:"not null;comment:留言者用户ID" json:"from_user_id"`
	Content     string `gorm:"size:2000;not null;comment:留言内容" json:"content"`
}

func (Guestbook) TableName() string { return "guestbook" }
