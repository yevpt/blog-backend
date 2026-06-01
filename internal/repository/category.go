package repository

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// ErrCategoryArticleMissing 表示请求中至少有一个文章 ID 不存在或已删除。
var ErrCategoryArticleMissing = errors.New("分类文章不存在")

// CategoryWithCount 分类及其下的公开文章数量。
type CategoryWithCount struct {
	model.Category
	ArticleCount int64 `gorm:"column:article_count"`
}

// CategoryUpdateData 分类更新数据；布尔字段表示对应属性是否参与更新。
type CategoryUpdateData struct {
	ParentID          *uint
	UpdateParentID    bool
	Name              *string
	URL               *string
	UpdateURL         bool
	Icon              *string
	UpdateIcon        bool
	Description       *string
	UpdateDescription bool
	CoverImgUrl       *string
	UpdateCoverImgUrl bool
	Seq               *uint
}

// CategoryRepository 分类数据访问接口。
type CategoryRepository interface {
	// ListWithArticleCount 查询所有未删除分类及其公开文章数量，
	// 按 seq ASC、article_count DESC、id ASC 排序。
	ListWithArticleCount() ([]CategoryWithCount, error)
	// Create 创建分类并返回含公开文章数量的分类信息。
	Create(category model.Category) (*CategoryWithCount, error)
	// Update 修改分类属性并返回含公开文章数量的分类信息。
	Update(id uint, data CategoryUpdateData) (*CategoryWithCount, error)
	// Delete 软删除分类，并清空该分类下的文章关联。
	Delete(id uint) (*model.Category, error)
	// AddArticles 将文章批量归入分类；文章原有分类关系会先被清空。
	AddArticles(categoryID uint, articleIDs []uint) (int64, error)
	// RemoveArticles 批量移除分类下的文章关联，不删除文章本身。
	RemoveArticles(categoryID uint, articleIDs []uint) (int64, error)
}

type categoryRepo struct {
	db *gorm.DB
}

// NewCategoryRepository 创建分类仓储实例。
func NewCategoryRepository(db *gorm.DB) CategoryRepository {
	return &categoryRepo{db: db}
}
