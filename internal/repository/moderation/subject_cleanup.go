package moderation

import (
	"context"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

const (
	articleCommentLikeType      uint8 = 2
	articleCommentReplyLikeType uint8 = 3
	momentLikeType              uint8 = 4
	guestbookLikeType           uint8 = 5
	momentCommentLikeType       uint8 = 6
	momentCommentReplyLikeType  uint8 = 7
	guestbookReplyLikeType      uint8 = 8
)

// deleteSubjectLikes 清理被删除对象的点赞行；类型值与 user_like 的既有业务协议保持一致。
func deleteSubjectLikes(ctx context.Context, tx *gorm.DB, ref SubjectRef) error {
	likeType, ok := subjectLikeType(ref.Type)
	if !ok {
		return nil
	}
	return tx.WithContext(ctx).Unscoped().
		Where("target_id = ? AND type = ?", ref.ID, likeType).
		Delete(&model.UserLike{}).Error
}

func subjectLikeType(subjectType SubjectType) (uint8, bool) {
	switch subjectType {
	case SubjectArticleComment:
		return articleCommentLikeType, true
	case SubjectArticleCommentReply:
		return articleCommentReplyLikeType, true
	case SubjectMoment:
		return momentLikeType, true
	case SubjectGuestbook:
		return guestbookLikeType, true
	case SubjectMomentComment:
		return momentCommentLikeType, true
	case SubjectMomentCommentReply:
		return momentCommentReplyLikeType, true
	case SubjectGuestbookReply:
		return guestbookReplyLikeType, true
	default:
		return 0, false
	}
}
