package user

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	ErrUsernameExists = errors.New("用户名已被占用")
	ErrWrongPassword  = errors.New("原密码错误")
)

// UserService 用户资料业务接口。
type UserService interface {
	GetDetail(userID uint) (*dto.UserDetailResp, error)
	GetPublicProfile(userID uint) (*dto.UserPublicProfileResp, error)
	ListLikedContent(userID uint, req dto.UserLikedContentListReq) (*dto.UserLikedContentPageResp, error)
	CountLikedContent(userID uint) (*dto.UserLikedContentCountResp, error)
	ListRecent(req *dto.UserListReq) (*dto.UserPageResp, error)
	ListAll(req *dto.UserListReq) (*dto.UserPageResp, error)
	Update(userID uint, req *dto.UserUpdateReq) (*dto.UserDetailResp, error)
	UpdateProfile(userID uint, req *dto.UpdateProfileReq) (*dto.UserDetailResp, error)
	UpdateMeta(userID uint, req *dto.UpdateMetaReq) (*dto.UserDetailResp, error)
	UpdateSocialLink(userID uint, platform string, url *string) (*dto.UserDetailResp, error)
	UpdateUsername(userID uint, username string) error
	UpdatePassword(userID uint, oldPwd, newPwd string) error
	SetInitialPassword(userID uint, newPwd, code string) error
	SendEmailCode(userID uint, emailAddr, captchaToken, ip string) error
	UpdateEmail(userID uint, target, emailAddr, code string) error
	UpdateEmailDisplay(userID uint, display string) error
	ChangeAvatar(userID uint, file *dto.UploadedImageFile) (*dto.UserDetailResp, error)
}

type userService struct {
	cache    UserCacheService
	repo     userrepo.UserRepository
	store    storage.ObjectStore
	resolver storage.ObjectURLResolver
	avatar   AvatarUploader
	security SecurityDeps
	presence OnlineChecker
}

// NewUserService 创建用户资料服务。
func NewUserService(
	cache UserCacheService,
	repo userrepo.UserRepository,
	store storage.ObjectStore,
	avatar AvatarUploader,
	presence OnlineChecker,
	security ...SecurityDeps,
) UserService {
	deps := SecurityDeps{}
	if len(security) > 0 {
		deps = security[0]
	}
	return &userService{
		cache:    cache,
		repo:     repo,
		store:    store,
		resolver: store,
		avatar:   avatar,
		security: deps,
		presence: presence,
	}
}

func (s *userService) GetDetail(userID uint) (*dto.UserDetailResp, error) {
	// context.Background() 是有意为之：此方法仅供 handler 过渡期使用，
	// Task 4 后 handler 将直接从 gin.Context 读取 UserDetail，此方法弃用。
	detail, err := s.cache.Get(context.Background(), int64(userID))
	if err != nil || detail == nil {
		return detail, err
	}
	enrichDetailPresence(context.Background(), s.presence, userID, &detail.IsOnline)
	return detail, nil
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

	return s.buildUserPageResp(context.Background(), users, total, page, pageSize)
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

	// 公开路由 GET /users 历史行为只展示 status = 1（正常）的用户，
	// 这里显式传入该筛选条件以保持向后兼容；管理端筛选走 Task 5 的新 handler。
	activeStatus := uint8(1)
	users, total, err := s.repo.ListAll(userrepo.UserListFilter{Status: &activeStatus}, offset, pageSize)
	if err != nil {
		return nil, err
	}

	return s.buildUserPageResp(context.Background(), users, total, page, pageSize)
}

func (s *userService) buildUserPageResp(ctx context.Context, users []model.User, total int64, page, pageSize int) (*dto.UserPageResp, error) {
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

		list = append(list, dto.UserListItemResp{
			ID:           u.ID,
			Nickname:     u.Nickname,
			AvatarUrl:    resolveUserAvatarURL(s.resolver, u.AvatarUrl),
			Mark:         u.Mark,
			Roles:        roles,
			LastLoginAt:  u.LastLoginAt,
			LastActiveAt: u.LastActiveAt,
		})
	}

	enrichListPresence(ctx, s.presence, list)

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

func (s *userService) Update(userID uint, req *dto.UserUpdateReq) (*dto.UserDetailResp, error) {
	updates := make(map[string]any)
	if req.Nickname != nil {
		updates["nickname"] = *req.Nickname
	}
	if req.Mark != nil {
		updates["mark"] = *req.Mark
	}

	if len(updates) > 0 {
		if err := s.repo.Update(userID, updates); err != nil {
			return nil, err
		}
		_ = s.cache.Invalidate(context.Background(), int64(userID))
	}
	return s.cache.Get(context.Background(), int64(userID))
}

func (s *userService) GetPublicProfile(userID uint) (*dto.UserPublicProfileResp, error) {
	agg, err := s.repo.FindDetailByID(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	if agg == nil {
		return nil, nil
	}
	resp := buildPublicProfile(s.resolver, agg)
	enrichDetailPresence(context.Background(), s.presence, userID, &resp.IsOnline)
	return resp, nil
}

func (s *userService) UpdateProfile(userID uint, req *dto.UpdateProfileReq) (*dto.UserDetailResp, error) {
	userUpdates := make(map[string]any)
	if req.Nickname != nil {
		userUpdates["nickname"] = *req.Nickname
	}
	if req.Mark != nil {
		userUpdates["mark"] = *req.Mark
	}
	if req.Site != nil {
		if *req.Site == "" {
			userUpdates["site"] = nil
		} else {
			userUpdates["site"] = *req.Site
		}
	}
	if len(userUpdates) > 0 {
		if err := s.repo.Update(userID, userUpdates); err != nil {
			return nil, err
		}
	}
	if req.Description != nil {
		if err := s.repo.UpsertMeta(userID, map[string]any{"description": *req.Description}); err != nil {
			return nil, err
		}
	}
	_ = s.cache.Invalidate(context.Background(), int64(userID))
	return s.cache.Get(context.Background(), int64(userID))
}

func (s *userService) UpdateMeta(userID uint, req *dto.UpdateMetaReq) (*dto.UserDetailResp, error) {
	metaUpdates := make(map[string]any)
	userUpdates := make(map[string]any)

	if req.Gender != nil {
		if *req.Gender == "" {
			metaUpdates["gender"] = nil
		} else {
			v, err := parseGender(*req.Gender)
			if err != nil {
				return nil, err
			}
			metaUpdates["gender"] = v
		}
	}
	if req.Birthday != nil {
		if *req.Birthday == "" {
			metaUpdates["birthday"] = nil
		} else {
			t, err := time.Parse("2006-01-02", *req.Birthday)
			if err != nil {
				return nil, fmt.Errorf("生日格式错误，应为 YYYY-MM-DD")
			}
			metaUpdates["birthday"] = t
		}
	}
	if req.Phone != nil {
		if *req.Phone == "" {
			userUpdates["phone"] = nil
		} else {
			userUpdates["phone"] = *req.Phone
		}
	}

	if len(metaUpdates) > 0 {
		if err := s.repo.UpsertMeta(userID, metaUpdates); err != nil {
			return nil, err
		}
	}
	if len(userUpdates) > 0 {
		if err := s.repo.Update(userID, userUpdates); err != nil {
			return nil, err
		}
	}
	_ = s.cache.Invalidate(context.Background(), int64(userID))
	return s.cache.Get(context.Background(), int64(userID))
}

func parseGender(s string) (uint8, error) {
	switch s {
	case "0":
		return 0, nil
	case "1":
		return 1, nil
	default:
		return 0, fmt.Errorf("性别值无效，应为 0 或 1")
	}
}

func (s *userService) UpdateSocialLink(userID uint, platform string, url *string) (*dto.UserDetailResp, error) {
	if url == nil || *url == "" {
		if err := s.repo.DeleteSocialLink(userID, platform); err != nil {
			return nil, err
		}
	} else {
		if err := s.repo.UpsertSocialLink(userID, platform, *url); err != nil {
			return nil, err
		}
	}
	_ = s.cache.Invalidate(context.Background(), int64(userID))
	return s.cache.Get(context.Background(), int64(userID))
}

func (s *userService) UpdateUsername(userID uint, username string) error {
	exists, err := s.repo.ExistsByUsername(username, userID)
	if err != nil {
		return err
	}
	if exists {
		return ErrUsernameExists
	}
	if err := s.repo.Update(userID, map[string]any{"username": username}); err != nil {
		return err
	}
	_ = s.cache.Invalidate(context.Background(), int64(userID))
	return nil
}

func (s *userService) UpdatePassword(userID uint, oldPwd, newPwd string) error {
	user, err := s.repo.FindByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPwd)); err != nil {
		return ErrWrongPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(userID, string(hash))
}

func (s *userService) UpdateEmailDisplay(userID uint, display string) error {
	var mailShow uint8
	switch display {
	case "main":
		mailShow = 1
	case "sub":
		mailShow = 0
	case "none":
		mailShow = 2
	default:
		return fmt.Errorf("无效的展示类型")
	}
	if err := s.repo.UpsertUserSetting(userID, map[string]any{"mail_show": mailShow}); err != nil {
		return err
	}
	_ = s.cache.Invalidate(context.Background(), int64(userID))
	return nil
}
