package comment

import (
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"github.com/vpt/blog-backend/pkg/storage"
)

func commentPageToDTO(result *commentrepo.PageResult, commentType uint8, resolver storage.ObjectURLResolver) *dto.CommentPageResp {
	pages := 0
	if result.PageSize > 0 {
		pages = int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize))
	}

	items := make([]dto.CommentItemResp, 0, len(result.Comments))
	for _, aggregate := range result.Comments {
		items = append(items, *commentToDTO(aggregate, commentType, resolver))
	}
	return &dto.CommentPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}
}

func commentToDTO(aggregate commentrepo.CommentAggregate, commentType uint8, resolver storage.ObjectURLResolver) *dto.CommentItemResp {
	return &dto.CommentItemResp{
		ID:         aggregate.Comment.ID,
		TargetType: targetTypeName(commentType),
		TargetID:   aggregate.Comment.TargetID,
		UserID:     aggregate.Comment.UserID,
		Content:    aggregate.Comment.Content,
		User:       userToDTO(aggregate.User, resolver),
		ReplyCount: aggregate.ReplyCount,
		LikeCount:  aggregate.LikeCount,
		IsLiked:    aggregate.IsLiked,
		CreatedAt:  aggregate.Comment.CreatedAt,
		UpdatedAt:  aggregate.Comment.UpdatedAt,
	}
}

func replyToDTO(aggregate commentrepo.ReplyAggregate, resolver storage.ObjectURLResolver) *dto.CommentReplyResp {
	reply := aggregate.Reply
	return &dto.CommentReplyResp{
		ID:            reply.ID,
		TargetType:    "",
		CommentID:     reply.CommentID,
		FromUserID:    reply.FromUserID,
		ToUserID:      reply.ToUserID,
		ParentReplyID: reply.ParentReplyID,
		Content:       reply.Content,
		FromUser:      userToDTO(aggregate.FromUser, resolver),
		ToUser:        userToDTO(aggregate.ToUser, resolver),
		LikeCount:     aggregate.LikeCount,
		IsLiked:       aggregate.IsLiked,
		CreatedAt:     reply.CreatedAt,
		UpdatedAt:     reply.UpdatedAt,
	}
}

func replyPageToDTO(result *commentrepo.ReplyPageResult, commentType uint8, resolver storage.ObjectURLResolver) *dto.CommentReplyPageResp {
	pages := 0
	if result.PageSize > 0 {
		pages = int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize))
	}

	items := make([]dto.CommentReplyResp, 0, len(result.Replies))
	for _, aggregate := range result.Replies {
		item := replyToDTO(aggregate, resolver)
		item.TargetType = targetTypeName(commentType)
		items = append(items, *item)
	}
	return &dto.CommentReplyPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}
}

func userToDTO(user *model.User, resolver storage.ObjectURLResolver) *dto.CommentUserResp {
	if user == nil {
		return nil
	}
	return &dto.CommentUserResp{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		AvatarUrl: storage.ResolvePtrURL(resolver, user.AvatarUrl),
		Site:      user.Site,
		Mark:      user.Mark,
	}
}
