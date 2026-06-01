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

	reply := model.CommentReply{
		CommentType:   data.Target.Type,
		CommentID:     data.CommentID,
		ToUserID:      toUserID,
		FromUserID:    data.FromUserID,
		ParentReplyID: data.ParentReplyID,
		Content:       data.Content,
	}
	if err := r.db.Create(&reply).Error; err != nil {
		return nil, err
	}
	return r.replyAggregate(reply)
}

func (r *commentRepo) replyToUserID(data ReplyData, comment *CommentRecord) (uint, error) {
	if data.ParentReplyID == 0 {
		return comment.UserID, nil
	}

	var parent model.CommentReply
	err := r.db.
		Where("id = ? AND comment_type = ? AND comment_id = ?", data.ParentReplyID, data.Target.Type, data.CommentID).
		First(&parent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrReplyNotFound
	}
	if err != nil {
		return 0, err
	}
	return parent.FromUserID, nil
}

func (r *commentRepo) repliesByCommentID(commentType uint8, commentIDs []uint) (map[uint][]ReplyAggregate, error) {
	result := make(map[uint][]ReplyAggregate, len(commentIDs))
	if len(commentIDs) == 0 {
		return result, nil
	}

	var replies []model.CommentReply
	err := r.db.
		Where("comment_type = ? AND comment_id IN ?", commentType, commentIDs).
		Order("created_at ASC").
		Order("id ASC").
		Find(&replies).Error
	if err != nil {
		return nil, err
	}

	userIDs := make([]uint, 0, len(replies)*2)
	for _, reply := range replies {
		userIDs = append(userIDs, reply.FromUserID, reply.ToUserID)
	}
	userMap, err := r.usersByID(userIDs)
	if err != nil {
		return nil, err
	}

	for _, reply := range replies {
		result[reply.CommentID] = append(result[reply.CommentID], ReplyAggregate{
			Reply:    reply,
			FromUser: userMap[reply.FromUserID],
			ToUser:   userMap[reply.ToUserID],
		})
	}
	return result, nil
}

func (r *commentRepo) replyAggregate(reply model.CommentReply) (*ReplyAggregate, error) {
	userMap, err := r.usersByID([]uint{reply.FromUserID, reply.ToUserID})
	if err != nil {
		return nil, err
	}
	return &ReplyAggregate{
		Reply:    reply,
		FromUser: userMap[reply.FromUserID],
		ToUser:   userMap[reply.ToUserID],
	}, nil
}
