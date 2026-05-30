package model

import "time"

// UserMeta 用户扩展信息，与 user 1:1，以 user_id 为主键
type UserMeta struct {
	UserID      uint       `gorm:"primarykey;comment:用户ID" json:"user_id"`
	Name        *string    `gorm:"size:155;comment:真实姓名" json:"name"`
	Description *string    `gorm:"size:1000;comment:个人简介" json:"description"`
	Gender      *uint8     `gorm:"type:tinyint;comment:性别 0=男 1=女" json:"gender"`
	Birthday    *time.Time `gorm:"type:date;comment:生日" json:"birthday"`
	IdCard      *string    `gorm:"size:60;comment:身份证号" json:"id_card"`
	Country     *string    `gorm:"size:40;default:中国;comment:国家" json:"country"`
	Province    *string    `gorm:"size:20;comment:省份" json:"province"`
	City        *string    `gorm:"size:50;comment:城市" json:"city"`
	Address     *string    `gorm:"size:200;comment:详细地址" json:"address"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (UserMeta) TableName() string { return "user_meta" }

// UserSetting 用户偏好设置，与 user 1:1，以 user_id 为主键
type UserSetting struct {
	UserID       uint  `gorm:"primarykey;comment:用户ID" json:"user_id"`
	MailShow     uint8 `gorm:"type:tinyint;default:1;comment:显示哪个邮箱 0=meta 1=user" json:"mail_show"`
	MailReceive  uint8 `gorm:"type:tinyint;default:1;comment:接收邮件用哪个邮箱 0=meta 1=user" json:"mail_receive"`
	DarkMode     uint8 `gorm:"type:tinyint;default:0;comment:暗黑模式 0=自动 1=亮 2=暗" json:"dark_mode"`
	ReceiveMail  bool  `gorm:"type:tinyint;default:1;comment:是否接收邮件" json:"receive_mail"`
	ShowName     bool  `gorm:"type:tinyint;default:0;comment:展示真实姓名" json:"show_name"`
	ShowAge      bool  `gorm:"type:tinyint;default:1;comment:展示年龄" json:"show_age"`
	ShowPhone    bool  `gorm:"type:tinyint;default:0;comment:展示手机号" json:"show_phone"`
	ShowQq       bool  `gorm:"type:tinyint;default:0;comment:展示QQ" json:"show_qq"`
	ShowWechat   bool  `gorm:"type:tinyint;default:0;comment:展示微信" json:"show_wechat"`
	ShowZhihu    bool  `gorm:"type:tinyint;default:0;comment:展示知乎" json:"show_zhihu"`
	ShowSina     bool  `gorm:"type:tinyint;default:0;comment:展示微博" json:"show_sina"`
	ShowBili     bool  `gorm:"type:tinyint;default:0;comment:展示B站" json:"show_bili"`
	ShowPosition bool  `gorm:"type:tinyint;default:1;comment:展示所在位置" json:"show_position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (UserSetting) TableName() string { return "user_setting" }

// UserSocialLink 用户社交平台账号，替代 user_meta 里的多个社交列
type UserSocialLink struct {
	Base
	UserID   uint   `gorm:"not null;uniqueIndex:idx_user_platform,priority:1;index;comment:用户ID" json:"user_id"`
	Platform string `gorm:"size:20;not null;uniqueIndex:idx_user_platform,priority:2;comment:平台标识 github/gitee/wechat/qq/bili/zhihu/sina/facebook/twitter" json:"platform"`
	URL      string `gorm:"size:500;not null;comment:账号链接或账号值" json:"url"`
}

func (UserSocialLink) TableName() string { return "user_social_link" }
