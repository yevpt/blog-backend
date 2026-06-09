package service

import (
	"context"
	"math"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/pkg/storage"
)

// UserService 用户资料业务接口。
type UserService interface {
	GetDetail(userID uint) (*dto.UserDetailResp, error)
	ListRecent(req *dto.UserListReq) (*dto.UserPageResp, error)
	ListAll(req *dto.UserListReq) (*dto.UserPageResp, error)
	Update(userID uint, req *dto.UserUpdateReq) error
	RecordLogin(userID uint) error
}

type userService struct {
	cache    UserCacheService
	repo     repository.UserRepository
	resolver storage.ObjectURLResolver
}

// NewUserService 创建用户资料服务。
func NewUserService(cache UserCacheService, repo repository.UserRepository, resolver storage.ObjectURLResolver) UserService {
	return &userService{
		cache:    cache,
		repo:     repo,
		resolver: resolver,
	}
}

func (s *userService) GetDetail(userID uint) (*dto.UserDetailResp, error) {
	// context.Background() 是有意为之：此方法仅供 handler 过渡期使用，
	// Task 4 后 handler 将直接从 gin.Context 读取 UserDetail，此方法弃用。
	return s.cache.Get(context.Background(), int64(userID))
}

func (s *userService) ListRecent(req *dto.UserListReq) (*dto.UserPageResp, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	users, total, err := s.repo.ListRecent(offset, pageSize)
	if err != nil {
		return nil, err
	}

	return s.buildUserPageResp(users, total, page, pageSize)
}

func (s *userService) ListAll(req *dto.UserListReq) (*dto.UserPageResp, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	users, total, err := s.repo.ListAll(offset, pageSize)
	if err != nil {
		return nil, err
	}

	return s.buildUserPageResp(users, total, page, pageSize)
}

func (s *userService) buildUserPageResp(users []model.User, total int64, page, pageSize int) (*dto.UserPageResp, error) {
	if len(users) == 0 {
		return &dto.UserPageResp{
			Total:    total,
			Pages:    0,
			Page:     page,
			PageSize: pageSize,
			List:     []dto.UserListItemResp{},
		}, nil
	}

	userIDs := make([]uint, 0, len(users))
	for _, u := range users {
		userIDs = append(userIDs, u.ID)
	}

	rolesMap, err := s.repo.FindRolesByUserIDs(userIDs)
	if err != nil {
		return nil, err
	}

	list := make([]dto.UserListItemResp, 0, len(users))
	for _, u := range users {
		roles := rolesMap[u.ID]
		if roles == nil {
			roles = []string{} // 兜底为空切片，避免返回 nil
		}
		lastLoginAt := u.LastLoginAt
		if lastLoginAt == nil {
			t := u.CreatedAt
			lastLoginAt = &t
		}
		
		list = append(list, dto.UserListItemResp{
			ID:          u.ID,
			Nickname:    u.Nickname,
			AvatarUrl:   resolveUserAvatarURL(s.resolver, u.AvatarUrl),
			Mark:        u.Mark,
			Roles:       roles,
			LastLoginAt: lastLoginAt,
		})
	}

	pages := 0
	if pageSize > 0 {
		pages = int(math.Ceil(float64(total) / float64(pageSize)))
	}

	return &dto.UserPageResp{
		Total:    total,
		Pages:    pages,
		Page:     page,
		PageSize: pageSize,
		List:     list,
	}, nil
}

func (s *userService) Update(userID uint, req *dto.UserUpdateReq) error {
	updates := make(map[string]interface{})
	if req.Nickname != nil {
		updates["nickname"] = *req.Nickname
	}
	if req.AvatarUrl != nil {
		updates["avatar_url"] = *req.AvatarUrl
	}
	if req.Mark != nil {
		updates["mark"] = *req.Mark
	}
	
	if len(updates) > 0 {
		if err := s.repo.Update(userID, updates); err != nil {
			return err
		}
		// 使缓存失效
		_ = s.cache.Invalidate(context.Background(), int64(userID))
	}
	return nil
}

func (s *userService) RecordLogin(userID uint) error {
	err := s.repo.UpdateLastLoginAt(userID)
	if err == nil {
		_ = s.cache.Invalidate(context.Background(), int64(userID))
	}
	return err
}
