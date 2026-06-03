package moment

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *momentRepo) Save(data SaveData) (*MomentAggregate, error) {
	if err := r.ensureAuthorExists(data.Moment.UserID); err != nil {
		return nil, err
	}

	var momentID uint
	err := r.db.Transaction(func(tx *gorm.DB) error {
		repo := &momentRepo{db: tx}
		if data.Moment.ID == 0 {
			if err := tx.Create(&data.Moment).Error; err != nil {
				return err
			}
		} else {
			existing, err := repo.findMomentForMutation(data.Moment.ID)
			if err != nil {
				return err
			}
			if !data.Force && existing.UserID != data.OperatorID {
				return ErrNoPermission
			}
			if err := tx.Model(&model.Moment{}).
				Where("id = ?", data.Moment.ID).
				Updates(momentUpdateFields(data.Moment)).Error; err != nil {
				return err
			}
		}
		momentID = data.Moment.ID

		if err := tx.Where("owner_id = ? AND owner_type = ?", momentID, MomentMediaOwnerType).Delete(&model.Media{}).Error; err != nil {
			return err
		}
		images := prepareImages(data.Moment, data.Images)
		if len(images) == 0 {
			return nil
		}
		return tx.Create(&images).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMomentNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.findAnyDetail(momentID, nil)
}

func (r *momentRepo) Delete(id uint, operatorID uint, force bool) (*model.Moment, error) {
	var moment model.Moment
	err := r.db.Transaction(func(tx *gorm.DB) error {
		repo := &momentRepo{db: tx}
		found, err := repo.findMomentForMutation(id)
		if err != nil {
			return err
		}
		if !force && found.UserID != operatorID {
			return ErrNoPermission
		}
		moment = *found
		return tx.Delete(&moment).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMomentNotFound
	}
	return &moment, err
}

func (r *momentRepo) SetTop(id uint, operatorID uint, force bool) (*model.Moment, error) {
	return r.updateTop(id, operatorID, force, true)
}

func (r *momentRepo) RemoveTop(id uint, operatorID uint, force bool) (*model.Moment, error) {
	return r.updateTop(id, operatorID, force, false)
}

func (r *momentRepo) IncrementReadCount(id uint) (*model.Moment, error) {
	var moment model.Moment
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Moment{}).
			Where("id = ? AND status = ?", id, uint8(1)).
			Update("read_count", gorm.Expr("read_count + 1"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.First(&moment, id).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMomentNotFound
	}
	return &moment, err
}

func (r *momentRepo) IsLiked(id uint, userID uint) (bool, int64, error) {
	var count int64
	if err := r.db.Model(&model.Moment{}).
		Where("id = ? AND status = ?", id, uint8(1)).
		Count(&count).Error; err != nil {
		return false, 0, err
	}
	if count == 0 {
		return false, 0, ErrMomentNotFound
	}

	var likedCount int64
	if err := r.db.Model(&model.UserLike{}).
		Where("target_id = ? AND user_id = ? AND type = ?", id, userID, MomentLikeType).
		Count(&likedCount).Error; err != nil {
		return false, 0, err
	}
	total, err := r.countLikes(id)
	return likedCount > 0, total, err
}

func (r *momentRepo) ToggleLike(id uint, userID uint) (*MomentAggregate, bool, error) {
	liked := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var moment model.Moment
		if err := tx.Where("status = ?", uint8(1)).First(&moment, id).Error; err != nil {
			return err
		}

		var like model.UserLike
		err := tx.Unscoped().
			Where("target_id = ? AND user_id = ? AND type = ?", id, userID, MomentLikeType).
			First(&like).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			liked = true
			if err := tx.Create(&model.UserLike{UserID: userID, TargetID: id, Type: MomentLikeType}).Error; err != nil {
				return err
			}
			return createMomentLikeMessage(tx, moment, userID)
		}
		if err != nil {
			return err
		}
		if like.DeletedAt.Valid {
			liked = true
			if err := tx.Unscoped().Model(&like).Update("deleted_at", nil).Error; err != nil {
				return err
			}
			return createMomentLikeMessage(tx, moment, userID)
		}

		liked = false
		return tx.Delete(&like).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, ErrMomentNotFound
	}
	if err != nil {
		return nil, false, err
	}

	detail, err := r.FindPublicDetail(id, &userID)
	return detail, liked, err
}

func (r *momentRepo) updateTop(id uint, operatorID uint, force bool, top bool) (*model.Moment, error) {
	var moment model.Moment
	err := r.db.Transaction(func(tx *gorm.DB) error {
		repo := &momentRepo{db: tx}
		found, err := repo.findMomentForMutation(id)
		if err != nil {
			return err
		}
		if !force && found.UserID != operatorID {
			return ErrNoPermission
		}
		if top && !found.IsTop {
			count, err := repo.countTopMoments(found.UserID, found.ID)
			if err != nil {
				return err
			}
			if count >= MaxTopMomentsPerUser {
				return ErrTopLimitExceeded
			}
		}
		if err := tx.Model(&model.Moment{}).Where("id = ?", id).Update("is_top", top).Error; err != nil {
			return err
		}
		moment = *found
		moment.IsTop = top
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMomentNotFound
	}
	return &moment, err
}

func (r *momentRepo) countTopMoments(userID uint, exceptID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.Moment{}).
		Where("user_id = ? AND is_top = ? AND id <> ?", userID, true, exceptID).
		Count(&count).Error
	return count, err
}

func (r *momentRepo) findMomentForMutation(id uint) (*model.Moment, error) {
	var moment model.Moment
	err := r.db.First(&moment, id).Error
	if err != nil {
		return nil, err
	}
	return &moment, nil
}

func (r *momentRepo) findAnyDetail(id uint, viewerID *uint) (*MomentAggregate, error) {
	var moment model.Moment
	err := r.db.First(&moment, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMomentNotFound
	}
	if err != nil {
		return nil, err
	}
	aggregates, err := r.attachRelations([]model.Moment{moment}, viewerID)
	if err != nil {
		return nil, err
	}
	if len(aggregates) == 0 {
		return nil, ErrMomentNotFound
	}
	return &aggregates[0], nil
}

func createMomentLikeMessage(tx *gorm.DB, moment model.Moment, fromUserID uint) error {
	if moment.UserID == fromUserID {
		return nil
	}
	title := "碎语点赞"
	content := ""
	message := model.Message{
		Title:      &title,
		Content:    &content,
		Type:       "moment_like",
		TypeID:     moment.ID,
		FromUserID: fromUserID,
	}
	if err := tx.Create(&message).Error; err != nil {
		return err
	}
	return tx.Create(&model.UserMessage{
		UserID:    moment.UserID,
		MessageID: message.ID,
		IsRead:    false,
	}).Error
}

func momentUpdateFields(moment model.Moment) map[string]interface{} {
	return map[string]interface{}{
		"user_id":        moment.UserID,
		"content":        moment.Content,
		"status":         moment.Status,
		"comment_status": moment.CommentStatus,
	}
}
