package model

import "time"

type User struct {
	Base
	Username    string     `gorm:"size:155;not null;uniqueIndex;comment:登录账号" json:"username"`
	Password    string     `gorm:"size:255;not null;comment:密码（bcrypt）" json:"-"`
	Nickname    *string    `gorm:"size:150;comment:用户昵称" json:"nickname"`
	Email       *string    `gorm:"size:155;comment:绑定邮箱" json:"email"`
	Phone       *string    `gorm:"size:50;comment:绑定手机号" json:"phone"`
	Site        *string    `gorm:"size:500;comment:个人站点" json:"site"`
	AvatarUrl   *string    `gorm:"size:255;comment:头像URL" json:"avatar_url"`
	Mark        *string    `gorm:"size:200;default:注册会员;comment:身份标签" json:"mark"`
	Status      uint8      `gorm:"type:tinyint;default:1;comment:状态 0=禁用 1=正常" json:"status"`
	LastLoginAt *time.Time `gorm:"comment:最后登录时间" json:"last_login_at"`
}

func (User) TableName() string { return "user" }

type UserRole struct {
	ID     uint `gorm:"primarykey" json:"id"`
	UserID uint `gorm:"not null;uniqueIndex:idx_user_role,priority:1;index;comment:用户ID" json:"user_id"`
	RoleID uint `gorm:"not null;uniqueIndex:idx_user_role,priority:2;comment:角色ID" json:"role_id"`
}

func (UserRole) TableName() string { return "user_role" }

type UserLike struct {
	Base
	UserID   uint  `gorm:"not null;uniqueIndex:idx_user_like,priority:1;comment:用户ID" json:"user_id"`
	TargetID uint  `gorm:"not null;uniqueIndex:idx_user_like,priority:2;comment:目标ID" json:"target_id"`
	Type     uint8 `gorm:"type:tinyint;not null;uniqueIndex:idx_user_like,priority:3;comment:类型 1=文章 2=文章评论 3=文章评论回复 4=说说 5=留言板留言 6=说说评论 7=说说评论回复 8=留言回复" json:"type"`
}

func (UserLike) TableName() string { return "user_like" }
