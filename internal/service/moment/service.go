package moment

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

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

func (s *momentService) ListAdmin(req dto.AdminMomentListReq) (*dto.AdminMomentPageResp, error) {
	status, err := adminMomentStatus(req.Status)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.ListAdmin(momentrepo.AdminListFilter{
		Page:     normalizeMomentPage(req.Page),
		PageSize: normalizeMomentPageSize(req.PageSize),
		Status:   status,
		Search:   strings.TrimSpace(req.Search),
	})
	if err != nil {
		return nil, mapRepoError(err)
	}
	page, err := s.momentPageToDTO(result)
	if err != nil {
		return nil, err
	}
	return &dto.AdminMomentPageResp{
		Total:    page.Total,
		Pages:    page.Pages,
		Page:     page.Page,
		PageSize: page.PageSize,
		List:     page.List,
	}, nil
}

func adminMomentStatus(status string) (*uint8, error) {
	switch status {
	case "", "all":
		return nil, nil
	case "public":
		value := uint8(1)
		return &value, nil
	case "hidden":
		value := uint8(0)
		return &value, nil
	default:
		return nil, ErrMomentInvalid
	}
}

func (s *momentService) GetDetail(id uint, viewerID *uint) (*dto.MomentItemResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	aggregate, err := s.repo.FindPublicDetail(id, viewerID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return s.momentToDTO(*aggregate, nil)
}

func (s *momentService) Save(req dto.MomentSaveReq, operatorID uint, roleNames []string) (resp *dto.MomentItemResp, err error) {
	uploaded := make([]string, 0, len(req.ImageFiles))
	rollbackUploaded := true
	defer func() {
		if recovered := recover(); recovered != nil {
			if rollbackUploaded {
				_ = s.deleteUploadedMomentImages(context.Background(), uploaded)
			}
			panic(recovered)
		}
		if err != nil && rollbackUploaded {
			err = errors.Join(err, s.deleteUploadedMomentImages(context.Background(), uploaded))
		}
	}()

	content, err := cleanMomentContent(req.Content)
	if err != nil {
		return nil, err
	}

	force := roles.HasPermission(roleNames, roles.AdminRole)
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

	removedURLs := make([]string, 0)
	aggregate, err := s.repo.Save(momentrepo.SaveData{
		Moment: moment,
		PrepareImages: func(saved model.Moment) ([]model.Media, error) {
			return s.prepareMomentImages(context.Background(), saved, req, &uploaded)
		},
		RemovedURLs: &removedURLs,
		OperatorID:  operatorID,
		Force:       force,
	})
	if err != nil {
		return nil, mapRepoError(err)
	}
	rollbackUploaded = false
	if err := s.deleteRemovedMomentImages(context.Background(), removedURLs); err != nil {
		return nil, err
	}
	return s.momentToDTO(*aggregate, nil)
}

func (s *momentService) Delete(id uint, operatorID uint, roleNames []string) (*dto.MomentDeleteResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	moment, images, err := s.repo.Delete(id, operatorID, roles.HasPermission(roleNames, roles.AdminRole))
	if err != nil {
		return nil, mapRepoError(err)
	}
	urls := mediaURLs(images)
	if err := s.deleteRemovedMomentImages(context.Background(), urls); err != nil {
		return nil, err
	}
	return &dto.MomentDeleteResp{ID: moment.ID}, nil
}

func mediaURLs(images []model.Media) []string {
	urls := make([]string, 0, len(images))
	for _, img := range images {
		if img.URL != "" {
			urls = append(urls, img.URL)
		}
	}
	return urls
}

func (s *momentService) SetTop(id uint, operatorID uint, roleNames []string) (*dto.MomentTopResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	moment, err := s.repo.SetTop(id, operatorID, roles.HasPermission(roleNames, roles.AdminRole))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.MomentTopResp{ID: moment.ID, IsTop: moment.IsTop}, nil
}

func (s *momentService) RemoveTop(id uint, operatorID uint, roleNames []string) (*dto.MomentTopResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	moment, err := s.repo.RemoveTop(id, operatorID, roles.HasPermission(roleNames, roles.AdminRole))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.MomentTopResp{ID: moment.ID, IsTop: moment.IsTop}, nil
}

func (s *momentService) View(id uint, visitorID string) (*dto.MomentViewResp, error) {
	if id == 0 {
		return nil, ErrMomentInvalid
	}
	// 访客去重：同一访客 24 小时内只计一次。
	isNew := true
	if visitorID != "" {
		is, err := s.uvSvc.CheckAndMark(context.Background(), "moment:viewed", strconv.FormatUint(uint64(id), 10), visitorID, 24*time.Hour)
		if err != nil {
			// Redis 异常时降级，视作新访客。
			isNew = true
		} else {
			isNew = is
		}
	}
	if !isNew {
		// 重复访客，返回当前阅读数，不增加。
		aggregate, err := s.repo.FindPublicDetail(id, nil)
		if err != nil {
			return nil, mapRepoError(err)
		}
		if aggregate == nil {
			return nil, ErrMomentNotFound
		}
		return &dto.MomentViewResp{ID: aggregate.Moment.ID, ViewCount: aggregate.Moment.ReadCount}, nil
	}
	moment, err := s.repo.IncrementReadCount(id)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.MomentViewResp{ID: moment.ID, ViewCount: moment.ReadCount}, nil
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
	aggregate, liked, err := s.repo.ToggleLike(id, userID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	// 仅在本次为点赞（而非取消）时发布通知事件，接收人由分发器按碎语作者解析。
	// 自己点赞自己的碎语不产生通知事件。
	if liked && aggregate.Moment.UserID != userID {
		s.notifyMomentLiked(id, userID, aggregate.Moment.Content)
	}
	return s.momentToDTO(*aggregate, nil)
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
