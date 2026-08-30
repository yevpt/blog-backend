package comment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/dto"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"github.com/vpt/blog-backend/internal/service/commentasset"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/internal/service/userrole"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/storage"
)

func (s *commentService) List(ctx context.Context, targetType string, targetID uint, req dto.CommentListReq, viewerID *uint) (*dto.CommentPageResp, error) {
	target, err := parseTarget(targetType, targetID)
	if err != nil {
		return nil, err
	}

	result, err := s.repo.WithContext(ctx).List(target, viewerID, normalizeCommentPage(req.Page), normalizeCommentPageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	rolesMap, err := s.lookupRoles(ctx, collectCommentPageUserIDs(result))
	if err != nil {
		return nil, err
	}
	views, err := s.loadCommentViews(ctx, result, target.Type, target.ID, moderationViewer(viewerID, false))
	if err != nil {
		return nil, err
	}
	return commentPageToDTO(ctx, result, target.Type, s.objectURLResolver, rolesMap, views), nil
}

func (s *commentService) ListAdmin(ctx context.Context, req dto.AdminCommentListReq) (*dto.AdminCommentPageResp, error) {
	targetType, err := parseAdminTargetType(req.TargetType)
	if err != nil {
		return nil, err
	}

	result, err := s.repo.WithContext(ctx).ListAdmin(targetType, strings.TrimSpace(req.Search), normalizeCommentPage(req.Page), normalizeCommentPageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	rolesMap, err := s.lookupRoles(ctx, collectAdminCommentPageUserIDs(result))
	if err != nil {
		return nil, err
	}
	views, err := s.loadAdminCommentViews(ctx, result)
	if err != nil {
		return nil, err
	}
	return adminCommentPageToDTO(ctx, result, s.objectURLResolver, rolesMap, views), nil
}

func (s *commentService) Create(ctx context.Context, targetType string, targetID uint, req dto.CommentCreateReq, userID uint) (*dto.CommentItemResp, error) {
	target, err := parseTarget(targetType, targetID)
	if err != nil {
		return nil, err
	}
	content, err := cleanCommentContent(req.Content)
	if err != nil {
		return nil, err
	}
	if s.moderation != nil {
		if target.Type == commentrepo.TargetMoment {
			if err := s.moderation.AssertCanInteract(ctx, moderationservice.SubjectRef{
				Type: moderationservice.SubjectMoment, ID: uint64(targetID),
			}); err != nil {
				return nil, err
			}
		}
		return s.submitComment(ctx, target, targetID, userID, content, req.IdempotencyKey)
	}
	normalized, store, err := s.normalizeCommentImages(ctx, content, userID, commentImageTargetPrefix(targetType, targetID))
	if err != nil {
		return nil, err
	}
	content = normalized.Content

	aggregate, err := s.repo.WithContext(ctx).Create(target, userID, content)
	if err != nil {
		cleanupCtx, cancelCleanup := commentPostCommitContext(ctx)
		defer cancelCleanup()
		_ = commentasset.DeleteKeys(cleanupCtx, store, normalized.CopiedKeys)
		return nil, mapRepoError(err)
	}
	postCommitCtx, cancelPostCommit := commentPostCommitContext(ctx)
	defer cancelPostCommit()
	if err := commentasset.DeleteKeys(postCommitCtx, store, normalized.TempKeys); err != nil {
		return nil, err
	}
	// 评论落库成功后发布通知事件，失败不影响评论本身。
	s.notifyCommentCreated(postCommitCtx, targetType, targetID, aggregate)
	rolesMap, err := s.lookupRoles(ctx, collectCommentAggregateUserIDs(aggregate))
	if err != nil {
		return nil, err
	}
	return commentToDTO(ctx, *aggregate, target.Type, s.objectURLResolver, rolesMap, nil), nil
}

func (s *commentService) ListReplies(ctx context.Context, targetType string, commentID uint, req dto.CommentReplyListReq, viewerID *uint) (*dto.CommentReplyPageResp, error) {
	target, err := parseTarget(targetType, 1)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}

	result, err := s.repo.WithContext(ctx).ListReplies(commentrepo.Target{Type: target.Type}, commentID, viewerID, normalizeCommentPage(req.Page), normalizeCommentPageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	rolesMap, err := s.lookupRoles(ctx, collectReplyPageUserIDs(result))
	if err != nil {
		return nil, err
	}
	views, err := s.loadReplyViews(ctx, result, target.Type, commentID, moderationViewer(viewerID, false))
	if err != nil {
		return nil, err
	}
	return replyPageToDTO(ctx, result, target.Type, s.objectURLResolver, rolesMap, views), nil
}

func (s *commentService) Reply(ctx context.Context, targetType string, commentID uint, req dto.CommentReplyCreateReq, userID uint) (*dto.CommentReplyResp, error) {
	commentType, err := parseTargetType(targetType)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	content, err := cleanCommentContent(req.Content)
	if err != nil {
		return nil, err
	}
	if s.moderation != nil {
		parentRef := moderationservice.SubjectRef{Type: commentSubjectType(commentType), ID: uint64(commentID)}
		if req.ParentReplyID > 0 {
			parentRef = moderationservice.SubjectRef{Type: replySubjectType(commentType), ID: uint64(req.ParentReplyID)}
		}
		if err := s.moderation.AssertCanInteract(ctx, parentRef); err != nil {
			return nil, err
		}
		return s.submitReply(ctx, commentType, commentID, userID, req, content)
	}
	normalized, store, err := s.normalizeCommentImages(ctx, content, userID, replyImageTargetPrefix(targetType, commentID))
	if err != nil {
		return nil, err
	}
	content = normalized.Content

	aggregate, err := s.repo.WithContext(ctx).Reply(commentrepo.ReplyData{
		Target:        commentrepo.Target{Type: commentType},
		CommentID:     commentID,
		ParentReplyID: req.ParentReplyID,
		FromUserID:    userID,
		Content:       content,
	})
	if err != nil {
		cleanupCtx, cancelCleanup := commentPostCommitContext(ctx)
		defer cancelCleanup()
		_ = commentasset.DeleteKeys(cleanupCtx, store, normalized.CopiedKeys)
		return nil, mapRepoError(err)
	}
	postCommitCtx, cancelPostCommit := commentPostCommitContext(ctx)
	defer cancelPostCommit()
	if err := commentasset.DeleteKeys(postCommitCtx, store, normalized.TempKeys); err != nil {
		return nil, err
	}
	// 回复落库成功后发布通知事件，接收人为被回复人。
	s.notifyReplyCreated(postCommitCtx, targetType, aggregate)
	rolesMap, err := s.lookupRoles(ctx, collectReplyAggregateUserIDs(aggregate))
	if err != nil {
		return nil, err
	}
	return replyToDTO(ctx, *aggregate, s.objectURLResolver, rolesMap, nil), nil
}

func (s *commentService) normalizeCommentImages(ctx context.Context, content string, userID uint, targetPrefix string) (*commentasset.NormalizeResult, storage.ObjectStore, error) {
	store, _ := s.objectURLResolver.(storage.ObjectStore)
	result, err := commentasset.Normalize(ctx, store, commentasset.NormalizeInput{
		UserID:       userID,
		Content:      content,
		TargetPrefix: targetPrefix,
	})
	if err != nil {
		return nil, store, mapCommentAssetError(err)
	}
	return result, store, nil
}

func commentImageTargetPrefix(targetType string, targetID uint) string {
	return fmt.Sprintf("comments/%s/%d/images", targetType, targetID)
}

func replyImageTargetPrefix(targetType string, commentID uint) string {
	return fmt.Sprintf("comments/%s/replies/%d/images", targetType, commentID)
}

func (s *commentService) ToggleLike(ctx context.Context, targetType string, commentID uint, userID uint) (*dto.CommentLikeResp, error) {
	target, err := parseTarget(targetType, 1)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	if s.moderation != nil {
		if err := s.moderation.AssertCanInteract(ctx, moderationservice.SubjectRef{
			Type: commentSubjectType(target.Type), ID: uint64(commentID),
		}); err != nil {
			return nil, err
		}
	}
	result, err := s.repo.WithContext(ctx).ToggleLike(commentrepo.Target{Type: target.Type}, commentID, userID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	// 仅在本次为点赞时发布通知事件，接收人为评论作者；点赞仅站内通知。
	if result.IsLiked {
		postCommitCtx, cancelPostCommit := commentPostCommitContext(ctx)
		defer cancelPostCommit()
		s.notifyCommentLiked(postCommitCtx, targetType, commentID, result, userID)
	}
	return &dto.CommentLikeResp{IsLiked: result.IsLiked, LikeCount: result.LikeCount}, nil
}

func (s *commentService) ToggleReplyLike(ctx context.Context, targetType string, replyID uint, userID uint) (*dto.CommentLikeResp, error) {
	target, err := parseTarget(targetType, 1)
	if err != nil || replyID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	if s.moderation != nil {
		if err := s.moderation.AssertCanInteract(ctx, moderationservice.SubjectRef{
			Type: replySubjectType(target.Type), ID: uint64(replyID),
		}); err != nil {
			return nil, err
		}
	}
	result, err := s.repo.WithContext(ctx).ToggleReplyLike(commentrepo.Target{Type: target.Type}, replyID, userID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if result.IsLiked {
		postCommitCtx, cancelPostCommit := commentPostCommitContext(ctx)
		defer cancelPostCommit()
		s.notifyReplyLiked(postCommitCtx, targetType, replyID, result, userID)
	}
	return &dto.CommentLikeResp{IsLiked: result.IsLiked, LikeCount: result.LikeCount}, nil
}

func (s *commentService) DeleteComment(ctx context.Context, targetType string, commentID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	commentType, err := parseTargetType(targetType)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	if s.moderation != nil {
		err := s.moderation.Delete(ctx, moderationservice.DeleteCommand{
			ActorID: uint64(userID), IsAdmin: roles.HasPermission(roleNames, roles.AdminRole),
			Subject: moderationservice.SubjectRef{Type: commentSubjectType(commentType), ID: uint64(commentID)},
		})
		if err != nil {
			return nil, err
		}
		return &dto.CommentDeleteResp{ID: commentID}, nil
	}
	comment, err := s.repo.WithContext(ctx).DeleteComment(commentrepo.Target{Type: commentType}, commentID, userID, roles.HasPermission(roleNames, roles.AdminRole))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.CommentDeleteResp{ID: comment.ID}, nil
}

func (s *commentService) DeleteReply(ctx context.Context, targetType string, replyID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	commentType, err := parseTargetType(targetType)
	if err != nil || replyID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	if s.moderation != nil {
		err := s.moderation.Delete(ctx, moderationservice.DeleteCommand{
			ActorID: uint64(userID), IsAdmin: roles.HasPermission(roleNames, roles.AdminRole),
			Subject: moderationservice.SubjectRef{Type: replySubjectType(commentType), ID: uint64(replyID)},
		})
		if err != nil {
			return nil, err
		}
		return &dto.CommentDeleteResp{ID: replyID}, nil
	}
	reply, err := s.repo.WithContext(ctx).DeleteReply(commentrepo.Target{Type: commentType}, replyID, userID, roles.HasPermission(roleNames, roles.AdminRole))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.CommentDeleteResp{ID: reply.ID}, nil
}

func cleanCommentContent(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", ErrCommentContentRequired
	}
	return trimmed, nil
}

func normalizeCommentPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizeCommentPageSize(pageSize int) int {
	if pageSize < 1 {
		return 10
	}
	if pageSize > 50 {
		return 50
	}
	return pageSize
}

func mapRepoError(err error) error {
	if errors.Is(err, commentrepo.ErrTargetNotFound) {
		return ErrCommentTargetNotFound
	}
	if errors.Is(err, commentrepo.ErrTargetCommentClosed) {
		return ErrCommentClosed
	}
	if errors.Is(err, commentrepo.ErrCommentNotFound) {
		return ErrCommentNotFound
	}
	if errors.Is(err, commentrepo.ErrReplyNotFound) {
		return ErrCommentReplyNotFound
	}
	if errors.Is(err, commentrepo.ErrNoDeletePermission) {
		return ErrCommentNoDeletePermission
	}
	return err
}

func mapCommentAssetError(err error) error {
	if errors.Is(err, commentasset.ErrImageInvalid) ||
		errors.Is(err, commentasset.ErrImageExternal) ||
		errors.Is(err, commentasset.ErrImageNotFound) {
		return fmt.Errorf("%w：%s", ErrCommentImageInvalid, err.Error())
	}
	return err
}

func (s *commentService) lookupRoles(ctx context.Context, userIDs []uint) (map[uint][]string, error) {
	if s.userRepo == nil {
		return map[uint][]string{}, nil
	}
	return userrole.LookupByUserIDsContext(ctx, s.userRepo, userIDs)
}

func commentPostCommitContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}
