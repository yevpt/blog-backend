package comment

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *commentRepo) findCommentByID(commentType uint8, commentID uint) (*CommentRecord, error) {
	switch commentType {
	case TargetArticle:
		var comment model.ArticleComment
		err := r.db.Where("id = ?", commentID).First(&comment).Error
		if err != nil {
			return nil, mapCommentFindError(err)
		}
		return articleCommentRecord(comment), nil
	case TargetMoment:
		var comment model.MomentComment
		err := r.db.Where("id = ?", commentID).First(&comment).Error
		if err != nil {
			return nil, mapCommentFindError(err)
		}
		return momentCommentRecord(comment), nil
	case TargetGuestbook:
		var comment model.Guestbook
		err := r.db.Where("id = ?", commentID).First(&comment).Error
		if err != nil {
			return nil, mapCommentFindError(err)
		}
		return guestbookRecord(comment), nil
	default:
		return nil, ErrCommentNotFound
	}
}

func mapCommentFindError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrCommentNotFound
	}
	if err != nil {
		return err
	}
	return nil
}

func articleCommentRecord(comment model.ArticleComment) *CommentRecord {
	return &CommentRecord{
		ID:        comment.ID,
		TargetID:  comment.ArticleID,
		UserID:    comment.UserID,
		Content:   comment.Content,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
	}
}

func momentCommentRecord(comment model.MomentComment) *CommentRecord {
	return &CommentRecord{
		ID:        comment.ID,
		TargetID:  comment.MomentID,
		UserID:    comment.UserID,
		Content:   comment.Content,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
	}
}

func guestbookRecord(comment model.Guestbook) *CommentRecord {
	return &CommentRecord{
		ID:        comment.ID,
		TargetID:  comment.OwnerUserID,
		UserID:    comment.FromUserID,
		Content:   comment.Content,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
	}
}

func commentIDs(comments []CommentRecord) []uint {
	ids := make([]uint, 0, len(comments))
	for _, comment := range comments {
		ids = append(ids, comment.ID)
	}
	return ids
}

func commentUserIDs(comments []CommentRecord) []uint {
	ids := make([]uint, 0, len(comments))
	for _, comment := range comments {
		ids = append(ids, comment.UserID)
	}
	return ids
}
