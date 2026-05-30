package model

import (
	"time"

	"gorm.io/gorm"
)

// Base 是所有数据库模型的公共字段
// 使用 GORM 的软删除：删除时设置 DeletedAt 而不是真正删除数据
type Base struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}
