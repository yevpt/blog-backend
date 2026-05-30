package model

type SocialUser struct {
	Base
	UUID         string  `gorm:"size:256;not null;uniqueIndex:idx_social_source_uuid,priority:2;comment:第三方系统唯一ID" json:"uuid"`
	Source       string  `gorm:"size:20;not null;uniqueIndex:idx_social_source_uuid,priority:1;comment:来源平台 GITHUB/GITEE/QQ/WECHAT_OPEN/WEIBO/DOUYIN 等" json:"source"`
	AccessToken  string  `gorm:"size:256;comment:授权令牌" json:"-"`
	RefreshToken *string `gorm:"size:256;comment:刷新令牌" json:"-"`
	OpenID       *string `gorm:"size:256;comment:第三方 OpenID" json:"open_id"`
	IsActive     bool    `gorm:"type:tinyint;default:1;comment:是否有效" json:"is_active"`
}

func (SocialUser) TableName() string { return "social_user" }

type SocialUserAuth struct {
	Base
	UserID       uint `gorm:"not null;uniqueIndex:idx_social_auth,priority:1;index;comment:系统用户ID" json:"user_id"`
	SocialUserID uint `gorm:"not null;uniqueIndex:idx_social_auth,priority:2;comment:社会化用户ID" json:"social_user_id"`
}

func (SocialUserAuth) TableName() string { return "social_user_auth" }
