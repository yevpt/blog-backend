package moment

import (
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *momentRepo) List(filter ListFilter, viewerID *uint) (*PageResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)

	var total int64
	if err := r.publicMomentQuery(filter, viewerID).Count(&total).Error; err != nil {
		return nil, err
	}

	var moments []model.Moment
	query := r.publicMomentQuery(filter, viewerID)
	if filter.Random {
		query = query.Order("RAND()").Limit(pageSize)
	} else {
		offset := (page - 1) * pageSize
		query = query.
			Order("moment.is_top DESC").
			Order("moment.created_at DESC").
			Order("moment.id DESC").
			Limit(pageSize).
			Offset(offset)
	}
	if err := query.Select("moment.*").Find(&moments).Error; err != nil {
		return nil, err
	}

	aggregates, err := r.attachRelations(moments, viewerID)
	if err != nil {
		return nil, err
	}
	return &PageResult{Total: total, Page: page, PageSize: pageSize, Moments: aggregates}, nil
}

func (r *momentRepo) ListAdmin(filter AdminListFilter) (*PageResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)

	var total int64
	if err := r.adminMomentQuery(filter).Count(&total).Error; err != nil {
		return nil, err
	}

	var moments []model.Moment
	offset := (page - 1) * pageSize
	if err := r.adminMomentQuery(filter).
		Order("is_top DESC").
		Order("created_at DESC").
		Order("id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&moments).Error; err != nil {
		return nil, err
	}

	aggregates, err := r.attachRelations(moments, nil)
	if err != nil {
		return nil, err
	}
	return &PageResult{Total: total, Page: page, PageSize: pageSize, Moments: aggregates}, nil
}

func (r *momentRepo) FindPublicDetail(id uint, viewerID *uint) (*MomentAggregate, error) {
	var moment model.Moment
	err := r.publicMomentBase(nil).Select("moment.*").Where("moment.id = ?", id).First(&moment).Error
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

// CountPublicByUser 统计某用户发布的公开碎语总数。
func (r *momentRepo) CountPublicByUser(userID uint) (int64, error) {
	var total int64
	filter := ListFilter{UserID: &userID}
	if err := r.publicMomentQuery(filter, &userID).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *momentRepo) publicMomentQuery(filter ListFilter, viewerID *uint) *gorm.DB {
	query := r.publicMomentBase(authorHiddenUserID(filter.UserID, viewerID))
	if filter.UserID != nil {
		query = query.Where("moment.user_id = ?", *filter.UserID)
	}
	if filter.RoleID != nil {
		query = query.Joins("JOIN user_role ON user_role.user_id = moment.user_id").
			Where("user_role.role_id = ?", *filter.RoleID)
	}
	if len(filter.ExcludeIDs) > 0 {
		query = query.Where("moment.id NOT IN ?", filter.ExcludeIDs)
	}
	return query
}

func (r *momentRepo) publicMomentBase(authorHiddenUserID *uint) *gorm.DB {
	query := r.db.Model(&model.Moment{})
	if !r.moderationEnabled {
		return query.Where("moment.status = ?", uint8(1))
	}
	if authorHiddenUserID != nil {
		return query.
			Joins("LEFT JOIN moderation_item AS public_moderation ON public_moderation.content_type = ? AND public_moderation.content_id = moment.id", model.ModerationContentMoment).
			Where(`(
			(public_moderation.id IS NULL AND moment.status = ?)
			OR (
				public_moderation.lifecycle_state = ?
				AND (
					(public_moderation.public_state = ? AND moment.status = ?)
					OR public_moderation.public_state = ?
					OR (public_moderation.public_state = ? AND moment.user_id = ?)
				)
			)
		)`, uint8(1), model.ModerationLifecycleActive, model.ModerationPublicVisible, uint8(1), model.ModerationPublicPlaceholder, model.ModerationPublicHidden, *authorHiddenUserID)
	}
	return query.
		Joins("LEFT JOIN moderation_item AS public_moderation ON public_moderation.content_type = ? AND public_moderation.content_id = moment.id", model.ModerationContentMoment).
		Where(`(
			(public_moderation.id IS NULL AND moment.status = ?)
			OR (
				public_moderation.lifecycle_state = ?
				AND (
					(public_moderation.public_state = ? AND moment.status = ?)
					OR public_moderation.public_state = ?
				)
			)
		)`, uint8(1), model.ModerationLifecycleActive, model.ModerationPublicVisible, uint8(1), model.ModerationPublicPlaceholder)
}

func authorHiddenUserID(filterUserID *uint, viewerID *uint) *uint {
	if filterUserID == nil || viewerID == nil || *filterUserID != *viewerID {
		return nil
	}
	return filterUserID
}

func (r *momentRepo) adminMomentQuery(filter AdminListFilter) *gorm.DB {
	query := r.db.Model(&model.Moment{})
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if keyword := strings.TrimSpace(filter.Search); keyword != "" {
		query = query.Where("content LIKE ?", "%"+keyword+"%")
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
