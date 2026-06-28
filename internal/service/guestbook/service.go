package guestbook

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	"github.com/vpt/blog-backend/internal/service/commentasset"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/storage"
)

func (s *guestbookService) List(req dto.GuestbookListReq, viewerID *uint) (*dto.GuestbookPageResp, error) {
	result, err := s.repo.List(normalizeOwnerUserID(req.OwnerUserID), viewerID, normalizePage(req.Page), normalizePageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	rolesMap, err := s.lookupRoles(collectGuestbookPageUserIDs(result))
	if err != nil {
		return nil, err
	}
	views, err := s.loadGuestbookViews(result, moderationViewer(viewerID, false))
	if err != nil {
		return nil, err
	}
	return guestbookPageToDTO(result, s.objectURLResolver, rolesMap, views), nil
}

func (s *guestbookService) ListAdmin(req dto.AdminGuestbookListReq) (*dto.AdminGuestbookPageResp, error) {
	result, err := s.repo.ListAdmin(strings.TrimSpace(req.Search), normalizePage(req.Page), normalizePageSize(req.PageSize))
	if err != nil {
		return nil, mapRepoError(err)
	}
	rolesMap, err := s.lookupRoles(collectGuestbookPageUserIDs(result))
	if err != nil {
		return nil, err
	}
	views, err := s.loadGuestbookViews(result, moderationViewer(nil, true))
	if err != nil {
		return nil, err
	}
	return adminGuestbookPageToDTO(result, s.objectURLResolver, rolesMap, views), nil
}

func (s *guestbookService) Create(req dto.GuestbookCreateReq, fromUserID uint) (*dto.GuestbookItemResp, error) {
	content, err := cleanContent(req.Content)
	if err != nil {
		return nil, err
	}

	ownerUserID := normalizeOwnerUserID(req.OwnerUserID)
	normalized, store, err := s.normalizeGuestbookImages(content, fromUserID, guestbookImageTargetPrefix(ownerUserID))
	if err != nil {
		return nil, err
	}
	content = normalized.Content
	aggregate, err := s.repo.Create(ownerUserID, fromUserID, content)
	if err != nil {
		_ = commentasset.DeleteKeys(context.Background(), store, normalized.CopiedKeys)
		return nil, mapRepoError(err)
	}
	if err := commentasset.DeleteKeys(context.Background(), store, normalized.TempKeys); err != nil {
		return nil, err
	}
	// 留言成功后发布 guestbook_created 事件，接收人为板主。
	s.notifyGuestbookCreated(ownerUserID, fromUserID, aggregate)
	rolesMap, err := s.lookupRoles(collectGuestbookAggregateUserIDs(aggregate))
	if err != nil {
		return nil, err
	}
	return guestbookItemToDTO(*aggregate, s.objectURLResolver, rolesMap, nil), nil
}

func (s *guestbookService) normalizeGuestbookImages(content string, userID uint, targetPrefix string) (*commentasset.NormalizeResult, storage.ObjectStore, error) {
	store, _ := s.objectURLResolver.(storage.ObjectStore)
	result, err := commentasset.Normalize(context.Background(), store, commentasset.NormalizeInput{
		UserID:       userID,
		Content:      content,
		TargetPrefix: targetPrefix,
	})
	if err != nil {
		return nil, store, mapGuestbookAssetError(err)
	}
	return result, store, nil
}

func guestbookImageTargetPrefix(ownerUserID uint) string {
	return fmt.Sprintf("comments/guestbook/%d/images", ownerUserID)
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
		s.notifyGuestbookLiked(id, userID, result.Content)
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

func mapGuestbookAssetError(err error) error {
	if errors.Is(err, commentasset.ErrImageInvalid) ||
		errors.Is(err, commentasset.ErrImageExternal) ||
		errors.Is(err, commentasset.ErrImageNotFound) {
		return fmt.Errorf("%w：%s", ErrGuestbookImageInvalid, err.Error())
	}
	return err
}
