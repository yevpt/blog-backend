package notification

import (
	"context"
	"fmt"

	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"github.com/vpt/blog-backend/internal/model"
)

// SourceEngagement 是通知来源对象在读取时的互动快照。
type SourceEngagement struct {
	LikeCount  int64
	IsLiked    bool
	ReplyCount int64
}

// SourceEngagementRef 标识一条通知的来源对象，用于批量解析互动数据。
type SourceEngagementRef struct {
	SourceType string
	SourceID   uint
	RootType   string
}

// SourceEngagementKey 标识通知来源对象。
type SourceEngagementKey struct {
	SourceType string
	SourceID   uint
}

type engagementPlan struct {
	key         SourceEngagementKey
	likeType    uint8
	commentType uint8
	countMode   engagementCountMode
}

type engagementCountMode uint8

const (
	engagementCountNone engagementCountMode = iota
	engagementCountByComment
	engagementCountByParentReply
)

// BatchSourceEngagement 批量解析评论/回复/留言来源的点赞与回复数。
func (d *Directory) BatchSourceEngagement(ctx context.Context, viewerID uint, refs []SourceEngagementRef) (map[SourceEngagementKey]SourceEngagement, error) {
	plans := make([]engagementPlan, 0, len(refs))
	seen := make(map[SourceEngagementKey]struct{}, len(refs))
	for _, ref := range refs {
		plan, ok := engagementPlanFor(ref)
		if !ok {
			continue
		}
		if _, exists := seen[plan.key]; exists {
			continue
		}
		seen[plan.key] = struct{}{}
		plans = append(plans, plan)
	}
	if len(plans) == 0 {
		return map[SourceEngagementKey]SourceEngagement{}, nil
	}

	out := make(map[SourceEngagementKey]SourceEngagement, len(plans))
	likeGroups := make(map[uint8][]uint)
	for _, plan := range plans {
		likeGroups[plan.likeType] = append(likeGroups[plan.likeType], plan.key.SourceID)
		out[plan.key] = SourceEngagement{}
	}

	likeCounts, err := d.batchLikeCounts(ctx, likeGroups)
	if err != nil {
		return nil, err
	}
	likedIDs, err := d.batchLikedIDs(ctx, viewerID, likeGroups)
	if err != nil {
		return nil, err
	}

	commentReplyGroups := make(map[uint8][]uint)
	parentReplyGroups := make(map[uint8][]uint)
	for _, plan := range plans {
		switch plan.countMode {
		case engagementCountByComment:
			commentReplyGroups[plan.commentType] = append(commentReplyGroups[plan.commentType], plan.key.SourceID)
		case engagementCountByParentReply:
			parentReplyGroups[plan.commentType] = append(parentReplyGroups[plan.commentType], plan.key.SourceID)
		}
	}

	commentReplyCounts, err := d.batchReplyCountsByComment(ctx, commentReplyGroups)
	if err != nil {
		return nil, err
	}
	parentReplyCounts, err := d.batchReplyCountsByParentReply(ctx, parentReplyGroups)
	if err != nil {
		return nil, err
	}

	for _, plan := range plans {
		eng := out[plan.key]
		eng.LikeCount = likeCounts[plan.likeType][plan.key.SourceID]
		eng.IsLiked = likedIDs[plan.likeType][plan.key.SourceID]
		switch plan.countMode {
		case engagementCountByComment:
			eng.ReplyCount = commentReplyCounts[plan.commentType][plan.key.SourceID]
		case engagementCountByParentReply:
			eng.ReplyCount = parentReplyCounts[plan.commentType][plan.key.SourceID]
		}
		out[plan.key] = eng
	}
	return out, nil
}

// BatchReplyCommentIDs 批量查询回复 ID 对应的父评论 ID。
func (d *Directory) BatchReplyCommentIDs(ctx context.Context, refs []SourceEngagementRef) (map[uint]uint, error) {
	groups := make(map[uint8][]uint)
	for _, ref := range refs {
		if ref.SourceType != "reply" || ref.SourceID == 0 {
			continue
		}
		commentType, ok := commentTypeForRoot(ref.RootType)
		if !ok {
			continue
		}
		groups[commentType] = append(groups[commentType], ref.SourceID)
	}
	if len(groups) == 0 {
		return map[uint]uint{}, nil
	}

	out := make(map[uint]uint)
	for commentType, ids := range groups {
		rows, err := d.queryReplyCommentIDs(ctx, commentType, ids)
		if err != nil {
			return nil, err
		}
		for replyID, commentID := range rows {
			out[replyID] = commentID
		}
	}
	return out, nil
}

func engagementPlanFor(ref SourceEngagementRef) (engagementPlan, bool) {
	if ref.SourceID == 0 {
		return engagementPlan{}, false
	}
	commentType, ok := commentTypeForRoot(ref.RootType)
	if !ok {
		return engagementPlan{}, false
	}

	key := SourceEngagementKey{SourceType: ref.SourceType, SourceID: ref.SourceID}
	switch ref.SourceType {
	case "comment":
		likeType, ok := commentLikeType(commentType)
		if !ok {
			return engagementPlan{}, false
		}
		return engagementPlan{
			key:         key,
			likeType:    likeType,
			commentType: commentType,
			countMode:   engagementCountByComment,
		}, true
	case "guestbook":
		return engagementPlan{
			key:         key,
			likeType:    commentrepo.GuestbookLikeType,
			commentType: commentrepo.TargetGuestbook,
			countMode:   engagementCountByComment,
		}, true
	case "reply":
		likeType, ok := replyLikeType(commentType)
		if !ok {
			return engagementPlan{}, false
		}
		return engagementPlan{
			key:         key,
			likeType:    likeType,
			commentType: commentType,
			countMode:   engagementCountByParentReply,
		}, true
	default:
		return engagementPlan{}, false
	}
}

func commentTypeForRoot(rootType string) (uint8, bool) {
	switch rootType {
	case "article":
		return commentrepo.TargetArticle, true
	case "moment":
		return commentrepo.TargetMoment, true
	case "guestbook":
		return commentrepo.TargetGuestbook, true
	default:
		return 0, false
	}
}

func commentLikeType(commentType uint8) (uint8, bool) {
	switch commentType {
	case commentrepo.TargetArticle:
		return commentrepo.ArticleCommentLikeType, true
	case commentrepo.TargetMoment:
		return commentrepo.MomentCommentLikeType, true
	case commentrepo.TargetGuestbook:
		return commentrepo.GuestbookLikeType, true
	default:
		return 0, false
	}
}

func replyLikeType(commentType uint8) (uint8, bool) {
	switch commentType {
	case commentrepo.TargetArticle:
		return commentrepo.ArticleCommentReplyLikeType, true
	case commentrepo.TargetMoment:
		return commentrepo.MomentCommentReplyLikeType, true
	case commentrepo.TargetGuestbook:
		return commentrepo.GuestbookReplyLikeType, true
	default:
		return 0, false
	}
}

func (d *Directory) batchLikeCounts(ctx context.Context, groups map[uint8][]uint) (map[uint8]map[uint]int64, error) {
	out := make(map[uint8]map[uint]int64, len(groups))
	for likeType, ids := range groups {
		counts, err := d.countLikesByType(ctx, likeType, ids)
		if err != nil {
			return nil, err
		}
		out[likeType] = counts
	}
	return out, nil
}

func (d *Directory) batchLikedIDs(ctx context.Context, viewerID uint, groups map[uint8][]uint) (map[uint8]map[uint]bool, error) {
	out := make(map[uint8]map[uint]bool, len(groups))
	if viewerID == 0 {
		return out, nil
	}
	for likeType, ids := range groups {
		liked, err := d.likedIDsByType(ctx, viewerID, likeType, ids)
		if err != nil {
			return nil, err
		}
		out[likeType] = liked
	}
	return out, nil
}

func (d *Directory) countLikesByType(ctx context.Context, likeType uint8, ids []uint) (map[uint]int64, error) {
	counts := make(map[uint]int64, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}
	var rows []struct {
		TargetID uint
		Count    int64
	}
	err := d.db.WithContext(ctx).Model(&model.UserLike{}).
		Select("target_id, count(*) as count").
		Where("type = ? AND target_id IN ?", likeType, ids).
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

func (d *Directory) likedIDsByType(ctx context.Context, viewerID uint, likeType uint8, ids []uint) (map[uint]bool, error) {
	liked := make(map[uint]bool, len(ids))
	if len(ids) == 0 {
		return liked, nil
	}
	var rows []uint
	err := d.db.WithContext(ctx).Model(&model.UserLike{}).
		Select("target_id").
		Where("type = ? AND user_id = ? AND target_id IN ?", likeType, viewerID, ids).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, id := range rows {
		liked[id] = true
	}
	return liked, nil
}

func (d *Directory) batchReplyCountsByComment(ctx context.Context, groups map[uint8][]uint) (map[uint8]map[uint]int64, error) {
	out := make(map[uint8]map[uint]int64, len(groups))
	for commentType, ids := range groups {
		counts, err := d.countRepliesByComment(ctx, commentType, ids)
		if err != nil {
			return nil, err
		}
		out[commentType] = counts
	}
	return out, nil
}

func (d *Directory) batchReplyCountsByParentReply(ctx context.Context, groups map[uint8][]uint) (map[uint8]map[uint]int64, error) {
	out := make(map[uint8]map[uint]int64, len(groups))
	for commentType, ids := range groups {
		counts, err := d.countRepliesByParentReply(ctx, commentType, ids)
		if err != nil {
			return nil, err
		}
		out[commentType] = counts
	}
	return out, nil
}

func (d *Directory) countRepliesByComment(ctx context.Context, commentType uint8, commentIDs []uint) (map[uint]int64, error) {
	counts := make(map[uint]int64, len(commentIDs))
	if len(commentIDs) == 0 {
		return counts, nil
	}
	table, err := replyTableName(commentType)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		CommentID uint
		Count     int64
	}
	err = d.db.WithContext(ctx).Table(table).
		Select("comment_id, count(*) as count").
		Where("comment_id IN ?", commentIDs).
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

func (d *Directory) countRepliesByParentReply(ctx context.Context, commentType uint8, parentReplyIDs []uint) (map[uint]int64, error) {
	counts := make(map[uint]int64, len(parentReplyIDs))
	if len(parentReplyIDs) == 0 {
		return counts, nil
	}
	table, err := replyTableName(commentType)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ParentReplyID uint
		Count         int64
	}
	err = d.db.WithContext(ctx).Table(table).
		Select("parent_reply_id, count(*) as count").
		Where("parent_reply_id IN ?", parentReplyIDs).
		Group("parent_reply_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ParentReplyID] = row.Count
	}
	return counts, nil
}

func (d *Directory) queryReplyCommentIDs(ctx context.Context, commentType uint8, replyIDs []uint) (map[uint]uint, error) {
	out := make(map[uint]uint, len(replyIDs))
	if len(replyIDs) == 0 {
		return out, nil
	}
	table, err := replyTableName(commentType)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID        uint
		CommentID uint
	}
	err = d.db.WithContext(ctx).Table(table).
		Select("id, comment_id").
		Where("id IN ?", replyIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.CommentID
	}
	return out, nil
}

func replyTableName(commentType uint8) (string, error) {
	switch commentType {
	case commentrepo.TargetArticle:
		return "article_comment_reply", nil
	case commentrepo.TargetMoment:
		return "moment_comment_reply", nil
	case commentrepo.TargetGuestbook:
		return "guestbook_reply", nil
	default:
		return "", fmt.Errorf("不支持的回复类型 %d", commentType)
	}
}
