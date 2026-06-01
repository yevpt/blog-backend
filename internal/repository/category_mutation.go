package repository

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// Create 创建分类并返回最新分类信息。
func (r *categoryRepo) Create(category model.Category) (*CategoryWithCount, error) {
	if err := r.db.Create(&category).Error; err != nil {
		return nil, err
	}
	return r.findWithArticleCount(category.ID)
}

// Update 修改分类属性并返回最新分类信息。
func (r *categoryRepo) Update(id uint, data CategoryUpdateData) (*CategoryWithCount, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var category model.Category
		if err := tx.First(&category, id).Error; err != nil {
			return err
		}

		fields := categoryUpdateFields(data)
		if len(fields) == 0 {
			return nil
		}
		return tx.Model(&model.Category{}).Where("id = ?", id).Updates(fields).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.findWithArticleCount(id)
}

// Delete 软删除分类，并清空分类下文章关联。
func (r *categoryRepo) Delete(id uint) (*model.Category, error) {
	var category model.Category
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&category, id).Error; err != nil {
			return err
		}
		if err := tx.Where("category_id = ?", id).Delete(&model.ArticleCategory{}).Error; err != nil {
			return err
		}
		return tx.Delete(&category).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &category, err
}

// AddArticles 批量把文章归入分类；文章已有分类关系会整体迁移到当前分类。
func (r *categoryRepo) AddArticles(categoryID uint, articleIDs []uint) (int64, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureCategoryExists(tx, categoryID); err != nil {
			return err
		}
		if err := ensureArticlesExist(tx, articleIDs); err != nil {
			return err
		}
		if err := tx.Where("article_id IN ?", articleIDs).Delete(&model.ArticleCategory{}).Error; err != nil {
			return err
		}
		rows := make([]model.ArticleCategory, 0, len(articleIDs))
		for _, articleID := range articleIDs {
			rows = append(rows, model.ArticleCategory{ArticleID: articleID, CategoryID: categoryID})
		}
		res := tx.Create(&rows)
		affected = res.RowsAffected
		return res.Error
	})
	return affected, err
}

// RemoveArticles 批量移除分类下文章关联，不删除文章。
func (r *categoryRepo) RemoveArticles(categoryID uint, articleIDs []uint) (int64, error) {
	var affected int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureCategoryExists(tx, categoryID); err != nil {
			return err
		}
		res := tx.Where("category_id = ? AND article_id IN ?", categoryID, articleIDs).
			Delete(&model.ArticleCategory{})
		affected = res.RowsAffected
		return res.Error
	})
	return affected, err
}

func categoryUpdateFields(data CategoryUpdateData) map[string]any {
	fields := make(map[string]any)
	if data.UpdateParentID {
		fields["parent_id"] = data.ParentID
	}
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

func ensureCategoryExists(tx *gorm.DB, categoryID uint) error {
	var category model.Category
	return tx.First(&category, categoryID).Error
}

func ensureArticlesExist(tx *gorm.DB, articleIDs []uint) error {
	var count int64
	if err := tx.Model(&model.Article{}).Where("id IN ?", articleIDs).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(articleIDs)) {
		return ErrCategoryArticleMissing
	}
	return nil
}
