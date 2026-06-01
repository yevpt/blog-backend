package guestbook

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *guestbookRepo) Create(ownerUserID uint, fromUserID uint, content string) (*GuestbookAggregate, error) {
	if err := r.ensureOwnerExists(ownerUserID); err != nil {
		return nil, err
	}

	message := model.Guestbook{OwnerUserID: ownerUserID, FromUserID: fromUserID, Content: content}
	if err := r.db.Create(&message).Error; err != nil {
		return nil, err
	}

	userMap, err := r.usersByID([]uint{fromUserID})
	if err != nil {
		return nil, err
	}
	return &GuestbookAggregate{
		Message:   message,
		User:      userMap[fromUserID],
		LikeCount: 0,
		IsLiked:   false,
	}, nil
}

func (r *guestbookRepo) ToggleLike(id uint, userID uint) (*LikeResult, error) {
	liked := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var message model.Guestbook
		if err := tx.First(&message, id).Error; err != nil {
			return err
		}

		var like model.UserLike
		err := tx.Unscoped().
			Where("target_id = ? AND user_id = ? AND type = ?", id, userID, LikeType).
			First(&like).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			liked = true
			return tx.Create(&model.UserLike{UserID: userID, TargetID: id, Type: LikeType}).Error
		}
		if err != nil {
			return err
		}
		if like.DeletedAt.Valid {
			liked = true
			return tx.Unscoped().Model(&like).Update("deleted_at", nil).Error
		}

		liked = false
		return tx.Delete(&like).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGuestbookNotFound
	}
	if err != nil {
		return nil, err
	}

	count, err := r.countLikes(id)
	if err != nil {
		return nil, err
	}
	return &LikeResult{ID: id, IsLiked: liked, LikeCount: count}, nil
}

func (r *guestbookRepo) Delete(id uint, userID uint, force bool) (*model.Guestbook, error) {
	var message model.Guestbook
	err := r.db.First(&message, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGuestbookNotFound
	}
	if err != nil {
		return nil, err
	}
	if !force && message.FromUserID != userID && message.OwnerUserID != userID {
		return nil, ErrNoDeletePermission
	}
	if err := r.db.Delete(&message).Error; err != nil {
		return nil, err
	}
	return &message, nil
}
