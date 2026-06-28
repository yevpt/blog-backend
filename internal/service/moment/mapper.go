package moment

import (
	"math"
	"path"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/internal/service/userrole"
	"github.com/vpt/blog-backend/pkg/storage"
)

func (s *momentService) momentPageToDTO(result *momentrepo.PageResult, viewer moderationservice.Viewer) (*dto.MomentPageResp, error) {
	rolesMap, err := s.lookupRoles(collectMomentPageUserIDs(result))
	if err != nil {
		return nil, err
	}

	views, err := s.loadMomentViews(result.Moments, viewer)
	if err != nil {
		return nil, err
	}

	items := make([]dto.MomentItemResp, 0, len(result.Moments))
	for _, aggregate := range result.Moments {
		item, err := s.momentToDTO(aggregate, rolesMap, views)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}

	pages := 0
	if result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}
	return &dto.MomentPageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}, nil
}

func (s *momentService) momentToDTO(aggregate momentrepo.MomentAggregate, rolesMap map[uint][]string, views map[moderationservice.SubjectKey]moderationservice.View) (*dto.MomentItemResp, error) {
	if rolesMap == nil {
		var err error
		rolesMap, err = s.lookupRoles(collectMomentAggregateUserIDs(aggregate))
		if err != nil {
			return nil, err
		}
	}

	moment := aggregate.Moment
	user := momentUserToDTO(aggregate.User, s.objectURLResolver, rolesMap)
	resp := &dto.MomentItemResp{
		ID:            moment.ID,
		UserID:        moment.UserID,
		Content:       moment.Content,
		Status:        moment.Status,
		CommentStatus: moment.CommentStatus,
		ReadCount:     moment.ReadCount,
		IsTop:         moment.IsTop,
		LikeCount:     aggregate.LikeCount,
		CommentCount:  aggregate.CommentCount,
		IsLiked:       aggregate.IsLiked,
		User:          user,
		Images:        s.mediaToDTO(aggregate.Images),
		CreatedAt:     moment.CreatedAt,
		UpdatedAt:     moment.UpdatedAt,
	}
	view, ok := views[moderationservice.SubjectKey{ContentType: moderationservice.SubjectMoment, ContentID: uint64(moment.ID)}]
	if ok {
		content, projected := moderationservice.ProjectView(view)
		resp.Content = content
		resp.Moderation = projected
		resp.Images = s.moderationImagesToDTO(view.VisibleImages)
	}
	return resp, nil
}

func (s *momentService) moderationImagesToDTO(images []moderationrepo.ImageView) []dto.MomentMediaResp {
	result := make([]dto.MomentMediaResp, 0, len(images))
	for _, image := range images {
		if image.DisplayObjectKey == "" {
			continue
		}
		name := path.Base(image.DisplayObjectKey)
		result = append(result, dto.MomentMediaResp{
			Name: name, FileType: strings.TrimPrefix(strings.ToLower(path.Ext(name)), "."),
			URL: image.DisplayObjectKey, AccessURL: s.resolveImageURL(image.DisplayObjectKey), Seq: image.Seq,
		})
	}
	return result
}

func (s *momentService) mediaToDTO(images []model.Media) []dto.MomentMediaResp {
	rows := make([]dto.MomentMediaResp, 0, len(images))
	for _, image := range images {
		rows = append(rows, dto.MomentMediaResp{
			ID:        image.ID,
			Name:      image.Name,
			FileType:  image.FileType,
			URL:       image.URL,
			AccessURL: s.resolveImageURL(image.URL),
			Size:      image.Size,
			Seq:       image.Seq,
		})
	}
	return rows
}

func momentUserToDTO(user *model.User, resolver storage.ObjectURLResolver, rolesMap map[uint][]string) *dto.MomentUserResp {
	if user == nil {
		return nil
	}
	return &dto.MomentUserResp{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		AvatarUrl: storage.ResolvePtrURL(resolver, user.AvatarUrl),
		Site:      user.Site,
		Mark:      user.Mark,
		Roles:     userrole.ForUser(rolesMap, user.ID),
	}
}

func (s *momentService) resolveImageURL(url string) string {
	return storage.ResolveURL(s.objectURLResolver, url)
}

func collectMomentPageUserIDs(result *momentrepo.PageResult) []uint {
	if result == nil {
		return nil
	}
	ids := make([]uint, 0, len(result.Moments))
	for _, aggregate := range result.Moments {
		if aggregate.User != nil {
			ids = append(ids, aggregate.User.ID)
		}
	}
	return ids
}

func collectMomentAggregateUserIDs(aggregate momentrepo.MomentAggregate) []uint {
	if aggregate.User == nil {
		return nil
	}
	return []uint{aggregate.User.ID}
}
