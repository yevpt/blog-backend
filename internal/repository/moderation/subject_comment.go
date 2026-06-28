package moderation

import (
	"context"
	"errors"
	"sort"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type commentAdapter struct{}

func (a *commentAdapter) Load(ctx context.Context, db *gorm.DB, ref SubjectRef) (SubjectSnapshot, error) {
	switch ref.Type {
	case SubjectArticleComment:
		var row model.ArticleComment
		if err := db.WithContext(ctx).First(&row, ref.ID).Error; err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return SubjectSnapshot{Ref: SubjectRef{Type: ref.Type, ID: uint64(row.ID), RootID: uint64(row.ArticleID)}, AuthorID: uint64(row.UserID), Content: row.Content}, nil
	case SubjectMomentComment:
		var row model.MomentComment
		if err := db.WithContext(ctx).First(&row, ref.ID).Error; err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return SubjectSnapshot{Ref: SubjectRef{Type: ref.Type, ID: uint64(row.ID), RootID: uint64(row.MomentID)}, AuthorID: uint64(row.UserID), Content: row.Content}, nil
	case SubjectArticleCommentReply:
		var row model.ArticleCommentReply
		if err := db.WithContext(ctx).First(&row, ref.ID).Error; err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return replySnapshot(ref.Type, row.ID, row.CommentID, row.ParentReplyID, row.FromUserID, row.Content), nil
	case SubjectMomentCommentReply:
		var row model.MomentCommentReply
		if err := db.WithContext(ctx).First(&row, ref.ID).Error; err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return replySnapshot(ref.Type, row.ID, row.CommentID, row.ParentReplyID, row.FromUserID, row.Content), nil
	default:
		return SubjectSnapshot{}, ErrInvalidCommand
	}
}

func (a *commentAdapter) Lock(ctx context.Context, tx *gorm.DB, ref SubjectRef, authorID uint64) (SubjectSnapshot, error) {
	query := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"})
	switch ref.Type {
	case SubjectArticleComment:
		var row model.ArticleComment
		err := query.Where("id = ? AND user_id = ? AND article_id = ?", ref.ID, authorID, ref.RootID).Take(&row).Error
		if err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return SubjectSnapshot{Ref: SubjectRef{Type: ref.Type, ID: uint64(row.ID), RootID: uint64(row.ArticleID)}, AuthorID: uint64(row.UserID), Content: row.Content}, nil
	case SubjectMomentComment:
		var row model.MomentComment
		err := query.Where("id = ? AND user_id = ? AND moment_id = ?", ref.ID, authorID, ref.RootID).Take(&row).Error
		if err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return SubjectSnapshot{Ref: SubjectRef{Type: ref.Type, ID: uint64(row.ID), RootID: uint64(row.MomentID)}, AuthorID: uint64(row.UserID), Content: row.Content}, nil
	case SubjectArticleCommentReply:
		if ref.ParentID == nil {
			return SubjectSnapshot{}, ErrInvalidCommand
		}
		var row model.ArticleCommentReply
		err := query.Where("id = ? AND from_user_id = ? AND comment_id = ? AND parent_reply_id = ?", ref.ID, authorID, ref.RootID, *ref.ParentID).Take(&row).Error
		if err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return replySnapshot(ref.Type, row.ID, row.CommentID, row.ParentReplyID, row.FromUserID, row.Content), nil
	case SubjectMomentCommentReply:
		if ref.ParentID == nil {
			return SubjectSnapshot{}, ErrInvalidCommand
		}
		var row model.MomentCommentReply
		err := query.Where("id = ? AND from_user_id = ? AND comment_id = ? AND parent_reply_id = ?", ref.ID, authorID, ref.RootID, *ref.ParentID).Take(&row).Error
		if err != nil {
			return SubjectSnapshot{}, subjectError(err)
		}
		return replySnapshot(ref.Type, row.ID, row.CommentID, row.ParentReplyID, row.FromUserID, row.Content), nil
	default:
		return SubjectSnapshot{}, ErrInvalidCommand
	}
}

func (a *commentAdapter) Materialize(ctx context.Context, tx *gorm.DB, cmd MaterializeCommand) error {
	if cmd.Ref.RootID == 0 || cmd.AuthorID == 0 {
		return ErrInvalidCommand
	}
	if cmd.Create {
		return a.create(ctx, tx, cmd)
	}
	if cmd.Ref.ID == 0 {
		return ErrInvalidCommand
	}
	query := tx.WithContext(ctx)
	var result *gorm.DB
	switch cmd.Ref.Type {
	case SubjectArticleComment:
		result = query.Model(&model.ArticleComment{}).
			Where("id = ? AND user_id = ? AND article_id = ?", cmd.Ref.ID, cmd.AuthorID, cmd.Ref.RootID).
			UpdateColumn("content", cmd.Content)
	case SubjectMomentComment:
		result = query.Model(&model.MomentComment{}).
			Where("id = ? AND user_id = ? AND moment_id = ?", cmd.Ref.ID, cmd.AuthorID, cmd.Ref.RootID).
			UpdateColumn("content", cmd.Content)
	case SubjectArticleCommentReply:
		if cmd.Ref.ParentID == nil {
			return ErrInvalidCommand
		}
		result = query.Model(&model.ArticleCommentReply{}).
			Where("id = ? AND from_user_id = ? AND comment_id = ? AND parent_reply_id = ?", cmd.Ref.ID, cmd.AuthorID, cmd.Ref.RootID, *cmd.Ref.ParentID).
			UpdateColumn("content", cmd.Content)
	case SubjectMomentCommentReply:
		if cmd.Ref.ParentID == nil {
			return ErrInvalidCommand
		}
		result = query.Model(&model.MomentCommentReply{}).
			Where("id = ? AND from_user_id = ? AND comment_id = ? AND parent_reply_id = ?", cmd.Ref.ID, cmd.AuthorID, cmd.Ref.RootID, *cmd.Ref.ParentID).
			UpdateColumn("content", cmd.Content)
	default:
		return ErrInvalidCommand
	}
	return subjectUpdateError(result)
}

func (a *commentAdapter) create(ctx context.Context, tx *gorm.DB, cmd MaterializeCommand) error {
	if cmd.AssignedID == nil || cmd.Ref.ID != 0 {
		return ErrInvalidCommand
	}
	var id uint
	switch cmd.Ref.Type {
	case SubjectArticleComment:
		if err := ensureArticleCommentable(ctx, tx, cmd.Ref.RootID); err != nil {
			return err
		}
		row := model.ArticleComment{ArticleID: uint(cmd.Ref.RootID), UserID: uint(cmd.AuthorID), Content: cmd.Content}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		id = row.ID
	case SubjectMomentComment:
		if err := ensureMomentCommentable(ctx, tx, cmd.Ref.RootID); err != nil {
			return err
		}
		row := model.MomentComment{MomentID: uint(cmd.Ref.RootID), UserID: uint(cmd.AuthorID), Content: cmd.Content}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		id = row.ID
	case SubjectArticleCommentReply:
		if cmd.Ref.ParentID == nil {
			return ErrInvalidCommand
		}
		toUserID, err := articleReplyRecipient(ctx, tx, cmd.Ref)
		if err != nil {
			return err
		}
		row := model.ArticleCommentReply{CommentID: uint(cmd.Ref.RootID), ParentReplyID: uint(*cmd.Ref.ParentID), FromUserID: uint(cmd.AuthorID), ToUserID: toUserID, Content: cmd.Content}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		id = row.ID
	case SubjectMomentCommentReply:
		if cmd.Ref.ParentID == nil {
			return ErrInvalidCommand
		}
		toUserID, err := momentReplyRecipient(ctx, tx, cmd.Ref)
		if err != nil {
			return err
		}
		row := model.MomentCommentReply{CommentID: uint(cmd.Ref.RootID), ParentReplyID: uint(*cmd.Ref.ParentID), FromUserID: uint(cmd.AuthorID), ToUserID: toUserID, Content: cmd.Content}
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

func ensureArticleCommentable(ctx context.Context, tx *gorm.DB, articleID uint64) error {
	var article model.Article
	err := tx.WithContext(ctx).Select("id").
		Where("id = ? AND status IN ? AND comment_status = ?", articleID, []uint{uint(model.ArticleStatusPublic), uint(model.ArticleStatusEncrypted)}, uint8(1)).
		Take(&article).Error
	return subjectError(err)
}

func ensureMomentCommentable(ctx context.Context, tx *gorm.DB, momentID uint64) error {
	var moment model.Moment
	err := tx.WithContext(ctx).Select("id").
		Where("id = ? AND status = ? AND comment_status = ?", momentID, uint8(1), uint8(1)).
		Take(&moment).Error
	return subjectError(err)
}

func ensureActiveUser(ctx context.Context, tx *gorm.DB, userID uint64) error {
	var user model.User
	err := tx.WithContext(ctx).Select("id").Where("id = ? AND status = ?", userID, uint8(1)).Take(&user).Error
	return subjectError(err)
}

func articleReplyRecipient(ctx context.Context, tx *gorm.DB, ref SubjectRef) (uint, error) {
	if ref.ParentID == nil {
		return 0, ErrInvalidCommand
	}
	if *ref.ParentID == 0 {
		var comment model.ArticleComment
		if err := tx.WithContext(ctx).Select("user_id").Where("id = ?", ref.RootID).Take(&comment).Error; err != nil {
			return 0, subjectError(err)
		}
		return comment.UserID, nil
	}
	var parent model.ArticleCommentReply
	if err := tx.WithContext(ctx).Select("from_user_id").Where("id = ? AND comment_id = ?", *ref.ParentID, ref.RootID).Take(&parent).Error; err != nil {
		return 0, subjectError(err)
	}
	return parent.FromUserID, nil
}

func momentReplyRecipient(ctx context.Context, tx *gorm.DB, ref SubjectRef) (uint, error) {
	if ref.ParentID == nil {
		return 0, ErrInvalidCommand
	}
	if *ref.ParentID == 0 {
		var comment model.MomentComment
		if err := tx.WithContext(ctx).Select("user_id").Where("id = ?", ref.RootID).Take(&comment).Error; err != nil {
			return 0, subjectError(err)
		}
		return comment.UserID, nil
	}
	var parent model.MomentCommentReply
	if err := tx.WithContext(ctx).Select("from_user_id").Where("id = ? AND comment_id = ?", *ref.ParentID, ref.RootID).Take(&parent).Error; err != nil {
		return 0, subjectError(err)
	}
	return parent.FromUserID, nil
}

func (a *commentAdapter) Delete(ctx context.Context, tx *gorm.DB, ref SubjectRef) error {
	var result *gorm.DB
	switch ref.Type {
	case SubjectArticleComment:
		result = tx.WithContext(ctx).Delete(&model.ArticleComment{}, ref.ID)
	case SubjectMomentComment:
		result = tx.WithContext(ctx).Delete(&model.MomentComment{}, ref.ID)
	case SubjectArticleCommentReply:
		result = tx.WithContext(ctx).Delete(&model.ArticleCommentReply{}, ref.ID)
	case SubjectMomentCommentReply:
		result = tx.WithContext(ctx).Delete(&model.MomentCommentReply{}, ref.ID)
	default:
		return ErrInvalidCommand
	}
	return requireSubjectRow(result)
}

func (a *commentAdapter) Descendants(ctx context.Context, tx *gorm.DB, ref SubjectRef) ([]SubjectRef, error) {
	var refs []SubjectRef
	switch ref.Type {
	case SubjectArticleComment:
		var rows []model.ArticleCommentReply
		if err := tx.WithContext(ctx).Where("comment_id = ?", ref.ID).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			refs = append(refs, SubjectRef{Type: SubjectArticleCommentReply, ID: uint64(row.ID), RootID: ref.ID, ParentID: uint64Pointer(uint64(row.ParentReplyID))})
		}
	case SubjectMomentComment:
		var rows []model.MomentCommentReply
		if err := tx.WithContext(ctx).Where("comment_id = ?", ref.ID).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			refs = append(refs, SubjectRef{Type: SubjectMomentCommentReply, ID: uint64(row.ID), RootID: ref.ID, ParentID: uint64Pointer(uint64(row.ParentReplyID))})
		}
	case SubjectArticleCommentReply:
		return articleReplyDescendants(ctx, tx, ref)
	case SubjectMomentCommentReply:
		return momentReplyDescendants(ctx, tx, ref)
	default:
		return nil, ErrInvalidCommand
	}
	sortSubjectRefs(refs)
	return refs, nil
}

type replyDescendantRow struct {
	ID            uint64
	CommentID     uint64
	ParentReplyID uint64
}

func articleReplyDescendants(ctx context.Context, tx *gorm.DB, ref SubjectRef) ([]SubjectRef, error) {
	const query = "WITH RECURSIVE reply_tree AS (SELECT id, comment_id, parent_reply_id FROM `article_comment_reply` WHERE parent_reply_id = ? AND comment_id = ? AND deleted_at IS NULL UNION ALL SELECT child.id, child.comment_id, child.parent_reply_id FROM `article_comment_reply` AS child JOIN reply_tree AS parent ON child.parent_reply_id = parent.id WHERE child.comment_id = ? AND child.deleted_at IS NULL) SELECT id, comment_id, parent_reply_id FROM reply_tree ORDER BY id ASC"
	return loadReplyDescendants(ctx, tx, ref.Type, ref, query)
}

func momentReplyDescendants(ctx context.Context, tx *gorm.DB, ref SubjectRef) ([]SubjectRef, error) {
	const query = "WITH RECURSIVE reply_tree AS (SELECT id, comment_id, parent_reply_id FROM `moment_comment_reply` WHERE parent_reply_id = ? AND comment_id = ? AND deleted_at IS NULL UNION ALL SELECT child.id, child.comment_id, child.parent_reply_id FROM `moment_comment_reply` AS child JOIN reply_tree AS parent ON child.parent_reply_id = parent.id WHERE child.comment_id = ? AND child.deleted_at IS NULL) SELECT id, comment_id, parent_reply_id FROM reply_tree ORDER BY id ASC"
	return loadReplyDescendants(ctx, tx, ref.Type, ref, query)
}

func loadReplyDescendants(ctx context.Context, tx *gorm.DB, subjectType SubjectType, ref SubjectRef, query string) ([]SubjectRef, error) {
	if ref.ParentID == nil {
		return nil, ErrInvalidCommand
	}
	var rows []replyDescendantRow
	if err := tx.WithContext(ctx).Raw(query, ref.ID, ref.RootID, ref.RootID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	refs := make([]SubjectRef, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, SubjectRef{Type: subjectType, ID: row.ID, RootID: row.CommentID, ParentID: uint64Pointer(row.ParentReplyID)})
	}
	sortSubjectRefs(refs)
	return refs, nil
}

func replySnapshot(subjectType SubjectType, id, commentID, parentID, authorID uint, content string) SubjectSnapshot {
	return SubjectSnapshot{
		Ref:      SubjectRef{Type: subjectType, ID: uint64(id), RootID: uint64(commentID), ParentID: uint64Pointer(uint64(parentID))},
		AuthorID: uint64(authorID), Content: content,
	}
}

func uint64Pointer(value uint64) *uint64 { return &value }

func subjectError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSubjectNotFound
	}
	return err
}

func requireSubjectRow(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrSubjectNotFound
	}
	return nil
}

func subjectUpdateError(result *gorm.DB) error {
	return result.Error
}

func sortSubjectRefs(refs []SubjectRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Type == refs[j].Type {
			return refs[i].ID < refs[j].ID
		}
		return refs[i].Type < refs[j].Type
	})
}
