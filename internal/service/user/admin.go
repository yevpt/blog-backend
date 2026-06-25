package user

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
)

// AdminService 管理端用户用例。
type AdminService interface {
	GrantVip(targetUserID uint) (*dto.AdminUserRolesResp, error)
	RevokeVip(targetUserID uint) (*dto.AdminUserRolesResp, error)
}

type adminService struct {
	repo  userrepo.UserRepository
	cache UserCacheService
}

// NewAdminService 创建管理端用户服务。
func NewAdminService(repo userrepo.UserRepository, cache UserCacheService) AdminService {
	return &adminService{repo: repo, cache: cache}
}

func (s *adminService) GrantVip(targetUserID uint) (*dto.AdminUserRolesResp, error) {
	return s.updateVipRole(targetUserID, s.repo.GrantVipRole)
}

func (s *adminService) RevokeVip(targetUserID uint) (*dto.AdminUserRolesResp, error) {
	return s.updateVipRole(targetUserID, s.repo.RevokeVipRole)
}

func (s *adminService) updateVipRole(targetUserID uint, mutate func(uint) error) (*dto.AdminUserRolesResp, error) {
	user, err := s.repo.FindByID(targetUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	if err := mutate(targetUserID); err != nil {
		return nil, err
	}
	_ = s.cache.Invalidate(context.Background(), int64(targetUserID))

	roles, err := s.repo.FindRolesByUserID(targetUserID)
	if err != nil {
		return nil, err
	}
	if roles == nil {
		roles = []string{}
	}
	return &dto.AdminUserRolesResp{UserID: targetUserID, Roles: roles}, nil
}
