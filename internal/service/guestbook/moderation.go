package guestbook

import (
	"context"

	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
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

func (s *guestbookService) loadGuestbookViews(result *guestbookrepo.PageResult, viewer moderationservice.Viewer) (map[moderationservice.SubjectKey]moderationservice.View, error) {
	if s.moderation == nil || result == nil {
		return nil, nil
	}
	refs := make([]moderationservice.SubjectRef, 0, len(result.Messages))
	for _, aggregate := range result.Messages {
		refs = append(refs, moderationservice.SubjectRef{
			Type: moderationservice.SubjectGuestbook, ID: uint64(aggregate.Message.ID),
			RootID: uint64(aggregate.Message.OwnerUserID),
		})
	}
	return s.moderation.LoadViews(context.Background(), refs, viewer)
}
