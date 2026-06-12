package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	domain "github.com/vpt/blog-backend/internal/oauth"
	"github.com/vpt/blog-backend/internal/repository"
	userservice "github.com/vpt/blog-backend/internal/service"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/roles"
)

var (
	ErrLoginRequired       = errors.New("请先登录后再绑定第三方账号")
	ErrSocialIdentityBound = errors.New("第三方账号已被其他用户绑定")
	ErrSourceAlreadyBound  = errors.New("当前用户已绑定该平台账号")
	ErrUserNotFound        = errors.New("用户不存在")
	ErrUserDisabled        = errors.New("账号已被禁用")
	ErrLastLoginMethod     = errors.New("至少保留一种可用登录方式")
)

// FlowManager 抽象 OAuth 协议流程，便于 service 测试时替换。
type FlowManager interface {
	Sources() []string
	Authorize(ctx context.Context, source string, flow domain.FlowContext) (string, error)
	Callback(ctx context.Context, source string, code string, state string) (*domain.CallbackResult, error)
}

// OAuthService 第三方登录业务接口。
type OAuthService interface {
	Providers(ctx context.Context) []string
	Authorize(ctx context.Context, source string, action domain.Action, userID uint, redirectURI string) (*dto.OAuthAuthorizeResp, error)
	Callback(ctx context.Context, source string, code string, state string) (*dto.OAuthCallbackResp, error)
	ListBindings(ctx context.Context, userID uint) ([]dto.OAuthBindingResp, error)
	Unbind(ctx context.Context, userID uint, source string) error
}

// AvatarSaver 保存远程头像并返回本站对象存储 key。
type AvatarSaver interface {
	SaveRemoteAvatar(ctx context.Context, avatarURL string) (string, error)
}

type service struct {
	flow       FlowManager
	socialRepo repository.SocialAuthRepository
	userRepo   repository.UserRepository
	jwt        *jwtpkg.Manager
	cache      userservice.UserCacheService
	avatar     AvatarSaver
}

func NewOAuthService(
	flow FlowManager,
	socialRepo repository.SocialAuthRepository,
	userRepo repository.UserRepository,
	jwt *jwtpkg.Manager,
	cache userservice.UserCacheService,
	avatar AvatarSaver,
) OAuthService {
	return &service{
		flow:       flow,
		socialRepo: socialRepo,
		userRepo:   userRepo,
		jwt:        jwt,
		cache:      cache,
		avatar:     avatar,
	}
}

func (s *service) Providers(ctx context.Context) []string {
	return s.flow.Sources()
}

func (s *service) Authorize(ctx context.Context, source string, action domain.Action, userID uint, redirectURI string) (*dto.OAuthAuthorizeResp, error) {
	// 绑定必须从当前登录态发起，callback 阶段只信任 Redis state 中保存的 user_id。
	if action == domain.ActionBind && userID == 0 {
		return nil, ErrLoginRequired
	}

	authorizeURL, err := s.flow.Authorize(ctx, source, domain.FlowContext{
		Source:      source,
		Action:      action,
		UserID:      userID,
		RedirectURI: redirectURI,
	})
	if err != nil {
		return nil, err
	}
	return &dto.OAuthAuthorizeResp{AuthorizeURL: authorizeURL}, nil
}

func (s *service) Callback(ctx context.Context, source string, code string, state string) (*dto.OAuthCallbackResp, error) {
	result, err := s.flow.Callback(ctx, source, code, state)
	if err != nil {
		return nil, err
	}

	switch result.Flow.Action {
	case domain.ActionLogin:
		login, err := s.login(ctx, result)
		if err != nil {
			return nil, err
		}
		return &dto.OAuthCallbackResp{Action: string(domain.ActionLogin), Login: login}, nil
	case domain.ActionBind:
		binding, err := s.bind(ctx, result)
		if err != nil {
			return nil, err
		}
		return &dto.OAuthCallbackResp{Action: string(domain.ActionBind), Binding: binding}, nil
	default:
		return nil, domain.ErrInvalidAction
	}
}

func (s *service) ListBindings(ctx context.Context, userID uint) ([]dto.OAuthBindingResp, error) {
	bindings, err := s.socialRepo.ListBindings(userID)
	if err != nil {
		return nil, err
	}
	resp := make([]dto.OAuthBindingResp, 0, len(bindings))
	for _, binding := range bindings {
		resp = append(resp, dto.OAuthBindingResp{
			Source:       binding.Source,
			SocialUserID: binding.SocialUserID,
		})
	}
	return resp, nil
}

func (s *service) Unbind(ctx context.Context, userID uint, source string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	count, err := s.socialRepo.CountBindings(userID)
	if err != nil {
		return err
	}
	// 当前模型里 OAuth 用户会写入随机密码；这里仍保留保护，避免未来支持无密码账号时解绑到无法登录。
	if strings.TrimSpace(user.Password) == "" && count <= 1 {
		return ErrLastLoginMethod
	}
	if err := s.socialRepo.Unbind(userID, source); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Invalidate(ctx, int64(userID))
	}
	return nil
}

func (s *service) login(ctx context.Context, result *domain.CallbackResult) (*dto.LoginResp, error) {
	socialUser, err := s.socialRepo.FindSocialUser(result.Profile.Source, result.Profile.UUID)
	if err != nil {
		return nil, err
	}
	if socialUser != nil {
		user, err := s.socialRepo.FindUserBySocialUserID(socialUser.ID)
		if err != nil {
			return nil, err
		}
		if user == nil {
			return nil, ErrUserNotFound
		}
		return s.issueLogin(ctx, user)
	}

	user, socialUser, err := s.newOAuthUser(ctx, result)
	if err != nil {
		return nil, err
	}
	if err := s.socialRepo.CreateUserWithSocialAuth(user, roles.NormalRoleId, socialUser); err != nil {
		return nil, err
	}
	return s.issueLogin(ctx, user)
}

func (s *service) bind(ctx context.Context, result *domain.CallbackResult) (*dto.OAuthBindingResp, error) {
	userID := result.Flow.UserID
	if userID == 0 {
		return nil, ErrLoginRequired
	}

	socialUser, err := s.socialRepo.FindSocialUser(result.Profile.Source, result.Profile.UUID)
	if err != nil {
		return nil, err
	}
	if socialUser != nil {
		boundUser, err := s.socialRepo.FindUserBySocialUserID(socialUser.ID)
		if err != nil {
			return nil, err
		}
		if boundUser != nil {
			return nil, ErrSocialIdentityBound
		}
	} else {
		socialUser = socialUserFromCallback(result)
	}

	// 同一本站用户同一平台只允许一个绑定账号，避免登录入口语义混乱。
	existing, err := s.socialRepo.FindBindingByUserAndSource(userID, result.Profile.Source)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrSourceAlreadyBound
	}

	if err := s.socialRepo.BindExistingUser(userID, socialUser); err != nil {
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.Invalidate(ctx, int64(userID))
	}
	return &dto.OAuthBindingResp{Source: socialUser.Source, SocialUserID: socialUser.ID}, nil
}

func (s *service) issueLogin(ctx context.Context, user *model.User) (*dto.LoginResp, error) {
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	accessToken, err := s.jwt.GenerateAccess(int64(user.ID))
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.jwt.GenerateRefresh(int64(user.ID))
	if err != nil {
		return nil, err
	}
	roles, err := s.userRepo.FindRolesByUserID(user.ID)
	if err != nil {
		return nil, err
	}
	_ = s.userRepo.UpdateLastLoginAt(user.ID)
	if s.cache != nil {
		_ = s.cache.Invalidate(ctx, int64(user.ID))
	}

	return &dto.LoginResp{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    7200,
		User: dto.UserResp{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Nickname: user.Nickname,
			Roles:    roles,
		},
	}, nil
}

func (s *service) newOAuthUser(ctx context.Context, result *domain.CallbackResult) (*model.User, *model.SocialUser, error) {
	username := oauthUsername(result.Profile)
	var email *string
	if result.Profile.Email != nil {
		exists, err := s.userRepo.ExistsByEmail(*result.Profile.Email)
		if err != nil {
			return nil, nil, err
		}
		// 邮箱已存在时不自动合并，避免第三方邮箱语义差异导致账号被错误接管。
		if !exists {
			username = *result.Profile.Email
			email = result.Profile.Email
		}
	}

	password, err := randomPasswordHash()
	if err != nil {
		return nil, nil, err
	}
	nickname := result.Profile.Nickname
	if nickname == nil {
		fallback := username
		nickname = &fallback
	}

	user := &model.User{
		Username: username,
		Password: password,
		Nickname: nickname,
		Email:    email,
		Site:     result.Profile.BlogURL,
		Status:   1,
	}
	// 第三方头像只在首次注册时同步短超时处理；失败不影响注册，也不保存第三方原始 URL。
	if avatarURL := s.saveAvatarIfPossible(ctx, result.Profile.AvatarURL); avatarURL != nil {
		user.AvatarUrl = avatarURL
	}
	return user, socialUserFromCallback(result), nil
}

func (s *service) saveAvatarIfPossible(ctx context.Context, remoteURL *string) *string {
	if s.avatar == nil || remoteURL == nil || strings.TrimSpace(*remoteURL) == "" {
		return nil
	}
	objectName, err := s.avatar.SaveRemoteAvatar(ctx, *remoteURL)
	if err != nil || strings.TrimSpace(objectName) == "" {
		return nil
	}
	return &objectName
}

func socialUserFromCallback(result *domain.CallbackResult) *model.SocialUser {
	socialUser := &model.SocialUser{
		UUID:        result.Profile.UUID,
		Source:      result.Profile.Source,
		AccessToken: result.Token.AccessToken,
		OpenID:      result.Profile.OpenID,
		IsActive:    true,
	}
	if result.Token.RefreshToken != nil {
		socialUser.RefreshToken = result.Token.RefreshToken
	}
	return socialUser
}

func oauthUsername(profile *domain.Profile) string {
	return fmt.Sprintf("%s_%s", profile.Source, profile.UUID)
}

func randomPasswordHash() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	hashed, err := bcrypt.GenerateFromPassword([]byte(raw), 12)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}
