package comment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"github.com/vpt/blog-backend/internal/service/commentasset"
	"github.com/vpt/blog-backend/internal/service/userrole"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/storage"
)

func (s *commentService) List(targetType string, targetID uint, req dto.CommentListReq, viewerID *uint) (*dto.CommentPageResp, error) {
	target, err := parseTarget(targetType, targetID)
	if err != nil {
		return nil, err
	}

	result, err := s.repo.List(target, viewerID, normalizeCommentPage(req.Page), normalizeCommentPageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	rolesMap, err := s.lookupRoles(collectCommentPageUserIDs(result))
	if err != nil {
		return nil, err
	}
	views, err := s.loadCommentViews(result, target.Type, target.ID, moderationViewer(viewerID, false))
	if err != nil {
		return nil, err
	}
	return commentPageToDTO(result, target.Type, s.objectURLResolver, rolesMap, views), nil
}

func (s *commentService) ListAdmin(req dto.AdminCommentListReq) (*dto.AdminCommentPageResp, error) {
	targetType, err := parseAdminTargetType(req.TargetType)
	if err != nil {
		return nil, err
	}

	result, err := s.repo.ListAdmin(targetType, strings.TrimSpace(req.Search), normalizeCommentPage(req.Page), normalizeCommentPageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	rolesMap, err := s.lookupRoles(collectAdminCommentPageUserIDs(result))
	if err != nil {
		return nil, err
	}
	views, err := s.loadAdminCommentViews(result)
	if err != nil {
		return nil, err
	}
	return adminCommentPageToDTO(result, s.objectURLResolver, rolesMap, views), nil
}

func (s *commentService) Create(targetType string, targetID uint, req dto.CommentCreateReq, userID uint) (*dto.CommentItemResp, error) {
	target, err := parseTarget(targetType, targetID)
	if err != nil {
		return nil, err
	}
	content, err := cleanCommentContent(req.Content)
	if err != nil {
		return nil, err
	}
	normalized, store, err := s.normalizeCommentImages(content, userID, commentImageTargetPrefix(targetType, targetID))
	if err != nil {
		return nil, err
	}
	content = normalized.Content

	aggregate, err := s.repo.Create(target, userID, content)
	if err != nil {
		_ = commentasset.DeleteKeys(context.Background(), store, normalized.CopiedKeys)
		return nil, mapRepoError(err)
	}
	if err := commentasset.DeleteKeys(context.Background(), store, normalized.TempKeys); err != nil {
		return nil, err
	}
	// 评论落库成功后发布通知事件，失败不影响评论本身。
	s.notifyCommentCreated(targetType, targetID, aggregate)
	rolesMap, err := s.lookupRoles(collectCommentAggregateUserIDs(aggregate))
	if err != nil {
		return nil, err
	}
	return commentToDTO(*aggregate, target.Type, s.objectURLResolver, rolesMap, nil), nil
}

func (s *commentService) ListReplies(targetType string, commentID uint, req dto.CommentReplyListReq, viewerID *uint) (*dto.CommentReplyPageResp, error) {
	target, err := parseTarget(targetType, 1)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}

	result, err := s.repo.ListReplies(commentrepo.Target{Type: target.Type}, commentID, viewerID, normalizeCommentPage(req.Page), normalizeCommentPageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	rolesMap, err := s.lookupRoles(collectReplyPageUserIDs(result))
	if err != nil {
		return nil, err
	}
	views, err := s.loadReplyViews(result, target.Type, commentID, moderationViewer(viewerID, false))
	if err != nil {
		return nil, err
	}
	return replyPageToDTO(result, target.Type, s.objectURLResolver, rolesMap, views), nil
}

func (s *commentService) Reply(targetType string, commentID uint, req dto.CommentReplyCreateReq, userID uint) (*dto.CommentReplyResp, error) {
	commentType, err := parseTargetType(targetType)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	content, err := cleanCommentContent(req.Content)
	if err != nil {
		return nil, err
	}
	normalized, store, err := s.normalizeCommentImages(content, userID, replyImageTargetPrefix(targetType, commentID))
	if err != nil {
		return nil, err
	}
	content = normalized.Content

	aggregate, err := s.repo.Reply(commentrepo.ReplyData{
		Target:        commentrepo.Target{Type: commentType},
		CommentID:     commentID,
		ParentReplyID: req.ParentReplyID,
		FromUserID:    userID,
		Content:       content,
	})
	if err != nil {
		_ = commentasset.DeleteKeys(context.Background(), store, normalized.CopiedKeys)
		return nil, mapRepoError(err)
	}
	if err := commentasset.DeleteKeys(context.Background(), store, normalized.TempKeys); err != nil {
		return nil, err
	}
	// 回复落库成功后发布通知事件，接收人为被回复人。
	s.notifyReplyCreated(targetType, aggregate)
	rolesMap, err := s.lookupRoles(collectReplyAggregateUserIDs(aggregate))
	if err != nil {
		return nil, err
	}
	return replyToDTO(*aggregate, s.objectURLResolver, rolesMap, nil), nil
}

func (s *commentService) normalizeCommentImages(content string, userID uint, targetPrefix string) (*commentasset.NormalizeResult, storage.ObjectStore, error) {
	store, _ := s.objectURLResolver.(storage.ObjectStore)
	result, err := commentasset.Normalize(context.Background(), store, commentasset.NormalizeInput{
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

func (s *commentService) ToggleLike(targetType string, commentID uint, userID uint) (*dto.CommentLikeResp, error) {
	target, err := parseTarget(targetType, 1)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	result, err := s.repo.ToggleLike(commentrepo.Target{Type: target.Type}, commentID, userID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	// 仅在本次为点赞时发布通知事件，接收人为评论作者；点赞仅站内通知。
	if result.IsLiked {
		s.notifyCommentLiked(targetType, commentID, result, userID)
	}
	return &dto.CommentLikeResp{IsLiked: result.IsLiked, LikeCount: result.LikeCount}, nil
}

func (s *commentService) ToggleReplyLike(targetType string, replyID uint, userID uint) (*dto.CommentLikeResp, error) {
	target, err := parseTarget(targetType, 1)
	if err != nil || replyID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	result, err := s.repo.ToggleReplyLike(commentrepo.Target{Type: target.Type}, replyID, userID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if result.IsLiked {
		s.notifyReplyLiked(targetType, replyID, result, userID)
	}
	return &dto.CommentLikeResp{IsLiked: result.IsLiked, LikeCount: result.LikeCount}, nil
}

func (s *commentService) DeleteComment(targetType string, commentID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	commentType, err := parseTargetType(targetType)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	comment, err := s.repo.DeleteComment(commentrepo.Target{Type: commentType}, commentID, userID, roles.HasPermission(roleNames, roles.AdminRole))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.CommentDeleteResp{ID: comment.ID}, nil
}

func (s *commentService) DeleteReply(targetType string, replyID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	commentType, err := parseTargetType(targetType)
	if err != nil || replyID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	reply, err := s.repo.DeleteReply(commentrepo.Target{Type: commentType}, replyID, userID, roles.HasPermission(roleNames, roles.AdminRole))
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

func (s *commentService) lookupRoles(userIDs []uint) (map[uint][]string, error) {
	if s.userRepo == nil {
		return map[uint][]string{}, nil
	}
	return userrole.LookupByUserIDs(s.userRepo, userIDs)
}
