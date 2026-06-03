package moment

import (
	"context"
	"math"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
	"github.com/vpt/blog-backend/pkg/storage"
)

func (s *momentService) momentPageToDTO(result *momentrepo.PageResult) (*dto.MomentPageResp, error) {
	items := make([]dto.MomentItemResp, 0, len(result.Moments))
	for _, aggregate := range result.Moments {
		item, err := s.momentToDTO(aggregate)
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

func (s *momentService) momentToDTO(aggregate momentrepo.MomentAggregate) (*dto.MomentItemResp, error) {
	moment := aggregate.Moment
	user := momentUserToDTO(aggregate.User, s.objectURLResolver)
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
	return resp, nil
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
			Seq:      image.Seq,
		})
	}
	return rows
}

func momentUserToDTO(user *model.User, resolver storage.ObjectURLResolver) *dto.MomentUserResp {
	if user == nil {
		return nil
	}
	return &dto.MomentUserResp{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		AvatarUrl: resolvePtrURL(resolver, user.AvatarUrl),
		Site:      user.Site,
		Mark:      user.Mark,
	}
}

func (s *momentService) resolveImageURL(url string) string {
	return resolveURL(s.objectURLResolver, url)
}

func resolveURL(resolver storage.ObjectURLResolver, url string) string {
	if resolver == nil {
		return url
	}
	if trimmed := strings.TrimSpace(url); trimmed != "" && !isAbsoluteURL(trimmed) {
		if resolved, err := resolver.ObjectURL(context.Background(), trimmed); err == nil {
			return resolved
		}
	}
	return url
}

func resolvePtrURL(resolver storage.ObjectURLResolver, url *string) *string {
	if url == nil || resolver == nil {
		return url
	}
	trimmed := strings.TrimSpace(*url)
	if trimmed == "" || isAbsoluteURL(trimmed) {
		return url
	}
	if resolved, err := resolver.ObjectURL(context.Background(), trimmed); err == nil {
		return &resolved
	}
	return url
}

func isAbsoluteURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}
