package comment

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *commentRepo) Reply(data ReplyData) (*ReplyAggregate, error) {
	comment, err := r.findCommentByID(data.Target.Type, data.CommentID)
	if err != nil {
		return nil, err
	}

	toUserID, err := r.replyToUserID(data, comment)
	if err != nil {
		return nil, err
	}

	reply, err := r.createReplyRecord(data.Target.Type, ReplyRecord{
		CommentID:     data.CommentID,
		ToUserID:      toUserID,
		FromUserID:    data.FromUserID,
		ParentReplyID: data.ParentReplyID,
		Content:       data.Content,
	})
	if err != nil {
		return nil, err
	}
	return r.replyAggregate(data.Target, *reply)
}

func (r *commentRepo) replyToUserID(data ReplyData, comment *CommentRecord) (uint, error) {
	if data.ParentReplyID == 0 {
		return comment.UserID, nil
	}

	parent, err := r.findReplyByID(data.Target.Type, data.ParentReplyID)
	if err != nil {
		return 0, err
	}
	if parent.CommentID != data.CommentID {
		return 0, ErrReplyNotFound
	}
	return parent.FromUserID, nil
}

func (r *commentRepo) countReplies(commentType uint8, commentID uint) (int64, error) {
	var total int64
	err := r.replyTable(commentType).
		Where("comment_id = ?", commentID).
		Count(&total).Error
	return total, err
}

func (r *commentRepo) replyCounts(commentType uint8, commentIDs []uint) (map[uint]int64, error) {
	counts := make(map[uint]int64, len(commentIDs))
	if len(commentIDs) == 0 {
		return counts, nil
	}

	var rows []struct {
		CommentID uint
		Count     int64
	}
	err := r.replyTable(commentType).
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

func (r *commentRepo) listReplies(commentType uint8, commentID uint, page int, pageSize int) ([]ReplyRecord, error) {
	offset := (page - 1) * pageSize
	switch commentType {
	case TargetArticle:
		var replies []model.ArticleCommentReply
		err := r.replyTable(commentType).
			Where("comment_id = ?", commentID).
			Order("created_at ASC").
			Order("id ASC").
			Limit(pageSize).
			Offset(offset).
			Find(&replies).Error
		return articleReplyRecords(replies), err
	case TargetMoment:
		var replies []model.MomentCommentReply
		err := r.replyTable(commentType).
			Where("comment_id = ?", commentID).
			Order("created_at ASC").
			Order("id ASC").
			Limit(pageSize).
			Offset(offset).
			Find(&replies).Error
		return momentReplyRecords(replies), err
	case TargetGuestbook:
		var replies []model.GuestbookReply
		err := r.replyTable(commentType).
			Where("comment_id = ?", commentID).
			Order("created_at ASC").
			Order("id ASC").
			Limit(pageSize).
			Offset(offset).
			Find(&replies).Error
		return guestbookReplyRecords(replies), err
	default:
		return nil, ErrReplyNotFound
	}
}

func (r *commentRepo) attachReplyRelations(target Target, replies []ReplyRecord, viewerID *uint) ([]ReplyAggregate, error) {
	userIDs := make([]uint, 0, len(replies)*2)
	replyIDs := make([]uint, 0, len(replies))
	for _, reply := range replies {
		userIDs = append(userIDs, reply.FromUserID, reply.ToUserID)
		replyIDs = append(replyIDs, reply.ID)
	}

	userMap, err := r.usersByID(userIDs)
	if err != nil {
		return nil, err
	}
	likeCounts, err := r.replyLikeCounts(target.Type, replyIDs)
	if err != nil {
		return nil, err
	}
	likedIDs, err := r.replyLikedIDs(target.Type, replyIDs, viewerID)
	if err != nil {
		return nil, err
	}

	aggregates := make([]ReplyAggregate, 0, len(replies))
	for _, reply := range replies {
		aggregates = append(aggregates, ReplyAggregate{
			Reply:     reply,
			FromUser:  userMap[reply.FromUserID],
			ToUser:    userMap[reply.ToUserID],
			LikeCount: likeCounts[reply.ID],
			IsLiked:   likedIDs[reply.ID],
		})
	}
	return aggregates, nil
}

func (r *commentRepo) replyAggregate(target Target, reply ReplyRecord) (*ReplyAggregate, error) {
	userMap, err := r.usersByID([]uint{reply.FromUserID, reply.ToUserID})
	if err != nil {
		return nil, err
	}
	likeCounts, err := r.replyLikeCounts(target.Type, []uint{reply.ID})
	if err != nil {
		return nil, err
	}
	return &ReplyAggregate{
		Reply:     reply,
		FromUser:  userMap[reply.FromUserID],
		ToUser:    userMap[reply.ToUserID],
		LikeCount: likeCounts[reply.ID],
		IsLiked:   false,
	}, nil
}

func (r *commentRepo) createReplyRecord(commentType uint8, record ReplyRecord) (*ReplyRecord, error) {
	switch commentType {
	case TargetArticle:
		reply := model.ArticleCommentReply{
			CommentID:     record.CommentID,
			ToUserID:      record.ToUserID,
			FromUserID:    record.FromUserID,
			ParentReplyID: record.ParentReplyID,
			Content:       record.Content,
		}
		if err := r.db.Create(&reply).Error; err != nil {
			return nil, err
		}
		return articleReplyRecord(reply), nil
	case TargetMoment:
		reply := model.MomentCommentReply{
			CommentID:     record.CommentID,
			ToUserID:      record.ToUserID,
			FromUserID:    record.FromUserID,
			ParentReplyID: record.ParentReplyID,
			Content:       record.Content,
		}
		if err := r.db.Create(&reply).Error; err != nil {
			return nil, err
		}
		return momentReplyRecord(reply), nil
	case TargetGuestbook:
		reply := model.GuestbookReply{
			CommentID:     record.CommentID,
			ToUserID:      record.ToUserID,
			FromUserID:    record.FromUserID,
			ParentReplyID: record.ParentReplyID,
			Content:       record.Content,
		}
		if err := r.db.Create(&reply).Error; err != nil {
			return nil, err
		}
		return guestbookReplyRecord(reply), nil
	default:
		return nil, ErrReplyNotFound
	}
}

func (r *commentRepo) findReplyByID(commentType uint8, replyID uint) (*ReplyRecord, error) {
	switch commentType {
	case TargetArticle:
		var reply model.ArticleCommentReply
		err := r.db.Where("id = ?", replyID).First(&reply).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReplyNotFound
		}
		if err != nil {
			return nil, err
		}
		return articleReplyRecord(reply), nil
	case TargetMoment:
		var reply model.MomentCommentReply
		err := r.db.Where("id = ?", replyID).First(&reply).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReplyNotFound
		}
		if err != nil {
			return nil, err
		}
		return momentReplyRecord(reply), nil
	case TargetGuestbook:
		var reply model.GuestbookReply
		err := r.db.Where("id = ?", replyID).First(&reply).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReplyNotFound
		}
		if err != nil {
			return nil, err
		}
		return guestbookReplyRecord(reply), nil
	default:
		return nil, ErrReplyNotFound
	}
}

func (r *commentRepo) replyTable(commentType uint8) *gorm.DB {
	switch commentType {
	case TargetArticle:
		return r.db.Model(&model.ArticleCommentReply{})
	case TargetMoment:
		return r.db.Model(&model.MomentCommentReply{})
	case TargetGuestbook:
		return r.db.Model(&model.GuestbookReply{})
	default:
		return nil
	}
}

func articleReplyRecord(reply model.ArticleCommentReply) *ReplyRecord {
	return &ReplyRecord{
		ID:            reply.ID,
		CommentID:     reply.CommentID,
		ToUserID:      reply.ToUserID,
		FromUserID:    reply.FromUserID,
		ParentReplyID: reply.ParentReplyID,
		Content:       reply.Content,
		CreatedAt:     reply.CreatedAt,
		UpdatedAt:     reply.UpdatedAt,
	}
}

func momentReplyRecord(reply model.MomentCommentReply) *ReplyRecord {
	return &ReplyRecord{
		ID:            reply.ID,
		CommentID:     reply.CommentID,
		ToUserID:      reply.ToUserID,
		FromUserID:    reply.FromUserID,
		ParentReplyID: reply.ParentReplyID,
		Content:       reply.Content,
		CreatedAt:     reply.CreatedAt,
		UpdatedAt:     reply.UpdatedAt,
	}
}

func guestbookReplyRecord(reply model.GuestbookReply) *ReplyRecord {
	return &ReplyRecord{
		ID:            reply.ID,
		CommentID:     reply.CommentID,
		ToUserID:      reply.ToUserID,
		FromUserID:    reply.FromUserID,
		ParentReplyID: reply.ParentReplyID,
		Content:       reply.Content,
		CreatedAt:     reply.CreatedAt,
		UpdatedAt:     reply.UpdatedAt,
	}
}

func articleReplyRecords(replies []model.ArticleCommentReply) []ReplyRecord {
	records := make([]ReplyRecord, 0, len(replies))
	for _, reply := range replies {
		records = append(records, *articleReplyRecord(reply))
	}
	return records
}

func momentReplyRecords(replies []model.MomentCommentReply) []ReplyRecord {
	records := make([]ReplyRecord, 0, len(replies))
	for _, reply := range replies {
		records = append(records, *momentReplyRecord(reply))
	}
	return records
}

func guestbookReplyRecords(replies []model.GuestbookReply) []ReplyRecord {
	records := make([]ReplyRecord, 0, len(replies))
	for _, reply := range replies {
		records = append(records, *guestbookReplyRecord(reply))
	}
	return records
}
