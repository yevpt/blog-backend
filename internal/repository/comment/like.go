package comment

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *commentRepo) ToggleLike(target Target, commentID uint, userID uint) (*LikeResult, error) {
	comment, err := r.findCommentByID(target.Type, commentID)
	if err != nil {
		return nil, err
	}
	result, err := r.toggleLike(commentLikeType(target.Type), commentID, userID)
	if err != nil {
		return nil, err
	}
	// 填充评论作者、根对象 ID 与评论正文，供通知层发布 comment_liked 事件使用。
	result.RootID = comment.TargetID
	result.TargetUserID = comment.UserID
	result.TargetContent = comment.Content
	if result.IsLiked {
		rootSnapshot, err := r.rootSnapshot(Target{Type: target.Type, ID: comment.TargetID})
		if err != nil {
			return nil, err
		}
		result.RootSnapshot = rootSnapshot
	}
	return result, nil
}

func (r *commentRepo) ToggleReplyLike(target Target, replyID uint, userID uint) (*LikeResult, error) {
	reply, err := r.findReplyByID(target.Type, replyID)
	if err != nil {
		return nil, err
	}
	comment, err := r.findCommentByID(target.Type, reply.CommentID)
	if err != nil {
		return nil, err
	}
	result, err := r.toggleLike(replyLikeType(target.Type), replyID, userID)
	if err != nil {
		return nil, err
	}
	rootID, rootSnapshot, err := r.replyRootSnapshot(target.Type, comment)
	if err != nil {
		return nil, err
	}
	result.RootID = rootID
	result.CommentID = reply.CommentID
	result.TargetUserID = reply.FromUserID
	result.TargetContent = reply.Content
	result.RootSnapshot = rootSnapshot
	return result, nil
}

func (r *commentRepo) toggleLike(likeType uint8, targetID uint, userID uint) (*LikeResult, error) {
	liked := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var like model.UserLike
		err := tx.Unscoped().
			Where("target_id = ? AND user_id = ? AND type = ?", targetID, userID, likeType).
			First(&like).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			liked = true
			return tx.Create(&model.UserLike{UserID: userID, TargetID: targetID, Type: likeType}).Error
		}
		if err != nil {
			return err
		}
		liked = false
		return tx.Unscoped().Delete(&like).Error
	})
	if err != nil {
		return nil, err
	}

	count, err := r.countLikesByType(targetID, likeType)
	if err != nil {
		return nil, err
	}
	return &LikeResult{IsLiked: liked, LikeCount: count}, nil
}

func (r *commentRepo) commentLikeCounts(commentType uint8, ids []uint) (map[uint]int64, error) {
	return r.likeCountsByType(ids, commentLikeType(commentType))
}

func (r *commentRepo) replyLikeCounts(commentType uint8, ids []uint) (map[uint]int64, error) {
	return r.likeCountsByType(ids, replyLikeType(commentType))
}

func (r *commentRepo) commentLikedIDs(commentType uint8, ids []uint, viewerID *uint) (map[uint]bool, error) {
	return r.likedIDsByType(ids, viewerID, commentLikeType(commentType))
}

func (r *commentRepo) replyLikedIDs(commentType uint8, ids []uint, viewerID *uint) (map[uint]bool, error) {
	return r.likedIDsByType(ids, viewerID, replyLikeType(commentType))
}

func (r *commentRepo) likeCountsByType(ids []uint, likeType uint8) (map[uint]int64, error) {
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

func (r *commentRepo) likedIDsByType(ids []uint, viewerID *uint, likeType uint8) (map[uint]bool, error) {
	liked := make(map[uint]bool, len(ids))
	if viewerID == nil || len(ids) == 0 {
		return liked, nil
	}

	var rows []uint
	err := r.db.Model(&model.UserLike{}).
		Select("target_id").
		Where("type = ? AND user_id = ? AND target_id IN ?", likeType, *viewerID, ids).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, id := range rows {
		liked[id] = true
	}
	return liked, nil
}

func (r *commentRepo) countLikesByType(targetID uint, likeType uint8) (int64, error) {
	var total int64
	err := r.db.Model(&model.UserLike{}).
		Where("target_id = ? AND type = ?", targetID, likeType).
		Count(&total).Error
	return total, err
}

func commentLikeType(commentType uint8) uint8 {
	switch commentType {
	case TargetArticle:
		return ArticleCommentLikeType
	case TargetMoment:
		return MomentCommentLikeType
	case TargetGuestbook:
		return GuestbookLikeType
	default:
		return 0
	}
}

func replyLikeType(commentType uint8) uint8 {
	switch commentType {
	case TargetArticle:
		return ArticleCommentReplyLikeType
	case TargetMoment:
		return MomentCommentReplyLikeType
	case TargetGuestbook:
		return GuestbookReplyLikeType
	default:
		return 0
	}
}
