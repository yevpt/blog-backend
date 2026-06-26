package comment

import (
	"sort"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *commentRepo) ListAdmin(targetType uint8, search string, page int, pageSize int) (*AdminPageResult, error) {
	targets, err := adminCommentTargets(targetType)
	if err != nil {
		return nil, err
	}

	page, pageSize = normalizePage(page, pageSize)
	total, err := r.countAdminComments(targets, search)
	if err != nil {
		return nil, err
	}

	comments, err := r.listAdminComments(targets, search, page, pageSize)
	if err != nil {
		return nil, err
	}

	return &AdminPageResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Comments: comments,
	}, nil
}

func adminCommentTargets(targetType uint8) ([]uint8, error) {
	switch targetType {
	case TargetAll:
		return []uint8{TargetArticle, TargetMoment}, nil
	case TargetArticle, TargetMoment:
		return []uint8{targetType}, nil
	default:
		return nil, ErrTargetNotFound
	}
}

func (r *commentRepo) countAdminComments(targets []uint8, search string) (int64, error) {
	var total int64
	for _, targetType := range targets {
		var count int64
		query := r.adminCommentTable(targetType, search)
		if query == nil {
			return 0, ErrTargetNotFound
		}
		if err := query.Count(&count).Error; err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func (r *commentRepo) listAdminComments(targets []uint8, search string, page int, pageSize int) ([]AdminCommentAggregate, error) {
	offset := (page - 1) * pageSize
	limit := pageSize
	if len(targets) > 1 {
		limit = offset + pageSize
		offset = 0
	}

	comments := make([]AdminCommentAggregate, 0, limit*len(targets))
	for _, targetType := range targets {
		records, err := r.listAdminCommentRecords(targetType, search, offset, limit)
		if err != nil {
			return nil, err
		}
		aggregates, err := r.attachCommentRelations(Target{Type: targetType}, records, nil)
		if err != nil {
			return nil, err
		}
		for _, aggregate := range aggregates {
			comments = append(comments, AdminCommentAggregate{
				TargetType: targetType,
				Comment:    aggregate.Comment,
				User:       aggregate.User,
				ReplyCount: aggregate.ReplyCount,
				LikeCount:  aggregate.LikeCount,
			})
		}
	}

	sort.SliceStable(comments, func(i int, j int) bool {
		left := comments[i].Comment
		right := comments[j].Comment
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.After(right.CreatedAt)
		}
		if left.ID != right.ID {
			return left.ID > right.ID
		}
		return comments[i].TargetType > comments[j].TargetType
	})

	if len(targets) == 1 {
		return comments, nil
	}
	start := (page - 1) * pageSize
	if start >= len(comments) {
		return []AdminCommentAggregate{}, nil
	}
	end := start + pageSize
	if end > len(comments) {
		end = len(comments)
	}
	return comments[start:end], nil
}

func (r *commentRepo) listAdminCommentRecords(targetType uint8, search string, offset int, limit int) ([]CommentRecord, error) {
	query := r.adminCommentTable(targetType, search)
	if query == nil {
		return nil, ErrTargetNotFound
	}

	switch targetType {
	case TargetArticle:
		var comments []model.ArticleComment
		err := query.Order("created_at DESC").Order("id DESC").Limit(limit).Offset(offset).Find(&comments).Error
		return articleCommentRecords(comments), err
	case TargetMoment:
		var comments []model.MomentComment
		err := query.Order("created_at DESC").Order("id DESC").Limit(limit).Offset(offset).Find(&comments).Error
		return momentCommentRecords(comments), err
	default:
		return nil, ErrTargetNotFound
	}
}

func (r *commentRepo) adminCommentTable(targetType uint8, search string) *gorm.DB {
	var query *gorm.DB
	switch targetType {
	case TargetArticle:
		query = r.db.Model(&model.ArticleComment{})
	case TargetMoment:
		query = r.db.Model(&model.MomentComment{})
	default:
		return nil
	}

	if keyword := strings.TrimSpace(search); keyword != "" {
		query = query.Where("content LIKE ?", "%"+keyword+"%")
	}
	return query
}
