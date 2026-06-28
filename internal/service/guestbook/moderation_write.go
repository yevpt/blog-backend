package guestbook

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/service/commentasset"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/roles"
)

func (s *guestbookService) submit(ownerUserID, fromUserID uint, content, idempotencyKey string) (*dto.GuestbookItemResp, error) {
	var imageKeys []string
	if commentasset.ContainsImage(content) {
		imageKeys = []string{"embedded-image"}
	}
	result, err := s.moderation.Submit(context.Background(), moderationservice.SubmitCommand{
		ActorID: uint64(fromUserID),
		Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectGuestbook, RootID: uint64(ownerUserID)},
		Content: content, ImageKeys: imageKeys, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	visibleContent, view := moderationservice.ProjectSubmitResult(result)
	return &dto.GuestbookItemResp{
		ID: uint(result.Subject.ID), OwnerUserID: ownerUserID, FromUserID: fromUserID,
		Content: visibleContent, Moderation: view,
	}, nil
}

func (s *guestbookService) Edit(id uint, req dto.GuestbookCreateReq, userID uint, roleNames []string) (*dto.GuestbookItemResp, error) {
	if id == 0 {
		return nil, ErrGuestbookInvalid
	}
	content, err := cleanContent(req.Content)
	if err != nil {
		return nil, err
	}
	var imageKeys []string
	if commentasset.ContainsImage(content) {
		imageKeys = []string{"embedded-image"}
	}
	result, err := s.moderation.Edit(context.Background(), moderationservice.EditCommand{
		ActorID: uint64(userID), IsAdmin: roles.HasPermission(roleNames, roles.AdminRole),
		Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectGuestbook, ID: uint64(id)},
		Content: content, ImageKeys: imageKeys, IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	visibleContent, view := moderationservice.ProjectSubmitResult(result)
	return &dto.GuestbookItemResp{
		ID: id, OwnerUserID: uint(result.Subject.RootID), FromUserID: userID,
		Content: visibleContent, Moderation: view,
	}, nil
}
