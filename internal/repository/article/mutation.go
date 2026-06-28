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
		}
		if data.PrepareArticle != nil {
			prepared, err := data.PrepareArticle(data.Article)
			if err != nil {
				return err
			}
			data.Article = prepared
		}
		res := tx.Model(&model.Article{}).
			Where("id = ?", data.Article.ID).
			Updates(articleUpdateFields(data.Article))
		if res.Error != nil {
			return res.Error
		}
		articleID = data.Article.ID

		if err := replaceArticleCategories(tx, articleID, data.CategoryIDs); err != nil {
			return err
		}
		if err := replaceArticleTags(tx, articleID, data.Tags); err != nil {
			return err
		}
		if err := replaceArticleMusic(tx, articleID, data.MusicIDs); err != nil {
			return err
		}
		if err := replaceArticleAiModels(tx, articleID, data.AiModels); err != nil {
			return err
		}
		return replaceArticleRecommend(tx, articleID, data.Recommend)
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

func (r *articleRepo) FindDeletedByID(id uint) (*model.Article, error) {
	var article model.Article
	err := r.db.Unscoped().Where("id = ?", id).First(&article).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &article, nil
}

func (r *articleRepo) PermanentDelete(id uint, operatorID uint) (*model.Article, error) {
	var article model.Article
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			First(&article).Error; err != nil {
			return err
		}
		if article.UserID != operatorID {
			return ErrNoDeletePermission
		}
		if !article.DeletedAt.Valid {
			return ErrArticleNotSoftDeleted
		}

		commentIDs, err := articleCommentIDs(tx, id)
		if err != nil {
			return err
		}
		replyIDs, err := articleReplyIDs(tx, commentIDs)
		if err != nil {
			return err
		}
		if err := hardDeleteArticleLikes(tx, id, commentIDs, replyIDs); err != nil {
			return err
		}
		if err := hardDeleteArticleRelations(tx, id); err != nil {
			return err
		}
		if len(commentIDs) > 0 {
			if err := tx.Unscoped().Where("comment_id IN ?", commentIDs).Delete(&model.ArticleCommentReply{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Unscoped().Where("article_id = ?", id).Delete(&model.ArticleComment{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&article).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &article, nil
}

func (r *articleRepo) IncrementReadCount(id uint) (*model.Article, error) {
	var article model.Article
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Article{}).
			Where("id = ?", id).
			Where("status IN ?", visibleArticleStatuses()).
			UpdateColumn("read_count", gorm.Expr("read_count + 1"))
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

func articleCommentIDs(tx *gorm.DB, articleID uint) ([]uint, error) {
	var ids []uint
	err := tx.Unscoped().
		Model(&model.ArticleComment{}).
		Where("article_id = ?", articleID).
		Pluck("id", &ids).Error
	return ids, err
}

func articleReplyIDs(tx *gorm.DB, commentIDs []uint) ([]uint, error) {
	if len(commentIDs) == 0 {
		return nil, nil
	}
	var ids []uint
	err := tx.Unscoped().
		Model(&model.ArticleCommentReply{}).
		Where("comment_id IN ?", commentIDs).
		Pluck("id", &ids).Error
	return ids, err
}

func hardDeleteArticleLikes(tx *gorm.DB, articleID uint, commentIDs []uint, replyIDs []uint) error {
	if err := tx.Unscoped().
		Where("target_id = ? AND type = ?", articleID, ArticleLikeType).
		Delete(&model.UserLike{}).Error; err != nil {
		return err
	}
	if len(commentIDs) > 0 {
		if err := tx.Unscoped().
			Where("target_id IN ? AND type = ?", commentIDs, uint8(2)).
			Delete(&model.UserLike{}).Error; err != nil {
			return err
		}
	}
	if len(replyIDs) > 0 {
		if err := tx.Unscoped().
			Where("target_id IN ? AND type = ?", replyIDs, uint8(3)).
			Delete(&model.UserLike{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func hardDeleteArticleRelations(tx *gorm.DB, articleID uint) error {
	if err := tx.Where("article_id = ?", articleID).Delete(&model.ArticleCategory{}).Error; err != nil {
		return err
	}
	if err := tx.Where("article_id = ?", articleID).Delete(&model.ArticleTag{}).Error; err != nil {
		return err
	}
	if err := tx.Where("article_id = ?", articleID).Delete(&model.ArticleMusic{}).Error; err != nil {
		return err
	}
	if err := tx.Where("article_id = ?", articleID).Delete(&model.ArticleAiModel{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("article_id = ?", articleID).Delete(&model.ArticleRecommend{}).Error
}

func (r *articleRepo) ToggleLike(articleID uint, userID uint) (*ArticleAggregate, bool, error) {
	liked := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var article model.Article
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status IN ?", visibleArticleStatuses()).
			First(&article, articleID).Error; err != nil {
			return err
		}

		var like model.UserLike
		err := tx.Unscoped().
			Where("target_id = ? AND user_id = ? AND type = ?", articleID, userID, ArticleLikeType).
			First(&like).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			liked = true
			// 点赞通知改由 service 层发布 notification_event，仓储不再写旧 message。
			return tx.Create(&model.UserLike{UserID: userID, TargetID: articleID, Type: ArticleLikeType}).Error
		}
		if err != nil {
			return err
		}
		liked = false
		return tx.Unscoped().Delete(&like).Error
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
	return []uint{uint(model.ArticleStatusPublic), uint(model.ArticleStatusEncrypted)}
}

func articleUpdateFields(article model.Article) map[string]any {
	return map[string]any{
		"title":          article.Title,
		"cover_img_url":        article.CoverImgUrl,
		"mobile_cover_img_url": article.MobileCoverImgUrl,
		"short_content":  article.ShortContent,
		"content":        article.Content,
		"user_id":        article.UserID,
		"status":         article.Status,
		"comment_status":        article.CommentStatus,
		"password":              article.Password,
		"cover_ai_generated":    article.CoverAiGenerated,
		"content_ai_referenced": article.ContentAiReferenced,
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

func replaceArticleTags(tx *gorm.DB, articleID uint, tags []ArticleTagSaveData) error {
	if err := tx.Where("article_id = ?", articleID).Delete(&model.ArticleTag{}).Error; err != nil {
		return err
	}
	rows := make([]model.ArticleTag, 0, len(tags))
	for _, tag := range tags {
		rows = append(rows, model.ArticleTag{ArticleID: articleID, TagID: tag.TagID, Seq: tag.Seq})
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

func replaceArticleAiModels(tx *gorm.DB, articleID uint, models []ArticleAiModelSaveData) error {
	if err := tx.Where("article_id = ?", articleID).Delete(&model.ArticleAiModel{}).Error; err != nil {
		return err
	}
	rows := make([]model.ArticleAiModel, 0, len(models))
	for _, item := range models {
		rows = append(rows, model.ArticleAiModel{
			ArticleID: articleID,
			Scope:     item.Scope,
			ModelName: item.ModelName,
			Seq:       item.Seq,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func replaceArticleRecommend(tx *gorm.DB, articleID uint, recommend bool) error {
	if !recommend {
		return tx.Where("article_id = ?", articleID).Delete(&model.ArticleRecommend{}).Error
	}

	var existing model.ArticleRecommend
	err := tx.Where("article_id = ?", articleID).First(&existing).Error
	if err == nil {
		// 已处于推荐状态：编辑保存时不改动 seq。
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	nextSeq, err := nextRecommendSeq(tx)
	if err != nil {
		return err
	}

	var deleted model.ArticleRecommend
	err = tx.Unscoped().Where("article_id = ?", articleID).First(&deleted).Error
	if err == nil {
		return tx.Unscoped().Model(&deleted).Updates(map[string]any{
			"seq":        nextSeq,
			"deleted_at": nil,
			"updated_at": gorm.Expr("NOW()"),
		}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return tx.Create(&model.ArticleRecommend{ArticleID: articleID, Seq: nextSeq}).Error
}
