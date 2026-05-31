package repository

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
)

var errArticleMutationNotImplemented = errors.New("文章写入仓储尚未实现")

func (r *articleRepo) Save(data ArticleSaveData) (*ArticleAggregate, error) {
	return nil, errArticleMutationNotImplemented
}

func (r *articleRepo) SoftDelete(id uint) (*model.Article, error) {
	return nil, errArticleMutationNotImplemented
}

func (r *articleRepo) IncrementReadCount(id uint) (*model.Article, error) {
	return nil, errArticleMutationNotImplemented
}

func (r *articleRepo) ToggleLike(articleID uint, userID uint) (*ArticleAggregate, bool, error) {
	return nil, false, errArticleMutationNotImplemented
}
