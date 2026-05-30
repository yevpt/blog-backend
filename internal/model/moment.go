package model

type Moment struct {
	Base
	UserID        uint   `gorm:"not null;index;comment:作者ID" json:"user_id"`
	Content       string `gorm:"size:800;not null;comment:说说内容" json:"content"`
	Status        uint8  `gorm:"type:tinyint;default:1;comment:状态 0=隐藏 1=公开" json:"status"`
	CommentStatus uint8  `gorm:"type:tinyint;default:1;comment:评论状态 0=关闭 1=开启" json:"comment_status"`
	ReadCount     uint   `gorm:"type:int;default:0;comment:阅读数" json:"read_count"`
	IsTop         bool   `gorm:"type:tinyint;default:0;comment:是否置顶" json:"is_top"`
}

func (Moment) TableName() string { return "moment" }
