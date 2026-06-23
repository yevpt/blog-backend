package comment

import (
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *commentRepo) rootSnapshot(target Target) (RootSnapshot, error) {
	switch target.Type {
	case TargetArticle:
		return r.articleRootSnapshot(target.ID)
	case TargetMoment:
		return r.momentRootSnapshot(target.ID)
	default:
		return RootSnapshot{}, nil
	}
}

func (r *commentRepo) articleRootSnapshot(articleID uint) (RootSnapshot, error) {
	var article model.Article
	err := r.db.
		Select("id", "title", "short_content", "content").
		Where("id = ? AND status IN ?", articleID, readableArticleStatuses()).
		First(&article).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return RootSnapshot{}, ErrTargetNotFound
		}
		return RootSnapshot{}, err
	}
	return RootSnapshot{
		Type:    "article",
		ID:      article.ID,
		Title:   article.Title,
		Excerpt: articleExcerpt(article),
	}, nil
}

func (r *commentRepo) momentRootSnapshot(momentID uint) (RootSnapshot, error) {
	var moment model.Moment
	err := r.db.
		Select("id", "content").
		Where("id = ? AND status = ?", momentID, uint8(1)).
		First(&moment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return RootSnapshot{}, ErrTargetNotFound
		}
		return RootSnapshot{}, err
	}
	return RootSnapshot{
		Type:    "moment",
		ID:      moment.ID,
		Excerpt: strings.TrimSpace(moment.Content),
	}, nil
}

func articleExcerpt(article model.Article) string {
	if article.ShortContent != nil {
		if excerpt := strings.TrimSpace(*article.ShortContent); excerpt != "" {
			return excerpt
		}
	}
	return strings.TrimSpace(article.Content)
}
