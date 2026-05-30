package model

type Role struct {
	ID   uint   `gorm:"primarykey" json:"id"`
	Name string `gorm:"size:30;not null;comment:角色名称" json:"name"`
}

func (Role) TableName() string { return "role" }
