package repository

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// ErrTagArticleMissing 表示请求中至少有一个文章 ID 不存在或已删除。
var ErrTagArticleMissing = errors.New("标签文章不存在")

// TagWithCount 标签及其下的公开文章数量。
type TagWithCount struct {
	model.Tag
	ArticleCount int64 `gorm:"column:article_count"`
}

// TagUpdateData 标签更新数据；布尔字段表示对应属性是否参与更新。
type TagUpdateData struct {
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

// TagRepository 标签数据访问接口。
type TagRepository interface {
	// ListWithArticleCount 查询所有未删除标签及其公开文章数量。
	ListWithArticleCount() ([]TagWithCount, error)
	// FindWithArticleCount 查询单个标签及其公开文章数量。
	FindWithArticleCount(id uint) (*TagWithCount, error)
	// Create 创建标签并返回含公开文章数量的标签信息。
	Create(tag model.Tag) (*TagWithCount, error)
	// Update 修改标签属性并返回含公开文章数量的标签信息。
	Update(id uint, data TagUpdateData) (*TagWithCount, error)
	// Delete 软删除标签，并清空该标签下的文章关联。
	Delete(id uint) (*model.Tag, error)
	// AddArticles 给单篇或多篇文章添加标签，已有关系会被跳过。
	AddArticles(tagID uint, articleIDs []uint) (int64, error)
	// RemoveArticles 批量移除标签下的文章关联，不删除文章本身。
	RemoveArticles(tagID uint, articleIDs []uint) (int64, error)
}

type tagRepo struct {
	db *gorm.DB
}

// NewTagRepository 创建标签仓储实例。
func NewTagRepository(db *gorm.DB) TagRepository {
	return &tagRepo{db: db}
}
