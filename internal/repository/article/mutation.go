package article

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *articleRepo) Save(data ArticleSaveData) (*ArticleAggregate, error) {
	var articleID uint
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if data.Article.ID == 0 {
			if err := tx.Create(&data.Article).Error; err != nil {
				return err
			}
		} else {
			var existing model.Article
			if err := tx.Select("id").First(&existing, data.Article.ID).Error; err != nil {
				return err
			}
			res := tx.Model(&model.Article{}).
				Where("id = ?", data.Article.ID).
				Updates(articleUpdateFields(data.Article))
			if res.Error != nil {
				return res.Error
			}
		}
		articleID = data.Article.ID

		if err := replaceArticleCategories(tx, articleID, data.CategoryIDs); err != nil {
			return err
		}
		if err := replaceArticleTags(tx, articleID, data.TagIDs); err != nil {
			return err
		}
		if err := replaceArticleMusic(tx, articleID, data.MusicIDs); err != nil {
			return err
		}
		return replaceArticleRecommend(tx, articleID, data.Recommend, data.RecommendSeq)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.FindAdminDetail(articleID, nil)
}

func (r *articleRepo) SoftDelete(id uint) (*model.Article, error) {
	var article model.Article
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&article, id).Error; err != nil {
			return err
		}
		return tx.Delete(&article).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &article, err
}

func (r *articleRepo) IncrementReadCount(id uint) (*model.Article, error) {
	var article model.Article
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Article{}).
			Where("id = ?", id).
			Where("status IN ?", visibleArticleStatuses()).
			Update("read_count", gorm.Expr("read_count + 1"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.First(&article, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &article, err
}

func (r *articleRepo) ToggleLike(articleID uint, userID uint) (*ArticleAggregate, bool, error) {
	liked := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var article model.Article
		if err := tx.Where("status IN ?", visibleArticleStatuses()).First(&article, articleID).Error; err != nil {
			return err
		}

		var like model.UserLike
		err := tx.Unscoped().
			Where("target_id = ? AND user_id = ? AND type = ?", articleID, userID, ArticleLikeType).
			First(&like).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			liked = true
			if err := tx.Create(&model.UserLike{UserID: userID, TargetID: articleID, Type: ArticleLikeType}).Error; err != nil {
				return err
			}
			return createArticleLikeMessage(tx, article, userID)
		}
		if err != nil {
			return err
		}
		if like.DeletedAt.Valid {
			liked = true
			if err := tx.Unscoped().Model(&like).Update("deleted_at", nil).Error; err != nil {
				return err
			}
			return createArticleLikeMessage(tx, article, userID)
		}

		liked = false
		return tx.Delete(&like).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	detail, err := r.FindPublicDetail(articleID, &userID)
	return detail, liked, err
}

func visibleArticleStatuses() []uint {
	return []uint{1, 2}
}

func createArticleLikeMessage(tx *gorm.DB, article model.Article, fromUserID uint) error {
	if article.UserID == fromUserID {
		return nil
	}
	title := "文章点赞"
	content := ""
	message := model.Message{
		Title:      &title,
		Content:    &content,
		Type:       "article_like",
		TypeID:     article.ID,
		FromUserID: fromUserID,
		ArticleID:  &article.ID,
	}
	if err := tx.Create(&message).Error; err != nil {
		return err
	}
	return tx.Create(&model.UserMessage{
		UserID:    article.UserID,
		MessageID: message.ID,
		IsRead:    false,
	}).Error
}

func articleUpdateFields(article model.Article) map[string]interface{} {
	return map[string]interface{}{
		"title":          article.Title,
		"cover_img_url":  article.CoverImgUrl,
		"short_content":  article.ShortContent,
		"content":        article.Content,
		"user_id":        article.UserID,
		"status":         article.Status,
		"comment_status": article.CommentStatus,
		"password":       article.Password,
	}
}

func replaceArticleCategories(tx *gorm.DB, articleID uint, categoryIDs []uint) error {
	if err := tx.Where("article_id = ?", articleID).Delete(&model.ArticleCategory{}).Error; err != nil {
		return err
	}
	rows := make([]model.ArticleCategory, 0, len(categoryIDs))
	for _, categoryID := range categoryIDs {
		rows = append(rows, model.ArticleCategory{ArticleID: articleID, CategoryID: categoryID})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func replaceArticleTags(tx *gorm.DB, articleID uint, tagIDs []uint) error {
	if err := tx.Where("article_id = ?", articleID).Delete(&model.ArticleTag{}).Error; err != nil {
		return err
	}
	rows := make([]model.ArticleTag, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		rows = append(rows, model.ArticleTag{ArticleID: articleID, TagID: tagID})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func replaceArticleMusic(tx *gorm.DB, articleID uint, musicIDs []uint) error {
	if err := tx.Where("article_id = ?", articleID).Delete(&model.ArticleMusic{}).Error; err != nil {
		return err
	}
	rows := make([]model.ArticleMusic, 0, len(musicIDs))
	for _, musicID := range musicIDs {
		rows = append(rows, model.ArticleMusic{ArticleID: articleID, MusicID: musicID})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func replaceArticleRecommend(tx *gorm.DB, articleID uint, recommend bool, seq uint) error {
	if !recommend {
		return tx.Where("article_id = ?", articleID).Delete(&model.ArticleRecommend{}).Error
	}
	row := model.ArticleRecommend{ArticleID: articleID, Seq: seq}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "article_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"seq":        seq,
			"deleted_at": nil,
			"updated_at": gorm.Expr("NOW()"),
		}),
	}).Create(&row).Error
}
