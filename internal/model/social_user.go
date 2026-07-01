package model

type SocialUser struct {
	Base
	UUID         string  `gorm:"size:256;not null;uniqueIndex:idx_social_source_uuid,priority:2" json:"uuid"`
	Source       string  `gorm:"size:20;not null;uniqueIndex:idx_social_source_uuid,priority:1" json:"source"`
	AccessToken  string  `gorm:"size:256" json:"-"`
	RefreshToken *string `gorm:"size:256" json:"-"`
	OpenID       *string `gorm:"size:256" json:"open_id"`
	IsActive     bool    `gorm:"type:tinyint;default:1" json:"is_active"`
}

func (SocialUser) TableName() string { return "social_user" }

type SocialUserAuth struct {
	Base
	UserID       uint `gorm:"not null;uniqueIndex:idx_social_auth,priority:1;index" json:"user_id"`
	SocialUserID uint `gorm:"not null;uniqueIndex:idx_social_auth,priority:2" json:"social_user_id"`
}

func (SocialUserAuth) TableName() string { return "social_user_auth" }
