package moment

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *momentRepo) List(filter ListFilter, viewerID *uint) (*PageResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)

	var total int64
	if err := r.publicMomentQuery(filter).Count(&total).Error; err != nil {
		return nil, err
	}

	var moments []model.Moment
	offset := (page - 1) * pageSize
	if err := r.publicMomentQuery(filter).
		Order("is_top DESC").
		Order("created_at DESC").
		Order("id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&moments).Error; err != nil {
		return nil, err
	}

	aggregates, err := r.attachRelations(moments, viewerID)
	if err != nil {
		return nil, err
	}
	return &PageResult{Total: total, Page: page, PageSize: pageSize, Moments: aggregates}, nil
}

func (r *momentRepo) FindPublicDetail(id uint, viewerID *uint) (*MomentAggregate, error) {
	var moment model.Moment
	err := r.db.Where("id = ? AND status = ?", id, uint8(1)).First(&moment).Error
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

func (r *momentRepo) publicMomentQuery(filter ListFilter) *gorm.DB {
	query := r.db.Model(&model.Moment{}).Where("status = ?", uint8(1))
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if filter.RoleID != nil {
		query = query.Joins("JOIN user_role ON user_role.user_id = moment.user_id").
			Where("user_role.role_id = ?", *filter.RoleID)
	}
	return query
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

func (r *momentRepo) attachRelations(moments []model.Moment, viewerID *uint) ([]MomentAggregate, error) {
	ids := momentIDs(moments)
	aggregates := make([]MomentAggregate, 0, len(moments))
	if len(ids) == 0 {
		return aggregates, nil
	}

	users, err := r.usersByID(momentUserIDs(moments))
	if err != nil {
		return nil, err
	}
	images, err := r.imagesByMomentID(ids)
	if err != nil {
		return nil, err
	}
	likeCounts, err := r.likeCounts(ids)
	if err != nil {
		return nil, err
	}
	commentCounts, err := r.commentCounts(ids)
	if err != nil {
		return nil, err
	}
	likedIDs, err := r.likedIDs(ids, viewerID)
	if err != nil {
		return nil, err
	}

	for _, moment := range moments {
		aggregates = append(aggregates, MomentAggregate{
			Moment:       moment,
			User:         users[moment.UserID],
			Images:       images[moment.ID],
			LikeCount:    likeCounts[moment.ID],
			CommentCount: commentCounts[moment.ID],
			IsLiked:      likedIDs[moment.ID],
		})
	}
	return aggregates, nil
}

func (r *momentRepo) likeCounts(ids []uint) (map[uint]int64, error) {
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
		Where("type = ? AND target_id IN ?", MomentLikeType, ids).
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

func (r *momentRepo) commentCounts(ids []uint) (map[uint]int64, error) {
	counts := make(map[uint]int64, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}

	var rows []struct {
		MomentID uint
		Count    int64
	}
	err := r.db.Model(&model.MomentComment{}).
		Select("moment_id, count(*) as count").
		Where("moment_id IN ?", ids).
		Group("moment_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.MomentID] = row.Count
	}
	return counts, nil
}

func (r *momentRepo) likedIDs(ids []uint, viewerID *uint) (map[uint]bool, error) {
	liked := make(map[uint]bool, len(ids))
	if viewerID == nil || len(ids) == 0 {
		return liked, nil
	}

	var rows []uint
	err := r.db.Model(&model.UserLike{}).
		Select("target_id").
		Where("type = ? AND user_id = ? AND target_id IN ?", MomentLikeType, *viewerID, ids).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, id := range rows {
		liked[id] = true
	}
	return liked, nil
}

func (r *momentRepo) countLikes(id uint) (int64, error) {
	var total int64
	err := r.db.Model(&model.UserLike{}).
		Where("target_id = ? AND type = ?", id, MomentLikeType).
		Count(&total).Error
	return total, err
}
