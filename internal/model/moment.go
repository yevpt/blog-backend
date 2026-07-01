package model

type Moment struct {
	Base
	UserID        uint   `gorm:"not null;index" json:"user_id"`
	Content       string `gorm:"size:800;not null" json:"content"`
	Status        uint8  `gorm:"type:tinyint;default:1" json:"status"`
	CommentStatus uint8  `gorm:"type:tinyint;default:1" json:"comment_status"`
	ReadCount     uint   `gorm:"type:int;default:0" json:"read_count"`
	IsTop         bool   `gorm:"type:tinyint;default:0" json:"is_top"`
}

func (Moment) TableName() string { return "moment" }
