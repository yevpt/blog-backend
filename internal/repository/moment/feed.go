package moment

import (
	"fmt"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *momentRepo) ListFeed(filter FeedFilter, viewerID *uint) (*PageResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)

	baseQuery := r.feedMomentQuery(filter, viewerID)

	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, err
	}

	var moments []model.Moment
	offset := (page - 1) * pageSize
	if err := r.applyFeedOrder(r.feedMomentQuery(filter, viewerID), filter.Sort).
		Select("moment.*").
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

func (r *momentRepo) feedMomentQuery(filter FeedFilter, viewerID *uint) *gorm.DB {
	hiddenOwnerID := (*uint)(nil)
	if filter.Scope == FeedScopeOwner {
		hiddenOwnerID = authorHiddenUserID(&filter.OwnerUserID, viewerID)
	}
	query := r.publicMomentBase(hiddenOwnerID)
	switch filter.Scope {
	case FeedScopeOwner:
		query = query.Where("moment.user_id = ?", filter.OwnerUserID)
	case FeedScopeFriends:
		query = query.Where("moment.user_id <> ?", filter.OwnerUserID)
	}
	return query
}

func (r *momentRepo) applyFeedOrder(query *gorm.DB, sort FeedSort) *gorm.DB {
	switch sort {
	case FeedSortHot:
		commentSub := r.db.Model(&model.MomentComment{}).
			Select("moment_id, COUNT(*) AS cnt").
			Group("moment_id")
		likeSub := r.db.Model(&model.UserLike{}).
			Select("target_id, COUNT(*) AS cnt").
			Where("type = ?", MomentLikeType).
			Group("target_id")
		scoreExpr := fmt.Sprintf(
			"(COALESCE(feed_comment_stats.cnt, 0) * %d + COALESCE(feed_like_stats.cnt, 0) * %d + moment.read_count) DESC",
			feedHotCommentWeight,
			feedHotLikeWeight,
		)
		return query.
			Select("moment.*").
			Joins("LEFT JOIN (?) AS feed_comment_stats ON feed_comment_stats.moment_id = moment.id", commentSub).
			Joins("LEFT JOIN (?) AS feed_like_stats ON feed_like_stats.target_id = moment.id", likeSub).
			Order(scoreExpr).
			Order("moment.is_top DESC").
			Order("moment.created_at DESC").
			Order("moment.id DESC")
	default:
		return query.
			Order("moment.is_top DESC").
			Order("moment.created_at DESC").
			Order("moment.id DESC")
	}
}
