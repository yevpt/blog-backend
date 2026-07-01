package model

import "time"

// UserMeta 用户扩展信息，与 user 1:1，以 user_id 为主键
type UserMeta struct {
	UserID             uint       `gorm:"primarykey" json:"user_id"`
	Name               *string    `gorm:"size:155" json:"name"`
	Description        *string    `gorm:"size:1000" json:"description"`
	SubEmail           *string    `gorm:"size:155" json:"sub_email"`
	SubEmailVerifiedAt *time.Time `json:"sub_email_verified_at"`
	Gender             *uint8     `gorm:"type:tinyint" json:"gender"`
	Birthday           *time.Time `gorm:"type:date" json:"birthday"`
	IdCard             *string    `gorm:"size:60" json:"id_card"`
	Country            *string    `gorm:"size:40;default:中国" json:"country"`
	Province           *string    `gorm:"size:20" json:"province"`
	City               *string    `gorm:"size:50" json:"city"`
	Address            *string    `gorm:"size:200" json:"address"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (UserMeta) TableName() string { return "user_meta" }

// UserSetting 用户偏好设置，与 user 1:1，以 user_id 为主键
type UserSetting struct {
	UserID       uint      `gorm:"primarykey" json:"user_id"`
	MailShow     uint8     `gorm:"type:tinyint;default:1" json:"mail_show"`
	MailReceive  uint8     `gorm:"type:tinyint;default:1" json:"mail_receive"`
	DarkMode     uint8     `gorm:"type:tinyint;default:0" json:"dark_mode"`
	ReceiveMail  bool      `gorm:"type:tinyint;default:1" json:"receive_mail"`
	ShowName     bool      `gorm:"type:tinyint;default:0" json:"show_name"`
	ShowAge      bool      `gorm:"type:tinyint;default:1" json:"show_age"`
	ShowPhone    bool      `gorm:"type:tinyint;default:0" json:"show_phone"`
	ShowQq       bool      `gorm:"type:tinyint;default:0" json:"show_qq"`
	ShowWechat   bool      `gorm:"type:tinyint;default:0" json:"show_wechat"`
	ShowZhihu    bool      `gorm:"type:tinyint;default:0" json:"show_zhihu"`
	ShowSina     bool      `gorm:"type:tinyint;default:0" json:"show_sina"`
	ShowBili     bool      `gorm:"type:tinyint;default:0" json:"show_bili"`
	ShowPosition bool      `gorm:"type:tinyint;default:1" json:"show_position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (UserSetting) TableName() string { return "user_setting" }

// UserSocialLink 用户社交平台账号，替代 user_meta 里的多个社交列
type UserSocialLink struct {
	Base
	UserID   uint   `gorm:"not null;uniqueIndex:idx_user_platform,priority:1;index" json:"user_id"`
	Platform string `gorm:"size:20;not null;uniqueIndex:idx_user_platform,priority:2" json:"platform"`
	URL      string `gorm:"size:500;not null" json:"url"`
}

func (UserSocialLink) TableName() string { return "user_social_link" }
