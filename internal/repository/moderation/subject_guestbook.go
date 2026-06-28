package moderation

import (
	"context"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type guestbookAdapter struct{}

func (a *guestbookAdapter) Load(ctx context.Context, db *gorm.DB, ref SubjectRef) (SubjectSnapshot, error) {
	switch ref.Type {
	case SubjectGuestbook:
		var row model.Guestbook
		if err := db.WithContext(ctx).First(&row, ref.ID).Error; err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return SubjectSnapshot{Ref: SubjectRef{Type: ref.Type, ID: uint64(row.ID), RootID: uint64(row.OwnerUserID)}, AuthorID: uint64(row.FromUserID), Content: row.Content}, nil
	case SubjectGuestbookReply:
		var row model.GuestbookReply
		if err := db.WithContext(ctx).First(&row, ref.ID).Error; err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return replySnapshot(ref.Type, row.ID, row.CommentID, row.ParentReplyID, row.FromUserID, row.Content), nil
	default:
		return SubjectSnapshot{}, ErrInvalidCommand
	}
}

func (a *guestbookAdapter) Lock(ctx context.Context, tx *gorm.DB, ref SubjectRef, authorID uint64) (SubjectSnapshot, error) {
	query := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"})
	switch ref.Type {
	case SubjectGuestbook:
		var row model.Guestbook
		err := query.Where("id = ? AND from_user_id = ? AND owner_user_id = ?", ref.ID, authorID, ref.RootID).Take(&row).Error
		if err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return SubjectSnapshot{Ref: SubjectRef{Type: ref.Type, ID: uint64(row.ID), RootID: uint64(row.OwnerUserID)}, AuthorID: uint64(row.FromUserID), Content: row.Content}, nil
	case SubjectGuestbookReply:
		if ref.ParentID == nil {
			return SubjectSnapshot{}, ErrInvalidCommand
		}
		var row model.GuestbookReply
		err := query.Where("id = ? AND from_user_id = ? AND comment_id = ? AND parent_reply_id = ?", ref.ID, authorID, ref.RootID, *ref.ParentID).Take(&row).Error
		if err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return replySnapshot(ref.Type, row.ID, row.CommentID, row.ParentReplyID, row.FromUserID, row.Content), nil
	default:
		return SubjectSnapshot{}, ErrInvalidCommand
	}
}

func (a *guestbookAdapter) Materialize(ctx context.Context, tx *gorm.DB, cmd MaterializeCommand) error {
	if cmd.Ref.RootID == 0 || cmd.AuthorID == 0 {
		return ErrInvalidCommand
	}
	if cmd.Create {
		return a.create(ctx, tx, cmd)
	}
	if cmd.Ref.ID == 0 {
		return ErrInvalidCommand
	}
	var result *gorm.DB
	switch cmd.Ref.Type {
	case SubjectGuestbook:
		result = tx.WithContext(ctx).Model(&model.Guestbook{}).
			Where("id = ? AND from_user_id = ? AND owner_user_id = ?", cmd.Ref.ID, cmd.AuthorID, cmd.Ref.RootID).
			UpdateColumn("content", cmd.Content)
	case SubjectGuestbookReply:
		if cmd.Ref.ParentID == nil {
			return ErrInvalidCommand
		}
		result = tx.WithContext(ctx).Model(&model.GuestbookReply{}).
			Where("id = ? AND from_user_id = ? AND comment_id = ? AND parent_reply_id = ?", cmd.Ref.ID, cmd.AuthorID, cmd.Ref.RootID, *cmd.Ref.ParentID).
			UpdateColumn("content", cmd.Content)
	default:
		return ErrInvalidCommand
	}
	return subjectUpdateError(result)
}

func (a *guestbookAdapter) create(ctx context.Context, tx *gorm.DB, cmd MaterializeCommand) error {
	if cmd.AssignedID == nil || cmd.Ref.ID != 0 {
		return ErrInvalidCommand
	}
	var id uint
	switch cmd.Ref.Type {
	case SubjectGuestbook:
		if err := ensureActiveUser(ctx, tx, cmd.Ref.RootID); err != nil {
			return err
		}
		row := model.Guestbook{OwnerUserID: uint(cmd.Ref.RootID), FromUserID: uint(cmd.AuthorID), Content: cmd.Content}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		id = row.ID
	case SubjectGuestbookReply:
		if cmd.Ref.ParentID == nil {
			return ErrInvalidCommand
		}
		toUserID, err := guestbookReplyRecipient(ctx, tx, cmd.Ref)
		if err != nil {
			return err
		}
		row := model.GuestbookReply{CommentID: uint(cmd.Ref.RootID), ParentReplyID: uint(*cmd.Ref.ParentID), FromUserID: uint(cmd.AuthorID), ToUserID: toUserID, Content: cmd.Content}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		id = row.ID
	default:
		return ErrInvalidCommand
	}
	*cmd.AssignedID = uint64(id)
	return nil
}

func guestbookReplyRecipient(ctx context.Context, tx *gorm.DB, ref SubjectRef) (uint, error) {
	if ref.ParentID == nil {
		return 0, ErrInvalidCommand
	}
	if *ref.ParentID == 0 {
		var message model.Guestbook
		if err := tx.WithContext(ctx).Select("from_user_id").Where("id = ?", ref.RootID).Take(&message).Error; err != nil {
			return 0, subjectError(err)
		}
		return message.FromUserID, nil
	}
	var parent model.GuestbookReply
	if err := tx.WithContext(ctx).Select("from_user_id").Where("id = ? AND comment_id = ?", *ref.ParentID, ref.RootID).Take(&parent).Error; err != nil {
		return 0, subjectError(err)
	}
	return parent.FromUserID, nil
}

func (a *guestbookAdapter) Delete(ctx context.Context, tx *gorm.DB, ref SubjectRef) error {
	if err := deleteSubjectLikes(ctx, tx, ref); err != nil {
		return err
	}
	var result *gorm.DB
	switch ref.Type {
	case SubjectGuestbook:
		result = tx.WithContext(ctx).Delete(&model.Guestbook{}, ref.ID)
	case SubjectGuestbookReply:
		result = tx.WithContext(ctx).Delete(&model.GuestbookReply{}, ref.ID)
	default:
		return ErrInvalidCommand
	}
	return requireSubjectRow(result)
}

func (a *guestbookAdapter) Descendants(ctx context.Context, tx *gorm.DB, ref SubjectRef) ([]SubjectRef, error) {
	if ref.Type == SubjectGuestbookReply {
		return guestbookReplyDescendants(ctx, tx, ref)
	}
	if ref.Type != SubjectGuestbook {
		return nil, ErrInvalidCommand
	}
	var rows []model.GuestbookReply
	if err := tx.WithContext(ctx).Where("comment_id = ?", ref.ID).Find(&rows).Error; err != nil {
		return nil, err
	}
	refs := make([]SubjectRef, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, SubjectRef{Type: SubjectGuestbookReply, ID: uint64(row.ID), RootID: ref.ID, ParentID: uint64Pointer(uint64(row.ParentReplyID))})
	}
	sortSubjectRefs(refs)
	return refs, nil
}

func guestbookReplyDescendants(ctx context.Context, tx *gorm.DB, ref SubjectRef) ([]SubjectRef, error) {
	const query = "WITH RECURSIVE reply_tree AS (SELECT id, comment_id, parent_reply_id FROM `guestbook_reply` WHERE parent_reply_id = ? AND comment_id = ? AND deleted_at IS NULL UNION ALL SELECT child.id, child.comment_id, child.parent_reply_id FROM `guestbook_reply` AS child JOIN reply_tree AS parent ON child.parent_reply_id = parent.id WHERE child.comment_id = ? AND child.deleted_at IS NULL) SELECT id, comment_id, parent_reply_id FROM reply_tree ORDER BY id ASC"
	return loadReplyDescendants(ctx, tx, ref.Type, ref, query)
}
