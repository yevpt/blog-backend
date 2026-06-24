package music

import (
	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// MusicRepository 音乐数据访问接口。
type MusicRepository interface {
	// List 查询所有未删除音乐，按 seq ASC、id ASC 排序。
	List() ([]model.Music, error)
}

type musicRepo struct {
	db *gorm.DB
}

// NewMusicRepository 创建音乐仓储实例。
func NewMusicRepository(db *gorm.DB) MusicRepository {
	return &musicRepo{db: db}
}

func (r *musicRepo) List() ([]model.Music, error) {
	var rows []model.Music
	err := r.db.Model(&model.Music{}).
		Order("seq ASC").
		Order("id ASC").
		Find(&rows).Error
	return rows, err
}
