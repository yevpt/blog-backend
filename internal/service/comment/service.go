package comment

import (
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
)

func (s *commentService) List(req dto.CommentListReq) (*dto.CommentPageResp, error) {
	target, err := parseTarget(req.TargetType, req.TargetID)
	if err != nil {
		return nil, err
	}

	result, err := s.repo.List(target, normalizeCommentPage(req.Page), normalizeCommentPageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return commentPageToDTO(result, target.Type, s.objectURLResolver), nil
}

func (s *commentService) Create(req dto.CommentCreateReq, userID uint) (*dto.CommentItemResp, error) {
	target, err := parseTarget(req.TargetType, req.TargetID)
	if err != nil {
		return nil, err
	}
	content, err := cleanCommentContent(req.Content)
	if err != nil {
		return nil, err
	}

	aggregate, err := s.repo.Create(target, userID, content)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return commentToDTO(*aggregate, target.Type, s.objectURLResolver), nil
}

func (s *commentService) Reply(commentID uint, req dto.CommentReplyCreateReq, userID uint) (*dto.CommentReplyResp, error) {
	commentType, err := parseTargetType(req.TargetType)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	content, err := cleanCommentContent(req.Content)
	if err != nil {
		return nil, err
	}

	aggregate, err := s.repo.Reply(commentrepo.ReplyData{
		Target:        commentrepo.Target{Type: commentType},
		CommentID:     commentID,
		ParentReplyID: req.ParentReplyID,
		FromUserID:    userID,
		Content:       content,
	})
	if err != nil {
		return nil, mapRepoError(err)
	}
	return replyToDTO(*aggregate, s.objectURLResolver), nil
}

func (s *commentService) DeleteComment(targetType string, commentID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	commentType, err := parseTargetType(targetType)
	if err != nil || commentID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	comment, err := s.repo.DeleteComment(commentrepo.Target{Type: commentType}, commentID, userID, hasAdminRole(roleNames))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.CommentDeleteResp{ID: comment.ID}, nil
}

func (s *commentService) DeleteReply(replyID uint, userID uint, roleNames []string) (*dto.CommentDeleteResp, error) {
	if replyID == 0 {
		return nil, ErrCommentTargetInvalid
	}
	reply, err := s.repo.DeleteReply(replyID, userID, hasAdminRole(roleNames))
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
