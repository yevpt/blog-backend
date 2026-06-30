package user

import (
	"context"

	"github.com/vpt/blog-backend/internal/dto"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	"github.com/vpt/blog-backend/pkg/storage"
)

// AdminService 管理端用户用例。
type AdminService interface {
	GrantVip(targetUserID uint) (*dto.AdminUserRolesResp, error)
	RevokeVip(targetUserID uint) (*dto.AdminUserRolesResp, error)
	NormalizeAvatars(ctx context.Context, req *dto.NormalizeAvatarsReq) (*dto.NormalizeAvatarsResp, error)
	ClearUserAvatar(ctx context.Context, userID uint) (*dto.ClearUserAvatarResp, error)
}

type adminService struct {
	repo       userrepo.UserRepository
	cache      UserCacheService
	store      storage.ObjectStore
	avatar     AvatarNormalizer
	friendLink FriendLinkLogoRefs
}

// NewAdminService 创建管理端用户服务。
func NewAdminService(repo userrepo.UserRepository, cache UserCacheService, deps ...AdminDeps) AdminService {
	svc := &adminService{repo: repo, cache: cache}
	if len(deps) > 0 {
		svc.store = deps[0].Store
		svc.avatar = deps[0].Avatar
		svc.friendLink = deps[0].FriendLink
	}
	return svc
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
