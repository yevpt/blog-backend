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

// ModerationProfileReader 只读取用户的处罚状态，供用户列表/详情展示，避免反向依赖完整审核服务。
type ModerationProfileReader interface {
	GetSanctionState(userID uint) (string, error)
}

// AdminService 管理端用户用例。
type AdminService interface {
	GrantVip(targetUserID uint) (*dto.AdminUserRolesResp, error)
	RevokeVip(targetUserID uint) (*dto.AdminUserRolesResp, error)
	NormalizeAvatars(ctx context.Context, req *dto.NormalizeAvatarsReq) (*dto.NormalizeAvatarsResp, error)
	ClearUserAvatar(ctx context.Context, userID uint) (*dto.ClearUserAvatarResp, error)
	DisableAccount(operatorID, targetUserID uint) error
	EnableAccount(targetUserID uint) error
	ListAdmin(req *dto.AdminUserListReq) (*dto.AdminUserPageResp, error)
	GetAdminDetail(userID uint) (*dto.AdminUserDetailResp, error)
}

type adminService struct {
	repo       userrepo.UserRepository
	cache      UserCacheService
	store      storage.ObjectStore
	avatar     AvatarNormalizer
	friendLink FriendLinkLogoRefs
	moderation ModerationProfileReader
	presence   OnlineChecker
}

// NewAdminService 创建管理端用户服务。
func NewAdminService(repo userrepo.UserRepository, cache UserCacheService, deps ...AdminDeps) AdminService {
	svc := &adminService{repo: repo, cache: cache}
	if len(deps) > 0 {
		svc.store = deps[0].Store
		svc.avatar = deps[0].Avatar
		svc.friendLink = deps[0].FriendLink
		svc.moderation = deps[0].Moderation
		svc.presence = deps[0].Presence
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

// ListAdmin 管理端分页查询用户，支持关键词/角色/状态筛选。
func (s *adminService) ListAdmin(req *dto.AdminUserListReq) (*dto.AdminUserPageResp, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	filter := userrepo.UserListFilter{Keyword: req.Keyword, Role: req.Role}
	if req.Status == "active" {
		status := uint8(1)
		filter.Status = &status
	} else if req.Status == "disabled" {
		status := uint8(0)
		filter.Status = &status
	}

	users, total, err := s.repo.ListAll(filter, offset, pageSize)
	if err != nil {
		return nil, err
	}

	ids := make([]uint, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	rolesByUser, err := s.repo.FindRolesByUserIDs(ids)
	if err != nil {
		return nil, err
	}

	list := make([]dto.AdminUserListItemResp, 0, len(users))
	for _, u := range users {
		list = append(list, dto.AdminUserListItemResp{
			ID: u.ID, Username: u.Username, Nickname: u.Nickname, Email: u.Email,
			AvatarUrl: u.AvatarUrl, Mark: u.Mark, Roles: rolesByUser[u.ID],
			Status: u.Status, SanctionState: s.resolveSanctionState(u.ID),
			LastLoginAt: u.LastLoginAt, LastActiveAt: u.LastActiveAt,
			CreatedAt: u.CreatedAt,
		})
	}
	enrichListPresenceBy(context.Background(), s.presence, len(list),
		func(i int) uint { return list[i].ID },
		func(i int, online bool) { list[i].IsOnline = online },
	)

	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	return &dto.AdminUserPageResp{Total: total, Pages: pages, Page: page, PageSize: pageSize, List: list}, nil
}

// GetAdminDetail 管理端查询用户详情，含真实邮箱/手机号与审核画像摘要。
func (s *adminService) GetAdminDetail(userID uint) (*dto.AdminUserDetailResp, error) {
	detail, err := s.repo.FindDetailByID(userID)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, ErrUserNotFound
	}

	resp := &dto.AdminUserDetailResp{
		ID: detail.User.ID, Username: detail.User.Username, Nickname: detail.User.Nickname,
		Email: detail.User.Email, EmailVerified: detail.User.EmailVerifiedAt != nil,
		Phone: detail.User.Phone, Site: detail.User.Site, AvatarUrl: detail.User.AvatarUrl,
		Mark: detail.User.Mark, Status: detail.User.Status, PasswordSet: detail.User.PasswordSet,
		Roles: detail.Roles, RegisterAt: detail.User.CreatedAt,
		LastLoginAt: detail.User.LastLoginAt, LastActiveAt: detail.User.LastActiveAt,
		SanctionState: s.resolveSanctionState(userID),
	}
	enrichDetailPresence(context.Background(), s.presence, userID, &resp.IsOnline)
	return resp, nil
}

// resolveSanctionState 读取用户处罚状态；无审核依赖或查询失败/无记录时视为 active。
func (s *adminService) resolveSanctionState(userID uint) string {
	if s.moderation == nil {
		return "active"
	}
	if state, err := s.moderation.GetSanctionState(userID); err == nil && state != "" {
		return state
	}
	return "active"
}
