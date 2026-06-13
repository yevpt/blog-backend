package model

type FriendLink struct {
	Base
	Name        string  `gorm:"size:50;not null;comment:网站名称" json:"name"`
	Description *string `gorm:"size:150;comment:网站描述" json:"description"`
	Email       *string `gorm:"size:155;comment:站长邮箱" json:"email"`
	Phone       *string `gorm:"size:50;comment:联系电话" json:"phone"`
	Site        string  `gorm:"size:500;not null;comment:网站URL" json:"site"`
	AvatarUrl   *string `gorm:"size:255;comment:网站头像/Logo" json:"avatar_url"`
	Seq         uint    `gorm:"type:int;default:0;comment:排序" json:"seq"`
	Status      uint8   `gorm:"type:tinyint;default:1;comment:状态 0=隐藏 1=显示 2=失联" json:"status"`
}

func (FriendLink) TableName() string { return "friend_link" }
