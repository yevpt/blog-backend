package model

import (
	"time"

	"gorm.io/gorm"
)

// Base 所有数据库模型的公共字段，DeletedAt 启用 GORM 软删除（删除时置位而非真删）
type Base struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}
