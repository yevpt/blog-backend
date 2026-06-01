package comment

import (
	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *commentRepo) List(target Target, page int, pageSize int) (*PageResult, error) {
	if err := r.ensureTargetReadable(target); err != nil {
		return nil, err
	}

	page, pageSize = normalizePage(page, pageSize)
	total, err := r.countComments(target)
	if err != nil {
		return nil, err
	}

	comments, err := r.listComments(target, page, pageSize)
	if err != nil {
		return nil, err
	}
	aggregates, err := r.attachCommentRelations(target.Type, comments)
	if err != nil {
		return nil, err
	}

	return &PageResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Comments: aggregates,
	}, nil
}

func (r *commentRepo) ensureTargetReadable(target Target) error {
	switch target.Type {
	case TargetArticle:
		return r.ensureArticleReadable(target.ID)
	case TargetMoment:
		return r.ensureMomentReadable(target.ID)
	case TargetGuestbook:
		return r.ensureGuestbookOwner(target.ID)
	default:
		return ErrTargetNotFound
	}
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

func (r *commentRepo) countComments(target Target) (int64, error) {
	var total int64
	query := r.commentTable(target)
	if query == nil {
		return 0, ErrTargetNotFound
	}
	err := query.Count(&total).Error
	return total, err
}

func (r *commentRepo) listComments(target Target, page int, pageSize int) ([]CommentRecord, error) {
	offset := (page - 1) * pageSize
	query := r.commentTable(target)
	if query == nil {
		return nil, ErrTargetNotFound
	}

	switch target.Type {
	case TargetArticle:
		var comments []model.ArticleComment
		err := query.Order("created_at DESC").Order("id DESC").Limit(pageSize).Offset(offset).Find(&comments).Error
		return articleCommentRecords(comments), err
	case TargetMoment:
		var comments []model.MomentComment
		err := query.Order("created_at DESC").Order("id DESC").Limit(pageSize).Offset(offset).Find(&comments).Error
		return momentCommentRecords(comments), err
	case TargetGuestbook:
		var comments []model.Guestbook
		err := query.Order("created_at DESC").Order("id DESC").Limit(pageSize).Offset(offset).Find(&comments).Error
		return guestbookRecords(comments), err
	default:
		return nil, ErrTargetNotFound
	}
}

func (r *commentRepo) commentTable(target Target) *gorm.DB {
	switch target.Type {
	case TargetArticle:
		return r.db.Model(&model.ArticleComment{}).Where("article_id = ?", target.ID)
	case TargetMoment:
		return r.db.Model(&model.MomentComment{}).Where("moment_id = ?", target.ID)
	case TargetGuestbook:
		return r.db.Model(&model.Guestbook{}).Where("owner_user_id = ?", target.ID)
	default:
		return nil
	}
}

func (r *commentRepo) attachCommentRelations(commentType uint8, comments []CommentRecord) ([]CommentAggregate, error) {
	userMap, err := r.usersByID(commentUserIDs(comments))
	if err != nil {
		return nil, err
	}
	repliesByCommentID, err := r.repliesByCommentID(commentType, commentIDs(comments))
	if err != nil {
		return nil, err
	}

	aggregates := make([]CommentAggregate, 0, len(comments))
	for _, comment := range comments {
		aggregates = append(aggregates, CommentAggregate{
			Comment: comment,
			User:    userMap[comment.UserID],
			Replies: repliesByCommentID[comment.ID],
		})
	}
	return aggregates, nil
}

func articleCommentRecords(comments []model.ArticleComment) []CommentRecord {
	records := make([]CommentRecord, 0, len(comments))
	for _, comment := range comments {
		records = append(records, *articleCommentRecord(comment))
	}
	return records
}

func momentCommentRecords(comments []model.MomentComment) []CommentRecord {
	records := make([]CommentRecord, 0, len(comments))
	for _, comment := range comments {
		records = append(records, *momentCommentRecord(comment))
	}
	return records
}

func guestbookRecords(comments []model.Guestbook) []CommentRecord {
	records := make([]CommentRecord, 0, len(comments))
	for _, comment := range comments {
		records = append(records, *guestbookRecord(comment))
	}
	return records
}
