package comment

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"github.com/vpt/blog-backend/internal/service/commentasset"
	"github.com/vpt/blog-backend/internal/service/userrole"
	"github.com/vpt/blog-backend/pkg/storage"
)

func commentPageToDTO(result *commentrepo.PageResult, commentType uint8, resolver storage.ObjectURLResolver, rolesMap map[uint][]string) *dto.CommentPageResp {
	pages := 0
	if result.PageSize > 0 {
		pages = int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize))
	}

	items := make([]dto.CommentItemResp, 0, len(result.Comments))
	for _, aggregate := range result.Comments {
		items = append(items, *commentToDTO(aggregate, commentType, resolver, rolesMap))
	}
	return &dto.CommentPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}
}

func commentToDTO(aggregate commentrepo.CommentAggregate, commentType uint8, resolver storage.ObjectURLResolver, rolesMap map[uint][]string) *dto.CommentItemResp {
	return &dto.CommentItemResp{
		ID:         aggregate.Comment.ID,
		TargetType: targetTypeName(commentType),
		TargetID:   aggregate.Comment.TargetID,
		UserID:     aggregate.Comment.UserID,
		Content:    commentasset.ResolveContent(context.Background(), resolver, aggregate.Comment.Content),
		User:       userToDTO(aggregate.User, resolver, rolesMap),
		ReplyCount: aggregate.ReplyCount,
		LikeCount:  aggregate.LikeCount,
		IsLiked:    aggregate.IsLiked,
		CreatedAt:  aggregate.Comment.CreatedAt,
		UpdatedAt:  aggregate.Comment.UpdatedAt,
	}
}

func replyToDTO(aggregate commentrepo.ReplyAggregate, resolver storage.ObjectURLResolver, rolesMap map[uint][]string) *dto.CommentReplyResp {
	reply := aggregate.Reply
	return &dto.CommentReplyResp{
		ID:            reply.ID,
		TargetType:    "",
		CommentID:     reply.CommentID,
		FromUserID:    reply.FromUserID,
		ToUserID:      reply.ToUserID,
		ParentReplyID: reply.ParentReplyID,
		Content:       commentasset.ResolveContent(context.Background(), resolver, reply.Content),
		FromUser:      userToDTO(aggregate.FromUser, resolver, rolesMap),
		ToUser:        userToDTO(aggregate.ToUser, resolver, rolesMap),
		LikeCount:     aggregate.LikeCount,
		IsLiked:       aggregate.IsLiked,
		CreatedAt:     reply.CreatedAt,
		UpdatedAt:     reply.UpdatedAt,
	}
}

func replyPageToDTO(result *commentrepo.ReplyPageResult, commentType uint8, resolver storage.ObjectURLResolver, rolesMap map[uint][]string) *dto.CommentReplyPageResp {
	pages := 0
	if result.PageSize > 0 {
		pages = int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize))
	}

	items := make([]dto.CommentReplyResp, 0, len(result.Replies))
	for _, aggregate := range result.Replies {
		item := replyToDTO(aggregate, resolver, rolesMap)
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

func userToDTO(user *model.User, resolver storage.ObjectURLResolver, rolesMap map[uint][]string) *dto.CommentUserResp {
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
		Roles:     userrole.ForUser(rolesMap, user.ID),
	}
}

func collectCommentPageUserIDs(result *commentrepo.PageResult) []uint {
	if result == nil {
		return nil
	}
	ids := make([]uint, 0, len(result.Comments))
	for _, aggregate := range result.Comments {
		if aggregate.User != nil {
			ids = append(ids, aggregate.User.ID)
		}
	}
	return ids
}

func collectReplyPageUserIDs(result *commentrepo.ReplyPageResult) []uint {
	if result == nil {
		return nil
	}
	ids := make([]uint, 0, len(result.Replies)*2)
	for _, aggregate := range result.Replies {
		if aggregate.FromUser != nil {
			ids = append(ids, aggregate.FromUser.ID)
		}
		if aggregate.ToUser != nil {
			ids = append(ids, aggregate.ToUser.ID)
		}
	}
	return ids
}

func collectCommentAggregateUserIDs(aggregate *commentrepo.CommentAggregate) []uint {
	if aggregate == nil || aggregate.User == nil {
		return nil
	}
	return []uint{aggregate.User.ID}
}

func collectReplyAggregateUserIDs(aggregate *commentrepo.ReplyAggregate) []uint {
	if aggregate == nil {
		return nil
	}
	ids := make([]uint, 0, 2)
	if aggregate.FromUser != nil {
		ids = append(ids, aggregate.FromUser.ID)
	}
	if aggregate.ToUser != nil {
		ids = append(ids, aggregate.ToUser.ID)
	}
	return ids
}
