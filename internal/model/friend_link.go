package model

type FriendLink struct {
	Base
	Name        string  `gorm:"size:50;not null" json:"name"`
	Description *string `gorm:"size:150" json:"description"`
	Email       *string `gorm:"size:155" json:"email"`
	Phone       *string `gorm:"size:50" json:"phone"`
	Site        string  `gorm:"size:500;not null" json:"site"`
	AvatarUrl   *string `gorm:"size:255" json:"avatar_url"`
	Seq         uint    `gorm:"type:int;default:0" json:"seq"`
	Status      uint8   `gorm:"type:tinyint;default:1" json:"status"`
}

func (FriendLink) TableName() string { return "friend_link" }
