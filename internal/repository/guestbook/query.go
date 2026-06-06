package guestbook

import (
	"github.com/vpt/blog-backend/internal/model"
)

func (r *guestbookRepo) List(ownerUserID uint, viewerID *uint, page int, pageSize int) (*PageResult, error) {
	if err := r.ensureOwnerExists(ownerUserID); err != nil {
		return nil, err
	}

	page, pageSize = normalizePage(page, pageSize)
	total, err := r.countMessages(ownerUserID)
	if err != nil {
		return nil, err
	}
	messages, err := r.listMessages(ownerUserID, page, pageSize)
	if err != nil {
		return nil, err
	}
	aggregates, err := r.attachRelations(messages, viewerID)
	if err != nil {
		return nil, err
	}

	return &PageResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Messages: aggregates,
	}, nil
}

func (r *guestbookRepo) ensureOwnerExists(ownerUserID uint) error {
	var count int64
	err := r.db.Model(&model.User{}).
		Where("id = ? AND status = ?", ownerUserID, uint8(1)).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrOwnerNotFound
	}
	return nil
}

func normalizePage(page int, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}
	return page, pageSize
}

func (r *guestbookRepo) countMessages(ownerUserID uint) (int64, error) {
	var total int64
	err := r.db.Model(&model.Guestbook{}).Where("owner_user_id = ?", ownerUserID).Count(&total).Error
	return total, err
}

func (r *guestbookRepo) listMessages(ownerUserID uint, page int, pageSize int) ([]model.Guestbook, error) {
	var messages []model.Guestbook
	offset := (page - 1) * pageSize
	err := r.db.
		Where("owner_user_id = ?", ownerUserID).
		Order("created_at DESC").
		Order("id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&messages).Error
	return messages, err
}

func (r *guestbookRepo) attachRelations(messages []model.Guestbook, viewerID *uint) ([]GuestbookAggregate, error) {
	userMap, err := r.usersByID(messageUserIDs(messages))
	if err != nil {
		return nil, err
	}
	ids := messageIDs(messages)
	replyCounts, err := r.replyCounts(ids)
	if err != nil {
		return nil, err
	}
	likeCounts, err := r.likeCounts(ids)
	if err != nil {
		return nil, err
	}
	likedIDs, err := r.likedIDs(ids, viewerID)
	if err != nil {
		return nil, err
	}

	aggregates := make([]GuestbookAggregate, 0, len(messages))
	for _, message := range messages {
		aggregates = append(aggregates, GuestbookAggregate{
			Message:    message,
			User:       userMap[message.FromUserID],
			ReplyCount: replyCounts[message.ID],
			LikeCount:  likeCounts[message.ID],
			IsLiked:    likedIDs[message.ID],
		})
	}
	return aggregates, nil
}

func (r *guestbookRepo) replyCounts(ids []uint) (map[uint]int64, error) {
	counts := make(map[uint]int64, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}

	var rows []struct {
		CommentID uint
		Count     int64
	}
	err := r.db.Model(&model.GuestbookReply{}).
		Select("comment_id, count(*) as count").
		Where("comment_id IN ?", ids).
		Group("comment_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.CommentID] = row.Count
	}
	return counts, nil
}

func (r *guestbookRepo) likeCounts(ids []uint) (map[uint]int64, error) {
	counts := make(map[uint]int64, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}

	var rows []struct {
		TargetID uint
		Count    int64
	}
	err := r.db.Model(&model.UserLike{}).
		Select("target_id, count(*) as count").
		Where("type = ? AND target_id IN ?", LikeType, ids).
		Group("target_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.TargetID] = row.Count
	}
	return counts, nil
}

func (r *guestbookRepo) likedIDs(ids []uint, viewerID *uint) (map[uint]bool, error) {
	liked := make(map[uint]bool, len(ids))
	if viewerID == nil || len(ids) == 0 {
		return liked, nil
	}

	var rows []uint
	err := r.db.Model(&model.UserLike{}).
		Select("target_id").
		Where("type = ? AND user_id = ? AND target_id IN ?", LikeType, *viewerID, ids).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, id := range rows {
		liked[id] = true
	}
	return liked, nil
}

func (r *guestbookRepo) countLikes(id uint) (int64, error) {
	var total int64
	err := r.db.Model(&model.UserLike{}).
		Where("target_id = ? AND type = ?", id, LikeType).
		Count(&total).Error
	return total, err
}
