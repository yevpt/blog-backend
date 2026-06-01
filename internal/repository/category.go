package repository

import (
	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// CategoryWithCount 分类及其下的公开文章数量。
type CategoryWithCount struct {
	model.Category
	ArticleCount int64 `gorm:"column:article_count"`
}

// CategoryRepository 分类数据访问接口。
type CategoryRepository interface {
	// ListWithArticleCount 查询所有未删除分类及其公开文章数量，
	// 按 seq ASC、article_count DESC、id ASC 排序。
	ListWithArticleCount() ([]CategoryWithCount, error)
}

type categoryRepo struct {
	db *gorm.DB
}

// NewCategoryRepository 创建分类仓储实例。
func NewCategoryRepository(db *gorm.DB) CategoryRepository {
	return &categoryRepo{db: db}
}

func (r *categoryRepo) ListWithArticleCount() ([]CategoryWithCount, error) {
	var rows []CategoryWithCount
	err := r.db.Table("category").
		Select("category.*, COUNT(DISTINCT article.id) AS article_count").
		Joins("LEFT JOIN article_category ON article_category.category_id = category.id").
		Joins("LEFT JOIN article ON article.id = article_category.article_id AND article.status = 1 AND article.deleted_at IS NULL").
		Where("category.deleted_at IS NULL").
		Group("category.id").
		Order("category.seq ASC").
		Order("article_count DESC").
		Order("category.id ASC").
		Find(&rows).Error
	return rows, err
}
