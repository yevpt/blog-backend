package service

import (
	"errors"
	"math"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/pkg/storage"
	"gorm.io/gorm"
)

const (
	friendLinkStatusHidden    uint8 = 0
	friendLinkStatusVisible   uint8 = 1
	friendLinkDefaultPage           = 1
	friendLinkDefaultPageSize       = 10
	friendLinkMaxPageSize           = 50
)

var (
	ErrFriendLinkNotFound      = errors.New("友情链接不存在")
	ErrFriendLinkNameRequired  = errors.New("友情链接名称不能为空")
	ErrFriendLinkSiteRequired  = errors.New("友情链接地址不能为空")
	ErrFriendLinkSeqRequired   = errors.New("友情链接排序不能为空")
	ErrFriendLinkStatusInvalid = errors.New("友情链接状态无效")
)

// FriendLinkService 友情链接业务接口。
type FriendLinkService interface {
	ListPublic(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error)
	GetPublic(id uint) (*dto.FriendLinkItemResp, error)
	ListAdmin(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error)
	Create(req dto.FriendLinkCreateReq) (*dto.FriendLinkItemResp, error)
	Update(id uint, req dto.FriendLinkUpdateReq) (*dto.FriendLinkItemResp, error)
	Delete(id uint) (*dto.FriendLinkItemResp, error)
}

type friendLinkService struct {
	repo     repository.FriendLinkRepository
	resolver storage.ObjectURLResolver
}

// NewFriendLinkService 创建友情链接业务服务实例。
func NewFriendLinkService(repo repository.FriendLinkRepository, resolver storage.ObjectURLResolver) FriendLinkService {
	return &friendLinkService{repo: repo, resolver: resolver}
}

func (s *friendLinkService) ListPublic(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error) {
	// 先归一化分页参数，避免把异常页码传入仓储层。
	page, pageSize, offset := normalizeFriendLinkPage(req.Page, req.PageSize)

	// 查询公开可见数据，仓储负责 status 和排序条件。
	links, total, err := s.repo.ListPublic(offset, pageSize)
	if err != nil {
		return nil, err
	}

	// 转换为 DTO，并在返回前解析头像对象 key。
	return s.friendLinkPageToDTO(links, total, page, pageSize), nil
}

func (s *friendLinkService) GetPublic(id uint) (*dto.FriendLinkItemResp, error) {
	// 公开详情只允许读取显示中的友链。
	link, err := s.repo.GetPublic(id)
	if err != nil {
		return nil, mapFriendLinkRepoError(err)
	}
	if link == nil {
		return nil, ErrFriendLinkNotFound
	}

	return s.friendLinkToDTO(link), nil
}

func (s *friendLinkService) ListAdmin(req dto.FriendLinkListReq) (*dto.FriendLinkPageResp, error) {
	// 管理端允许按 status 过滤，但仍要限制非法状态值。
	if err := validateFriendLinkStatusPtr(req.Status); err != nil {
		return nil, err
	}
	page, pageSize, offset := normalizeFriendLinkPage(req.Page, req.PageSize)

	// 管理列表查询未删除数据，可包含隐藏项。
	links, total, err := s.repo.ListAdmin(offset, pageSize, req.Status)
	if err != nil {
		return nil, err
	}

	return s.friendLinkPageToDTO(links, total, page, pageSize), nil
}

func (s *friendLinkService) Create(req dto.FriendLinkCreateReq) (*dto.FriendLinkItemResp, error) {
	// 将请求转换为 model，集中完成 trim、必填校验和默认状态。
	link, err := newFriendLinkFromCreateReq(req)
	if err != nil {
		return nil, err
	}

	// 创建后返回最新数据，避免 handler 接触 model。
	created, err := s.repo.Create(link)
	if err != nil {
		return nil, err
	}

	return s.friendLinkToDTO(created), nil
}

func (s *friendLinkService) Update(id uint, req dto.FriendLinkUpdateReq) (*dto.FriendLinkItemResp, error) {
	// 将可选字段转换为明确的更新数据，未传字段不参与更新。
	data, err := newFriendLinkUpdateData(req)
	if err != nil {
		return nil, err
	}

	// 仓储返回 nil 表示目标不存在。
	updated, err := s.repo.Update(id, data)
	if err != nil {
		return nil, mapFriendLinkRepoError(err)
	}
	if updated == nil {
		return nil, ErrFriendLinkNotFound
	}

	return s.friendLinkToDTO(updated), nil
}

func (s *friendLinkService) Delete(id uint) (*dto.FriendLinkItemResp, error) {
	// 删除使用 GORM 软删除，保留原始记录用于返回。
	deleted, err := s.repo.Delete(id)
	if err != nil {
		return nil, mapFriendLinkRepoError(err)
	}
	if deleted == nil {
		return nil, ErrFriendLinkNotFound
	}

	return s.friendLinkToDTO(deleted), nil
}

func newFriendLinkFromCreateReq(req dto.FriendLinkCreateReq) (model.FriendLink, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return model.FriendLink{}, ErrFriendLinkNameRequired
	}
	site := strings.TrimSpace(req.Site)
	if site == "" {
		return model.FriendLink{}, ErrFriendLinkSiteRequired
	}
	if req.Seq == nil {
		return model.FriendLink{}, ErrFriendLinkSeqRequired
	}
	status := friendLinkStatusVisible
	if req.Status != nil {
		if err := validateFriendLinkStatus(*req.Status); err != nil {
			return model.FriendLink{}, err
		}
		status = *req.Status
	}

	return model.FriendLink{
		Name:        name,
		Description: cleanOptionalString(req.Description),
		Email:       cleanOptionalString(req.Email),
		Phone:       cleanOptionalString(req.Phone),
		Site:        site,
		AvatarUrl:   cleanOptionalString(req.AvatarUrl),
		Seq:         *req.Seq,
		Status:      status,
	}, nil
}

func newFriendLinkUpdateData(req dto.FriendLinkUpdateReq) (repository.FriendLinkUpdateData, error) {
	var data repository.FriendLinkUpdateData
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return data, ErrFriendLinkNameRequired
		}
		data.Name = &name
	}
	data.Description, data.UpdateDescription = cleanOptionalUpdateString(req.Description)
	data.Email, data.UpdateEmail = cleanOptionalUpdateString(req.Email)
	data.Phone, data.UpdatePhone = cleanOptionalUpdateString(req.Phone)
	if req.Site != nil {
		site := strings.TrimSpace(*req.Site)
		if site == "" {
			return data, ErrFriendLinkSiteRequired
		}
		data.Site = &site
	}
	data.AvatarUrl, data.UpdateAvatarUrl = cleanOptionalUpdateString(req.AvatarUrl)
	data.Seq = req.Seq
	if req.Status != nil {
		if err := validateFriendLinkStatus(*req.Status); err != nil {
			return data, err
		}
		data.Status = req.Status
	}

	return data, nil
}

func normalizeFriendLinkPage(page, pageSize int) (int, int, int) {
	if page < 1 {
		page = friendLinkDefaultPage
	}
	if pageSize < 1 {
		pageSize = friendLinkDefaultPageSize
	}
	if pageSize > friendLinkMaxPageSize {
		pageSize = friendLinkMaxPageSize
	}
	return page, pageSize, (page - 1) * pageSize
}

func validateFriendLinkStatusPtr(status *uint8) error {
	if status == nil {
		return nil
	}
	return validateFriendLinkStatus(*status)
}

func validateFriendLinkStatus(status uint8) error {
	if status != friendLinkStatusHidden && status != friendLinkStatusVisible {
		return ErrFriendLinkStatusInvalid
	}
	return nil
}

func mapFriendLinkRepoError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrFriendLinkNotFound
	}
	return err
}

func (s *friendLinkService) friendLinkPageToDTO(links []model.FriendLink, total int64, page, pageSize int) *dto.FriendLinkPageResp {
	list := make([]dto.FriendLinkItemResp, 0, len(links))
	for i := range links {
		list = append(list, *s.friendLinkToDTO(&links[i]))
	}

	pages := 0
	if pageSize > 0 && total > 0 {
		pages = int(math.Ceil(float64(total) / float64(pageSize)))
	}

	return &dto.FriendLinkPageResp{
		Total:    total,
		Pages:    pages,
		Page:     page,
		PageSize: pageSize,
		List:     list,
	}
}

func (s *friendLinkService) friendLinkToDTO(link *model.FriendLink) *dto.FriendLinkItemResp {
	if link == nil {
		return nil
	}
	return &dto.FriendLinkItemResp{
		ID:          link.ID,
		Name:        link.Name,
		Description: link.Description,
		Email:       link.Email,
		Phone:       link.Phone,
		Site:        link.Site,
		AvatarUrl:   storage.ResolvePtrURL(s.resolver, link.AvatarUrl),
		Seq:         link.Seq,
		Status:      link.Status,
		CreatedAt:   link.CreatedAt,
		UpdatedAt:   link.UpdatedAt,
	}
}
