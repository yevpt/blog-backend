package moment

import (
	"context"
	"math"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
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
	images, err := s.mediaToDTO(aggregate.Images)
	if err != nil {
		return nil, err
	}
	return &dto.MomentItemResp{
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
		User:          momentUserToDTO(aggregate.User),
		Images:        images,
		CreatedAt:     moment.CreatedAt,
		UpdatedAt:     moment.UpdatedAt,
	}, nil
}

func momentUserToDTO(user *model.User) *dto.MomentUserResp {
	if user == nil {
		return nil
	}
	return &dto.MomentUserResp{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		AvatarUrl: user.AvatarUrl,
		Site:      user.Site,
		Mark:      user.Mark,
	}
}

func (s *momentService) mediaToDTO(images []model.Media) ([]dto.MomentMediaResp, error) {
	rows := make([]dto.MomentMediaResp, 0, len(images))
	for _, image := range images {
		accessURL, err := s.resolveAccessURL(image.URL)
		if err != nil {
			return nil, err
		}
		rows = append(rows, dto.MomentMediaResp{
			ID:        image.ID,
			Name:      image.Name,
			FileType:  image.FileType,
			URL:       image.URL,
			AccessURL: accessURL,
			Size:      image.Size,
			Seq:       image.Seq,
		})
	}
	return rows, nil
}

func (s *momentService) resolveAccessURL(objectName string) (string, error) {
	if s.objectURLResolver == nil {
		return objectName, nil
	}
	return s.objectURLResolver.ObjectURL(context.Background(), objectName)
}
