package comment

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"github.com/vpt/blog-backend/internal/service/commentasset"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/internal/service/userrole"
	"github.com/vpt/blog-backend/pkg/storage"
)

func commentPageToDTO(ctx context.Context, result *commentrepo.PageResult, commentType uint8, resolver storage.ObjectURLResolver, rolesMap map[uint][]string, views map[moderationservice.SubjectKey]moderationservice.View) *dto.CommentPageResp {
	pages := 0
	if result.PageSize > 0 {
		pages = int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize))
	}

	items := make([]dto.CommentItemResp, 0, len(result.Comments))
	for _, aggregate := range result.Comments {
		items = append(items, *commentToDTO(ctx, aggregate, commentType, resolver, rolesMap, views))
	}
	return &dto.CommentPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}
}

func adminCommentPageToDTO(ctx context.Context, result *commentrepo.AdminPageResult, resolver storage.ObjectURLResolver, rolesMap map[uint][]string, views map[moderationservice.SubjectKey]moderationservice.View) *dto.AdminCommentPageResp {
	pages := 0
	if result.PageSize > 0 {
		pages = int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize))
	}

	items := make([]dto.CommentItemResp, 0, len(result.Comments))
	for _, aggregate := range result.Comments {
		item := dto.CommentItemResp{
			ID:         aggregate.Comment.ID,
			TargetType: targetTypeName(aggregate.TargetType),
			TargetID:   aggregate.Comment.TargetID,
			UserID:     aggregate.Comment.UserID,
			Content:    commentasset.ResolveContent(ctx, resolver, aggregate.Comment.Content),
			User:       userToDTO(ctx, aggregate.User, resolver, rolesMap),
			ReplyCount: aggregate.ReplyCount,
			LikeCount:  aggregate.LikeCount,
			CreatedAt:  aggregate.Comment.CreatedAt,
			UpdatedAt:  aggregate.Comment.UpdatedAt,
		}
		applyCommentModeration(ctx, &item, commentSubjectType(aggregate.TargetType), aggregate.Comment.ID, resolver, views)
		items = append(items, item)
	}
	return &dto.AdminCommentPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}
}

func commentToDTO(ctx context.Context, aggregate commentrepo.CommentAggregate, commentType uint8, resolver storage.ObjectURLResolver, rolesMap map[uint][]string, views map[moderationservice.SubjectKey]moderationservice.View) *dto.CommentItemResp {
	result := &dto.CommentItemResp{
		ID:         aggregate.Comment.ID,
		TargetType: targetTypeName(commentType),
		TargetID:   aggregate.Comment.TargetID,
		UserID:     aggregate.Comment.UserID,
		Content:    commentasset.ResolveContent(ctx, resolver, aggregate.Comment.Content),
		User:       userToDTO(ctx, aggregate.User, resolver, rolesMap),
		ReplyCount: aggregate.ReplyCount,
		LikeCount:  aggregate.LikeCount,
		IsLiked:    aggregate.IsLiked,
		CreatedAt:  aggregate.Comment.CreatedAt,
		UpdatedAt:  aggregate.Comment.UpdatedAt,
	}
	applyCommentModeration(ctx, result, commentSubjectType(commentType), aggregate.Comment.ID, resolver, views)
	return result
}

func replyToDTO(ctx context.Context, aggregate commentrepo.ReplyAggregate, resolver storage.ObjectURLResolver, rolesMap map[uint][]string, views map[moderationservice.SubjectKey]moderationservice.View) *dto.CommentReplyResp {
	reply := aggregate.Reply
	result := &dto.CommentReplyResp{
		ID:            reply.ID,
		TargetType:    "",
		CommentID:     reply.CommentID,
		FromUserID:    reply.FromUserID,
		ToUserID:      reply.ToUserID,
		ParentReplyID: reply.ParentReplyID,
		Content:       commentasset.ResolveContent(ctx, resolver, reply.Content),
		FromUser:      userToDTO(ctx, aggregate.FromUser, resolver, rolesMap),
		ToUser:        userToDTO(ctx, aggregate.ToUser, resolver, rolesMap),
		LikeCount:     aggregate.LikeCount,
		IsLiked:       aggregate.IsLiked,
		CreatedAt:     reply.CreatedAt,
		UpdatedAt:     reply.UpdatedAt,
	}
	return result
}

func replyPageToDTO(ctx context.Context, result *commentrepo.ReplyPageResult, commentType uint8, resolver storage.ObjectURLResolver, rolesMap map[uint][]string, views map[moderationservice.SubjectKey]moderationservice.View) *dto.CommentReplyPageResp {
	pages := 0
	if result.PageSize > 0 {
		pages = int((result.Total + int64(result.PageSize) - 1) / int64(result.PageSize))
	}

	items := make([]dto.CommentReplyResp, 0, len(result.Replies))
	for _, aggregate := range result.Replies {
		item := replyToDTO(ctx, aggregate, resolver, rolesMap, views)
		item.TargetType = targetTypeName(commentType)
		applyReplyModeration(ctx, item, views, resolver)
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

func applyCommentModeration(ctx context.Context, result *dto.CommentItemResp, subjectType moderationservice.SubjectType, id uint, resolver storage.ObjectURLResolver, views map[moderationservice.SubjectKey]moderationservice.View) {
	view, ok := views[moderationservice.SubjectKey{ContentType: subjectType, ContentID: uint64(id)}]
	if !ok {
		return
	}
	content, projected := moderationservice.ProjectView(view)
	result.Content = commentasset.ResolveContent(ctx, resolver, content)
	result.Moderation = projected
}

func applyReplyModeration(ctx context.Context, result *dto.CommentReplyResp, views map[moderationservice.SubjectKey]moderationservice.View, resolver storage.ObjectURLResolver) {
	subjectType := replySubjectTypeName(result.TargetType)
	view, ok := views[moderationservice.SubjectKey{ContentType: subjectType, ContentID: uint64(result.ID)}]
	if !ok {
		return
	}
	content, projected := moderationservice.ProjectView(view)
	result.Content = commentasset.ResolveContent(ctx, resolver, content)
	result.Moderation = projected
}

func userToDTO(ctx context.Context, user *model.User, resolver storage.ObjectURLResolver, rolesMap map[uint][]string) *dto.CommentUserResp {
	if user == nil {
		return nil
	}
	return &dto.CommentUserResp{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		AvatarUrl: storage.ResolvePtrURLContext(ctx, resolver, user.AvatarUrl),
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

func collectAdminCommentPageUserIDs(result *commentrepo.AdminPageResult) []uint {
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
