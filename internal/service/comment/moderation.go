package comment

import (
	"context"

	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
)

func moderationViewer(viewerID *uint, admin bool) moderationservice.Viewer {
	if admin {
		return moderationservice.Viewer{Role: moderationrepo.ViewerAdmin}
	}
	if viewerID == nil {
		return moderationservice.Viewer{Role: moderationrepo.ViewerPublic}
	}
	return moderationservice.Viewer{Role: moderationrepo.ViewerAuthor, UserID: uint64(*viewerID)}
}

func commentSubjectType(targetType uint8) moderationservice.SubjectType {
	if targetType == commentrepo.TargetMoment {
		return moderationservice.SubjectMomentComment
	}
	if targetType == commentrepo.TargetGuestbook {
		return moderationservice.SubjectGuestbook
	}
	return moderationservice.SubjectArticleComment
}

func replySubjectType(targetType uint8) moderationservice.SubjectType {
	if targetType == commentrepo.TargetMoment {
		return moderationservice.SubjectMomentCommentReply
	}
	if targetType == commentrepo.TargetGuestbook {
		return moderationservice.SubjectGuestbookReply
	}
	return moderationservice.SubjectArticleCommentReply
}

func replySubjectTypeName(targetType string) moderationservice.SubjectType {
	switch targetType {
	case "moment":
		return moderationservice.SubjectMomentCommentReply
	case "guestbook":
		return moderationservice.SubjectGuestbookReply
	default:
		return moderationservice.SubjectArticleCommentReply
	}
}

func (s *commentService) loadCommentViews(ctx context.Context, result *commentrepo.PageResult, targetType uint8, targetID uint, viewer moderationservice.Viewer) (map[moderationservice.SubjectKey]moderationservice.View, error) {
	if s.moderation == nil || result == nil {
		return nil, nil
	}
	refs := make([]moderationservice.SubjectRef, 0, len(result.Comments))
	for _, aggregate := range result.Comments {
		refs = append(refs, moderationservice.SubjectRef{
			Type: commentSubjectType(targetType), ID: uint64(aggregate.Comment.ID), RootID: uint64(targetID),
		})
	}
	return s.moderation.LoadViews(ctx, refs, viewer)
}

func (s *commentService) loadAdminCommentViews(ctx context.Context, result *commentrepo.AdminPageResult) (map[moderationservice.SubjectKey]moderationservice.View, error) {
	if s.moderation == nil || result == nil {
		return nil, nil
	}
	refs := make([]moderationservice.SubjectRef, 0, len(result.Comments))
	for _, aggregate := range result.Comments {
		refs = append(refs, moderationservice.SubjectRef{
			Type: commentSubjectType(aggregate.TargetType), ID: uint64(aggregate.Comment.ID),
			RootID: uint64(aggregate.Comment.TargetID),
		})
	}
	return s.moderation.LoadViews(ctx, refs, moderationViewer(nil, true))
}

func (s *commentService) loadReplyViews(ctx context.Context, result *commentrepo.ReplyPageResult, targetType uint8, commentID uint, viewer moderationservice.Viewer) (map[moderationservice.SubjectKey]moderationservice.View, error) {
	if s.moderation == nil || result == nil {
		return nil, nil
	}
	refs := make([]moderationservice.SubjectRef, 0, len(result.Replies))
	for _, aggregate := range result.Replies {
		parentID := uint64(aggregate.Reply.ParentReplyID)
		refs = append(refs, moderationservice.SubjectRef{
			Type: replySubjectType(targetType), ID: uint64(aggregate.Reply.ID),
			RootID: uint64(commentID), ParentID: &parentID,
		})
	}
	return s.moderation.LoadViews(ctx, refs, viewer)
}
