package comment

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"github.com/vpt/blog-backend/internal/service/commentasset"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/roles"
)

func (s *commentService) submitComment(target commentrepo.Target, targetID, userID uint, content, idempotencyKey string) (*dto.CommentItemResp, error) {
	imageKeys := moderationImageSignal(content)
	result, err := s.moderation.Submit(context.Background(), moderationservice.SubmitCommand{
		ActorID: uint64(userID),
		Subject: moderationservice.SubjectRef{Type: commentSubjectType(target.Type), RootID: uint64(targetID)},
		Content: content, ImageKeys: imageKeys, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	visibleContent, view := moderationservice.ProjectSubmitResult(result)
	return &dto.CommentItemResp{
		ID: uint(result.Subject.ID), TargetType: targetTypeName(target.Type), TargetID: targetID,
		UserID: userID, Content: visibleContent, Moderation: view,
	}, nil
}

func (s *commentService) EditComment(targetType string, commentID uint, req dto.CommentCreateReq, userID uint, roleNames []string) (*dto.CommentItemResp, error) {
	commentType, err := parseTargetType(targetType)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	content, err := cleanCommentContent(req.Content)
	if err != nil {
		return nil, err
	}
	result, err := s.moderation.Edit(context.Background(), moderationservice.EditCommand{
		ActorID: uint64(userID), IsAdmin: roles.HasPermission(roleNames, roles.AdminRole),
		Subject: moderationservice.SubjectRef{Type: commentSubjectType(commentType), ID: uint64(commentID)},
		Content: content, ImageKeys: moderationImageSignal(content), IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	visibleContent, view := moderationservice.ProjectSubmitResult(result)
	return &dto.CommentItemResp{
		ID: commentID, TargetType: targetTypeName(commentType), TargetID: uint(result.Subject.RootID),
		UserID: userID, Content: visibleContent, Moderation: view,
	}, nil
}

func (s *commentService) submitReply(targetType uint8, commentID, userID uint, req dto.CommentReplyCreateReq, content string) (*dto.CommentReplyResp, error) {
	parentID := uint64(req.ParentReplyID)
	imageKeys := moderationImageSignal(content)
	result, err := s.moderation.Submit(context.Background(), moderationservice.SubmitCommand{
		ActorID: uint64(userID),
		Subject: moderationservice.SubjectRef{Type: replySubjectType(targetType), RootID: uint64(commentID), ParentID: &parentID},
		Content: content, ImageKeys: imageKeys, IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	visibleContent, view := moderationservice.ProjectSubmitResult(result)
	return &dto.CommentReplyResp{
		ID: uint(result.Subject.ID), TargetType: targetTypeName(targetType), CommentID: commentID,
		FromUserID: userID, ParentReplyID: req.ParentReplyID, Content: visibleContent, Moderation: view,
	}, nil
}

func (s *commentService) EditReply(targetType string, replyID uint, req dto.CommentReplyCreateReq, userID uint, roleNames []string) (*dto.CommentReplyResp, error) {
	commentType, err := parseTargetType(targetType)
	if err != nil || replyID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	content, err := cleanCommentContent(req.Content)
	if err != nil {
		return nil, err
	}
	result, err := s.moderation.Edit(context.Background(), moderationservice.EditCommand{
		ActorID: uint64(userID), IsAdmin: roles.HasPermission(roleNames, roles.AdminRole),
		Subject: moderationservice.SubjectRef{Type: replySubjectType(commentType), ID: uint64(replyID)},
		Content: content, ImageKeys: moderationImageSignal(content), IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	visibleContent, view := moderationservice.ProjectSubmitResult(result)
	return &dto.CommentReplyResp{
		ID: replyID, TargetType: targetTypeName(commentType), CommentID: uint(result.Subject.RootID),
		FromUserID: userID, ParentReplyID: optionalUint(result.Subject.ParentID),
		Content: visibleContent, Moderation: view,
	}, nil
}

func optionalUint(value *uint64) uint {
	if value == nil {
		return 0
	}
	return uint(*value)
}

func moderationImageSignal(content string) []string {
	if commentasset.ContainsImage(content) {
		return []string{"embedded-image"}
	}
	return nil
}
