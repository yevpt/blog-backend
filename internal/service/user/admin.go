package user

import (
	"context"
	"errors"

	"github.com/vpt/blog-backend/internal/dto"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/storage"
)

// ErrCannotDisableSelf 表示管理员试图禁用自己的账号。
var ErrCannotDisableSelf = errors.New("不能禁用自己的账号")

// ErrLastAdminAccount 表示目标账号是系统里最后一个管理员，禁止禁用。
var ErrLastAdminAccount = errors.New("不能禁用系统里最后一个管理员账号")

// AdminService 管理端用户用例。
type AdminService interface {
	GrantVip(targetUserID uint) (*dto.AdminUserRolesResp, error)
	RevokeVip(targetUserID uint) (*dto.AdminUserRolesResp, error)
	NormalizeAvatars(ctx context.Context, req *dto.NormalizeAvatarsReq) (*dto.NormalizeAvatarsResp, error)
	ClearUserAvatar(ctx context.Context, userID uint) (*dto.ClearUserAvatarResp, error)
	DisableAccount(operatorID, targetUserID uint) error
	EnableAccount(targetUserID uint) error
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

// DisableAccount 禁用目标账号；禁止管理员禁用自己，也禁止禁用系统里最后一个管理员账号。
func (s *adminService) DisableAccount(operatorID, targetUserID uint) error {
	if operatorID == targetUserID {
		return ErrCannotDisableSelf
	}
	user, err := s.repo.FindByID(targetUserID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	userRoles, err := s.repo.FindRolesByUserID(targetUserID)
	if err != nil {
		return err
	}
	for _, r := range userRoles {
		if r == roles.AdminRole {
			count, err := s.repo.CountByRole(roles.AdminRole)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastAdminAccount
			}
			break
		}
	}
	if err := s.repo.SetStatus(targetUserID, 0); err != nil {
		return err
	}
	_ = s.cache.Invalidate(context.Background(), int64(targetUserID))
	return nil
}

// EnableAccount 恢复目标账号为正常状态。
func (s *adminService) EnableAccount(targetUserID uint) error {
	user, err := s.repo.FindByID(targetUserID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	if err := s.repo.SetStatus(targetUserID, 1); err != nil {
		return err
	}
	_ = s.cache.Invalidate(context.Background(), int64(targetUserID))
	return nil
}
