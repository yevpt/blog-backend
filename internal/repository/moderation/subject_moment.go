package moderation

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type momentAdapter struct{}

// momentInsert 不带 default 标签，确保首次先审后发显式写入 status=0。
type momentInsert struct {
	ID            uint `gorm:"primaryKey"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt
	UserID        uint
	Content       string
	Status        uint8
	CommentStatus uint8
	ReadCount     uint
	IsTop         bool
}

func (a *momentAdapter) Lock(ctx context.Context, tx *gorm.DB, ref SubjectRef, authorID uint64) (SubjectSnapshot, error) {
	if ref.Type != SubjectMoment {
		return SubjectSnapshot{}, ErrInvalidCommand
	}
	var row model.Moment
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", ref.ID, authorID).Take(&row).Error
	if err != nil {
		return SubjectSnapshot{}, subjectError(err)
	}
	return SubjectSnapshot{Ref: SubjectRef{Type: SubjectMoment, ID: uint64(row.ID)}, AuthorID: uint64(row.UserID), Content: row.Content}, nil
}

func (momentInsert) TableName() string { return "moment" }

func (a *momentAdapter) Load(ctx context.Context, db *gorm.DB, ref SubjectRef) (SubjectSnapshot, error) {
	if ref.Type != SubjectMoment {
		return SubjectSnapshot{}, ErrInvalidCommand
	}
	var row model.Moment
	if err := db.WithContext(ctx).First(&row, ref.ID).Error; err != nil {
		return SubjectSnapshot{}, subjectError(err)
	}
	return SubjectSnapshot{Ref: SubjectRef{Type: SubjectMoment, ID: uint64(row.ID)}, AuthorID: uint64(row.UserID), Content: row.Content}, nil
}

func (a *momentAdapter) Materialize(ctx context.Context, tx *gorm.DB, cmd MaterializeCommand) error {
	if cmd.Ref.Type != SubjectMoment || cmd.AuthorID == 0 {
		return ErrInvalidCommand
	}
	if cmd.Create {
		if cmd.AssignedID == nil || cmd.Ref.ID != 0 {
			return ErrInvalidCommand
		}
		if err := ensureActiveUser(ctx, tx, cmd.AuthorID); err != nil {
			return err
		}
		status := uint8(0)
		if cmd.Visible {
			status = 1
		}
		commentStatus := uint8(1)
		if cmd.MomentOptions != nil {
			if cmd.MomentOptions.Status == 0 {
				status = 0
			}
			commentStatus = cmd.MomentOptions.CommentStatus
		}
		row := momentInsert{UserID: uint(cmd.AuthorID), Content: cmd.Content, Status: status, CommentStatus: commentStatus}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		*cmd.AssignedID = uint64(row.ID)
		return nil
	}
	if cmd.Ref.ID == 0 {
		return ErrInvalidCommand
	}
	updates := map[string]any{"content": cmd.Content, "status": uint8(1)}
	if cmd.MomentOptions != nil {
		updates["status"] = cmd.MomentOptions.Status
		updates["comment_status"] = cmd.MomentOptions.CommentStatus
	}
	result := tx.WithContext(ctx).Model(&model.Moment{}).
		Where("id = ? AND user_id = ?", cmd.Ref.ID, cmd.AuthorID).
		Updates(updates)
	return subjectUpdateError(result)
}

func (a *momentAdapter) Delete(ctx context.Context, tx *gorm.DB, ref SubjectRef) error {
	if ref.Type != SubjectMoment {
		return ErrInvalidCommand
	}
	if err := deleteSubjectLikes(ctx, tx, ref); err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Unscoped().Where("moment_id = ?", ref.ID).Delete(&model.Media{}).Error; err != nil {
		return err
	}
	return requireSubjectRow(tx.WithContext(ctx).Delete(&model.Moment{}, ref.ID))
}

func (a *momentAdapter) Descendants(ctx context.Context, tx *gorm.DB, ref SubjectRef) ([]SubjectRef, error) {
	if ref.Type != SubjectMoment {
		return nil, ErrInvalidCommand
	}
	var comments []model.MomentComment
	if err := tx.WithContext(ctx).Where("moment_id = ?", ref.ID).Find(&comments).Error; err != nil {
		return nil, err
	}
	refs := make([]SubjectRef, 0, len(comments))
	commentIDs := make([]uint, 0, len(comments))
	for _, row := range comments {
		commentIDs = append(commentIDs, row.ID)
		refs = append(refs, SubjectRef{Type: SubjectMomentComment, ID: uint64(row.ID), RootID: ref.ID})
	}
	if len(commentIDs) > 0 {
		var replies []model.MomentCommentReply
		if err := tx.WithContext(ctx).Where("comment_id IN ?", commentIDs).Find(&replies).Error; err != nil {
			return nil, err
		}
		for _, row := range replies {
			refs = append(refs, SubjectRef{Type: SubjectMomentCommentReply, ID: uint64(row.ID), RootID: uint64(row.CommentID), ParentID: uint64Pointer(uint64(row.ParentReplyID))})
		}
	}
	sortSubjectRefs(refs)
	return refs, nil
}
