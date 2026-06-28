package moment

import (
	"github.com/vpt/blog-backend/internal/dto"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
)

const defaultMomentOwnerUserID uint = 1

func (s *momentService) FeedList(req dto.MomentFeedReq, viewerID *uint) (*dto.MomentPageResp, error) {
	result, err := s.repo.ListFeed(momentrepo.FeedFilter{
		Page:        normalizeMomentPage(req.Page),
		PageSize:    normalizeMomentPageSize(req.PageSize),
		Scope:       momentrepo.FeedScope(req.Scope),
		Sort:        momentrepo.FeedSort(req.Sort),
		OwnerUserID: defaultMomentOwnerUserID,
	}, viewerID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return s.momentPageToDTO(result, moderationViewer(viewerID, false))
}
