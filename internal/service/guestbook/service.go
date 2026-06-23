package guestbook

import (
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	"github.com/vpt/blog-backend/pkg/roles"
)

func (s *guestbookService) List(req dto.GuestbookListReq, viewerID *uint) (*dto.GuestbookPageResp, error) {
	result, err := s.repo.List(normalizeOwnerUserID(req.OwnerUserID), viewerID, normalizePage(req.Page), normalizePageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return guestbookPageToDTO(result, s.objectURLResolver), nil
}

func (s *guestbookService) Create(req dto.GuestbookCreateReq, fromUserID uint) (*dto.GuestbookItemResp, error) {
	content, err := cleanContent(req.Content)
	if err != nil {
		return nil, err
	}

	ownerUserID := normalizeOwnerUserID(req.OwnerUserID)
	aggregate, err := s.repo.Create(ownerUserID, fromUserID, content)
	if err != nil {
		return nil, mapRepoError(err)
	}
	// 留言成功后发布 guestbook_created 事件，接收人为板主。
	s.notifyGuestbookCreated(ownerUserID, fromUserID, aggregate)
	return guestbookItemToDTO(*aggregate, s.objectURLResolver), nil
}

func (s *guestbookService) ToggleLike(id uint, userID uint) (*dto.GuestbookLikeResp, error) {
	if id == 0 {
		return nil, ErrGuestbookInvalid
	}
	result, err := s.repo.ToggleLike(id, userID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	// 仅在本次为点赞时发布事件，接收人由分发器按留言归属解析；点赞仅站内通知。
	// 自己点赞自己的留言不产生通知事件。
	if result.IsLiked && result.OwnerUserID != userID {
		s.notifyGuestbookLiked(id, userID)
	}
	return &dto.GuestbookLikeResp{ID: result.ID, IsLiked: result.IsLiked, LikeCount: result.LikeCount}, nil
}

func (s *guestbookService) Delete(id uint, userID uint, roleNames []string) (*dto.GuestbookDeleteResp, error) {
	if id == 0 {
		return nil, ErrGuestbookInvalid
	}
	message, err := s.repo.Delete(id, userID, roles.HasPermission(roleNames, roles.AdminRole))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.GuestbookDeleteResp{ID: message.ID}, nil
}

func normalizeOwnerUserID(ownerUserID uint) uint {
	if ownerUserID == 0 {
		return defaultOwnerUserID
	}
	return ownerUserID
}

func normalizePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizePageSize(pageSize int) int {
	if pageSize < 1 {
		return 10
	}
	if pageSize > 50 {
		return 50
	}
	return pageSize
}

func cleanContent(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", ErrGuestbookContentRequired
	}
	return trimmed, nil
}

func mapRepoError(err error) error {
	if errors.Is(err, guestbookrepo.ErrOwnerNotFound) {
		return ErrGuestbookOwnerNotFound
	}
	if errors.Is(err, guestbookrepo.ErrGuestbookNotFound) {
		return ErrGuestbookNotFound
	}
	if errors.Is(err, guestbookrepo.ErrNoDeletePermission) {
		return ErrGuestbookNoDeletePermission
	}
	return err
}
