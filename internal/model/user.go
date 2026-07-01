package model

import "time"

type User struct {
	Base
	Username        string     `gorm:"size:155;not null;uniqueIndex" json:"username"`
	Password        string     `gorm:"size:255;not null" json:"-"`
	PasswordSet     bool       `gorm:"type:tinyint;not null;default:1" json:"password_set"`
	Nickname        *string    `gorm:"size:150" json:"nickname"`
	Email           *string    `gorm:"size:155" json:"email"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	Phone           *string    `gorm:"size:50" json:"phone"`
	Site            *string    `gorm:"size:500" json:"site"`
	AvatarUrl       *string    `gorm:"size:255" json:"avatar_url"`
	Mark            *string    `gorm:"size:200;default:注册会员" json:"mark"`
	Status          uint8      `gorm:"type:tinyint;default:1" json:"status"`
	LastLoginAt     *time.Time `json:"last_login_at"`
	LastActiveAt    *time.Time `json:"last_active_at"`
}

func (User) TableName() string { return "user" }

type UserRole struct {
	ID     uint `gorm:"primarykey" json:"id"`
	UserID uint `gorm:"not null;uniqueIndex:idx_user_role,priority:1;index" json:"user_id"`
	RoleID uint `gorm:"not null;uniqueIndex:idx_user_role,priority:2" json:"role_id"`
}

func (UserRole) TableName() string { return "user_role" }

type UserLike struct {
	Base
	UserID   uint  `gorm:"not null;uniqueIndex:idx_user_like,priority:1" json:"user_id"`
	TargetID uint  `gorm:"not null;uniqueIndex:idx_user_like,priority:2" json:"target_id"`
	Type     uint8 `gorm:"type:tinyint;not null;uniqueIndex:idx_user_like,priority:3" json:"type"`
}

func (UserLike) TableName() string { return "user_like" }
