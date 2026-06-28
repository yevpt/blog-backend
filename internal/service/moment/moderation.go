package moment

import (
	"context"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
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

func (s *momentService) loadMomentViews(aggregates []momentrepo.MomentAggregate, viewer moderationservice.Viewer) (map[moderationservice.SubjectKey]moderationservice.View, error) {
	if s.moderation == nil {
		return nil, nil
	}
	refs := make([]moderationservice.SubjectRef, 0, len(aggregates))
	for _, aggregate := range aggregates {
		refs = append(refs, moderationservice.SubjectRef{
			Type: moderationservice.SubjectMoment, ID: uint64(aggregate.Moment.ID),
		})
	}
	return s.moderation.LoadViews(context.Background(), refs, viewer)
}
