package comment

import (
	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *commentRepo) Create(target Target, userID uint, content string) (*CommentAggregate, error) {
	ownerUserID, err := r.ensureCommentableTarget(target)
	if err != nil {
		return nil, err
	}
	rootSnapshot, err := r.rootSnapshot(target)
	if err != nil {
		return nil, err
	}

	// 评论落库；通知改由 service 层发布 notification_event，不再在仓储写旧 message。
	comment, err := r.createCommentRecord(target, userID, content)
	if err != nil {
		return nil, err
	}
	userMap, err := r.usersByID([]uint{userID})
	if err != nil {
		return nil, err
	}
	return &CommentAggregate{
		Comment:      *comment,
		User:         userMap[userID],
		ReplyCount:   0,
		LikeCount:    0,
		IsLiked:      false,
		OwnerUserID:  ownerUserID,
		RootSnapshot: rootSnapshot,
	}, nil
}

func (r *commentRepo) DeleteComment(target Target, commentID uint, userID uint, force bool) (*CommentRecord, error) {
	comment, err := r.findCommentByID(target.Type, commentID)
	if err != nil {
		return nil, err
	}
	if !force && comment.UserID != userID {
		return nil, ErrNoDeletePermission
	}

	err = r.db.Transaction(func(tx *gorm.DB) error {
		if err := deleteRepliesByCommentWithTx(tx, target.Type, commentID); err != nil {
			return err
		}
		return deleteCommentWithTx(tx, target.Type, commentID)
	})
	if err != nil {
		return nil, err
	}
	return comment, nil
}

func (r *commentRepo) DeleteReply(target Target, replyID uint, userID uint, force bool) (*ReplyRecord, error) {
	reply, err := r.findReplyByID(target.Type, replyID)
	if err != nil {
		return nil, err
	}
	if !force && reply.FromUserID != userID {
		return nil, ErrNoDeletePermission
	}
	if err := deleteReplyWithTx(r.db, target.Type, replyID); err != nil {
		return nil, err
	}
	return reply, nil
}

func (r *commentRepo) createCommentRecord(target Target, userID uint, content string) (*CommentRecord, error) {
	switch target.Type {
	case TargetArticle:
		comment := model.ArticleComment{ArticleID: target.ID, UserID: userID, Content: content}
		if err := r.db.Create(&comment).Error; err != nil {
			return nil, err
		}
		return articleCommentRecord(comment), nil
	case TargetMoment:
		comment := model.MomentComment{MomentID: target.ID, UserID: userID, Content: content}
		if err := r.db.Create(&comment).Error; err != nil {
			return nil, err
		}
		return momentCommentRecord(comment), nil
	case TargetGuestbook:
		comment := model.Guestbook{OwnerUserID: target.ID, FromUserID: userID, Content: content}
		if err := r.db.Create(&comment).Error; err != nil {
			return nil, err
		}
		return guestbookRecord(comment), nil
	default:
		return nil, ErrTargetNotFound
	}
}

func deleteCommentWithTx(tx *gorm.DB, commentType uint8, commentID uint) error {
	switch commentType {
	case TargetArticle:
		return tx.Delete(&model.ArticleComment{}, commentID).Error
	case TargetMoment:
		return tx.Delete(&model.MomentComment{}, commentID).Error
	case TargetGuestbook:
		return tx.Delete(&model.Guestbook{}, commentID).Error
	default:
		return ErrCommentNotFound
	}
}

func deleteRepliesByCommentWithTx(tx *gorm.DB, commentType uint8, commentID uint) error {
	switch commentType {
	case TargetArticle:
		return tx.Where("comment_id = ?", commentID).Delete(&model.ArticleCommentReply{}).Error
	case TargetMoment:
		return tx.Where("comment_id = ?", commentID).Delete(&model.MomentCommentReply{}).Error
	case TargetGuestbook:
		return tx.Where("comment_id = ?", commentID).Delete(&model.GuestbookReply{}).Error
	default:
		return ErrReplyNotFound
	}
}

func deleteReplyWithTx(tx *gorm.DB, commentType uint8, replyID uint) error {
	switch commentType {
	case TargetArticle:
		return tx.Delete(&model.ArticleCommentReply{}, replyID).Error
	case TargetMoment:
		return tx.Delete(&model.MomentCommentReply{}, replyID).Error
	case TargetGuestbook:
		return tx.Delete(&model.GuestbookReply{}, replyID).Error
	default:
		return ErrReplyNotFound
	}
}
