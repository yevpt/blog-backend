package comment

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *commentRepo) ensureCommentableTarget(target Target) error {
	switch target.Type {
	case TargetArticle:
		return r.ensureArticleCommentable(target.ID)
	case TargetMoment:
		return r.ensureMomentCommentable(target.ID)
	case TargetGuestbook:
		return r.ensureGuestbookOwner(target.ID)
	default:
		return ErrTargetNotFound
	}
}

func (r *commentRepo) ensureArticleCommentable(articleID uint) error {
	var article model.Article
	err := r.db.
		Select("id", "comment_status").
		Where("id = ? AND status IN ?", articleID, []uint8{1, 2}).
		First(&article).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrTargetNotFound
	}
	if err != nil {
		return err
	}
	if article.CommentStatus == 0 {
		return ErrTargetCommentClosed
	}
	return nil
}

func (r *commentRepo) ensureArticleReadable(articleID uint) error {
	var count int64
	err := r.db.Model(&model.Article{}).
		Where("id = ? AND status IN ?", articleID, []uint8{1, 2}).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrTargetNotFound
	}
	return nil
}

func (r *commentRepo) ensureMomentCommentable(momentID uint) error {
	var moment model.Moment
	err := r.db.
		Select("id", "comment_status").
		Where("id = ? AND status = ?", momentID, uint8(1)).
		First(&moment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrTargetNotFound
	}
	if err != nil {
		return err
	}
	if moment.CommentStatus == 0 {
		return ErrTargetCommentClosed
	}
	return nil
}

func (r *commentRepo) ensureMomentReadable(momentID uint) error {
	var count int64
	err := r.db.Model(&model.Moment{}).
		Where("id = ? AND status = ?", momentID, uint8(1)).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrTargetNotFound
	}
	return nil
}

func (r *commentRepo) ensureGuestbookOwner(ownerUserID uint) error {
	var count int64
	err := r.db.Model(&model.User{}).
		Where("id = ? AND status = ?", ownerUserID, uint8(1)).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrTargetNotFound
	}
	return nil
}
