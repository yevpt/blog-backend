package repository

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Create 创建标签并返回最新标签信息。
func (r *tagRepo) Create(tag model.Tag) (*TagWithCount, error) {
	if err := r.db.Create(&tag).Error; err != nil {
		return nil, err
	}
	return r.FindWithArticleCount(tag.ID)
}

// Update 修改标签属性并返回最新标签信息。
func (r *tagRepo) Update(id uint, data TagUpdateData) (*TagWithCount, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var tag model.Tag
		if err := tx.First(&tag, id).Error; err != nil {
			return err
		}

		fields := tagUpdateFields(data)
		if len(fields) == 0 {
			return nil
		}
		return tx.Model(&model.Tag{}).Where("id = ?", id).Updates(fields).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.FindWithArticleCount(id)
}

// Delete 软删除标签，并清空标签下文章关联。
func (r *tagRepo) Delete(id uint) (*model.Tag, error) {
	var tag model.Tag
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&tag, id).Error; err != nil {
			return err
		}
		if err := tx.Where("tag_id = ?", id).Delete(&model.ArticleTag{}).Error; err != nil {
			return err
		}
		return tx.Delete(&tag).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &tag, err
}

// AddArticles 批量给文章添加标签；已有标签关系会被跳过。
func (r *tagRepo) AddArticles(tagID uint, articleIDs []uint) (int64, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureTagExists(tx, tagID); err != nil {
			return err
		}
		if err := ensureTagArticlesExist(tx, articleIDs); err != nil {
			return err
		}
		rows := make([]model.ArticleTag, 0, len(articleIDs))
		for _, articleID := range articleIDs {
			rows = append(rows, model.ArticleTag{ArticleID: articleID, TagID: tagID})
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows)
		affected = res.RowsAffected
		return res.Error
	})
	return affected, err
}

// RemoveArticles 批量移除标签下文章关联，不删除文章。
func (r *tagRepo) RemoveArticles(tagID uint, articleIDs []uint) (int64, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureTagExists(tx, tagID); err != nil {
			return err
		}
		res := tx.Where("tag_id = ? AND article_id IN ?", tagID, articleIDs).
			Delete(&model.ArticleTag{})
		affected = res.RowsAffected
		return res.Error
	})
	return affected, err
}

func tagUpdateFields(data TagUpdateData) map[string]any {
	fields := make(map[string]any)
	if data.Name != nil {
		fields["name"] = *data.Name
	}
	if data.UpdateURL {
		fields["url"] = data.URL
	}
	if data.UpdateIcon {
		fields["icon"] = data.Icon
	}
	if data.UpdateDescription {
		fields["description"] = data.Description
	}
	if data.UpdateCoverImgUrl {
		fields["cover_img_url"] = data.CoverImgUrl
	}
	if data.Seq != nil {
		fields["seq"] = *data.Seq
	}
	return fields
}

func ensureTagExists(tx *gorm.DB, tagID uint) error {
	var tag model.Tag
	return tx.First(&tag, tagID).Error
}

func ensureTagArticlesExist(tx *gorm.DB, articleIDs []uint) error {
	var count int64
	if err := tx.Model(&model.Article{}).Where("id IN ?", articleIDs).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(articleIDs)) {
		return ErrTagArticleMissing
	}
	return nil
}
