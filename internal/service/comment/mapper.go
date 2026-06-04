package comment

import (
	"context"
	"strings"

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
	replies := make([]dto.CommentReplyResp, 0, len(aggregate.Replies))
	for _, reply := range aggregate.Replies {
		replies = append(replies, *replyToDTO(reply, resolver))
	}
	return &dto.CommentItemResp{
		ID:         aggregate.Comment.ID,
		TargetType: targetTypeName(commentType),
		TargetID:   aggregate.Comment.TargetID,
		UserID:     aggregate.Comment.UserID,
		Content:    aggregate.Comment.Content,
		User:       userToDTO(aggregate.User, resolver),
		Replies:    replies,
		CreatedAt:  aggregate.Comment.CreatedAt,
		UpdatedAt:  aggregate.Comment.UpdatedAt,
	}
}

func replyToDTO(aggregate commentrepo.ReplyAggregate, resolver storage.ObjectURLResolver) *dto.CommentReplyResp {
	reply := aggregate.Reply
	return &dto.CommentReplyResp{
		ID:            reply.ID,
		TargetType:    targetTypeName(reply.CommentType),
		CommentID:     reply.CommentID,
		FromUserID:    reply.FromUserID,
		ToUserID:      reply.ToUserID,
		ParentReplyID: reply.ParentReplyID,
		Content:       reply.Content,
		FromUser:      userToDTO(aggregate.FromUser, resolver),
		ToUser:        userToDTO(aggregate.ToUser, resolver),
		CreatedAt:     reply.CreatedAt,
		UpdatedAt:     reply.UpdatedAt,
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
		AvatarUrl: resolvePtrURL(resolver, user.AvatarUrl),
		Site:      user.Site,
		Mark:      user.Mark,
	}
}

func resolvePtrURL(resolver storage.ObjectURLResolver, url *string) *string {
	if url == nil || resolver == nil {
		return url
	}
	trimmed := strings.TrimSpace(*url)
	if trimmed == "" || isAbsoluteURL(trimmed) {
		return url
	}
	if resolved, err := resolver.ObjectURL(context.Background(), trimmed); err == nil {
		return &resolved
	}
	return url
}

func isAbsoluteURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
