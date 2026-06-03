package moment

import (
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
	"github.com/vpt/blog-backend/pkg/roles"
)

func (s *momentService) List(req dto.MomentListReq, viewerID *uint) (*dto.MomentPageResp, error) {
	result, err := s.repo.List(momentrepo.ListFilter{
		Page:     normalizeMomentPage(req.Page),
		PageSize: normalizeMomentPageSize(req.PageSize),
		UserID:   req.UserID,
		RoleID:   req.RoleID,
	}, viewerID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return s.momentPageToDTO(result)
}

func (s *momentService) GetDetail(id uint, viewerID *uint) (*dto.MomentItemResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	aggregate, err := s.repo.FindPublicDetail(id, viewerID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return s.momentToDTO(*aggregate)
}

func (s *momentService) Save(req dto.MomentSaveReq, operatorID uint, roleNames []string) (*dto.MomentItemResp, error) {
	content, err := cleanMomentContent(req.Content)
	if err != nil {
		return nil, err
	}

	force := hasAdminRole(roleNames)
	authorID := operatorID
	if force && req.UserID != nil && *req.UserID > 0 {
		authorID = *req.UserID
	}

	moment := model.Moment{
		UserID:        authorID,
		Content:       content,
		Status:        req.Status,
		CommentStatus: req.CommentStatus,
	}
	if req.ID != nil {
		moment.ID = *req.ID
	}

	aggregate, err := s.repo.Save(momentrepo.SaveData{
		Moment:     moment,
		Images:     momentImagesFromDTO(req.Images),
		OperatorID: operatorID,
		Force:      force,
	})
	if err != nil {
		return nil, mapRepoError(err)
	}
	return s.momentToDTO(*aggregate)
}

func (s *momentService) Delete(id uint, operatorID uint, roleNames []string) (*dto.MomentDeleteResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	moment, err := s.repo.Delete(id, operatorID, hasAdminRole(roleNames))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.MomentDeleteResp{ID: moment.ID}, nil
}

func (s *momentService) SetTop(id uint, operatorID uint, roleNames []string) (*dto.MomentTopResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	moment, err := s.repo.SetTop(id, operatorID, hasAdminRole(roleNames))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.MomentTopResp{ID: moment.ID, IsTop: moment.IsTop}, nil
}

func (s *momentService) RemoveTop(id uint, operatorID uint, roleNames []string) (*dto.MomentTopResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	moment, err := s.repo.RemoveTop(id, operatorID, hasAdminRole(roleNames))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.MomentTopResp{ID: moment.ID, IsTop: moment.IsTop}, nil
}

func (s *momentService) Read(id uint) (*dto.MomentReadResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	moment, err := s.repo.IncrementReadCount(id)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.MomentReadResp{ID: moment.ID, ReadCount: moment.ReadCount}, nil
}

func (s *momentService) IsLiked(id uint, userID uint) (*dto.MomentLikeResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	liked, count, err := s.repo.IsLiked(id, userID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.MomentLikeResp{IsLiked: liked, LikeCount: count}, nil
}

func (s *momentService) ToggleLike(id uint, userID uint) (*dto.MomentItemResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	aggregate, _, err := s.repo.ToggleLike(id, userID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return s.momentToDTO(*aggregate)
}

func cleanMomentContent(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", ErrMomentContentRequired
	}
	return trimmed, nil
}

func normalizeMomentPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizeMomentPageSize(pageSize int) int {
	if pageSize < 1 {
		return 10
	}
	if pageSize > 50 {
		return 50
	}
	return pageSize
}

func hasAdminRole(roleNames []string) bool {
	for _, roleName := range roleNames {
		if roleName == roles.AdminRole {
			return true
		}
	}
	return false
}

func mapRepoError(err error) error {
	if errors.Is(err, momentrepo.ErrMomentNotFound) {
		return ErrMomentNotFound
	}
	if errors.Is(err, momentrepo.ErrAuthorNotFound) {
		return ErrMomentAuthorNotFound
	}
	if errors.Is(err, momentrepo.ErrNoPermission) {
		return ErrMomentNoPermission
	}
	if errors.Is(err, momentrepo.ErrTopLimitExceeded) {
		return ErrMomentTopLimitExceeded
	}
	return err
}

func momentImagesFromDTO(images []dto.MomentMediaReq) []model.Media {
	rows := make([]model.Media, 0, len(images))
	for _, image := range images {
		rows = append(rows, model.Media{
			Name:     image.Name,
			FileType: image.FileType,
			URL:      strings.TrimSpace(image.URL),
			Size:     image.Size,
			Seq:      image.Seq,
		})
	}
	return rows
}
