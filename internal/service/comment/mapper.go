package comment

import (
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
)

func commentPageToDTO(result *commentrepo.PageResult, commentType uint8) *dto.CommentPageResp {
	pages := 0
	if result.PageSize > 0 {
		pages = int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize))
	}

	items := make([]dto.CommentItemResp, 0, len(result.Comments))
	for _, aggregate := range result.Comments {
		items = append(items, *commentToDTO(aggregate, commentType))
	}
	return &dto.CommentPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}
}

func commentToDTO(aggregate commentrepo.CommentAggregate, commentType uint8) *dto.CommentItemResp {
	replies := make([]dto.CommentReplyResp, 0, len(aggregate.Replies))
	for _, reply := range aggregate.Replies {
		replies = append(replies, *replyToDTO(reply))
	}
	return &dto.CommentItemResp{
		ID:         aggregate.Comment.ID,
		TargetType: targetTypeName(commentType),
		TargetID:   aggregate.Comment.TargetID,
		UserID:     aggregate.Comment.UserID,
		Content:    aggregate.Comment.Content,
		User:       userToDTO(aggregate.User),
		Replies:    replies,
		CreatedAt:  aggregate.Comment.CreatedAt,
		UpdatedAt:  aggregate.Comment.UpdatedAt,
	}
}

func replyToDTO(aggregate commentrepo.ReplyAggregate) *dto.CommentReplyResp {
	reply := aggregate.Reply
	return &dto.CommentReplyResp{
		ID:            reply.ID,
		TargetType:    targetTypeName(reply.CommentType),
		CommentID:     reply.CommentID,
		FromUserID:    reply.FromUserID,
		ToUserID:      reply.ToUserID,
		ParentReplyID: reply.ParentReplyID,
		Content:       reply.Content,
		FromUser:      userToDTO(aggregate.FromUser),
		ToUser:        userToDTO(aggregate.ToUser),
		CreatedAt:     reply.CreatedAt,
		UpdatedAt:     reply.UpdatedAt,
	}
}

func userToDTO(user *model.User) *dto.CommentUserResp {
	if user == nil {
		return nil
	}
	return &dto.CommentUserResp{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		AvatarUrl: user.AvatarUrl,
		Site:      user.Site,
		Mark:      user.Mark,
	}
}
