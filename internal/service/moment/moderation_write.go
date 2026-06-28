package moment

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
)

func (s *momentService) submit(req dto.MomentSaveReq, actorID uint, isAdmin bool, content string) (*dto.MomentItemResp, error) {
	imageKeys := momentImageSignals(req)
	result, err := s.moderation.Submit(context.Background(), moderationservice.SubmitCommand{
		ActorID: uint64(actorID), IsAdmin: isAdmin,
		Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectMoment},
		Content: content, ImageKeys: imageKeys, IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	visibleContent, view := moderationservice.ProjectSubmitResult(result)
	return &dto.MomentItemResp{
		ID: uint(result.Subject.ID), UserID: actorID, Content: visibleContent,
		Status: req.Status, CommentStatus: req.CommentStatus, Images: []dto.MomentMediaResp{}, Moderation: view,
	}, nil
}

func (s *momentService) edit(req dto.MomentSaveReq, actorID uint, isAdmin bool, content string) (*dto.MomentItemResp, error) {
	result, err := s.moderation.Edit(context.Background(), moderationservice.EditCommand{
		ActorID: uint64(actorID), IsAdmin: isAdmin,
		Subject: moderationservice.SubjectRef{Type: moderationservice.SubjectMoment, ID: uint64(*req.ID)},
		Content: content, ImageKeys: momentImageSignals(req), IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	visibleContent, view := moderationservice.ProjectSubmitResult(result)
	return &dto.MomentItemResp{
		ID: *req.ID, UserID: actorID, Content: visibleContent,
		Status: req.Status, CommentStatus: req.CommentStatus, Images: []dto.MomentMediaResp{}, Moderation: view,
	}, nil
}

func momentImageSignals(req dto.MomentSaveReq) []string {
	if len(req.ImageURLs) > 0 || len(req.ImageFiles) > 0 || len(req.ImageOrder) > 0 {
		return []string{"moment-image"}
	}
	return nil
}
