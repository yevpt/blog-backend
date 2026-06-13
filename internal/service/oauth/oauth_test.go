package oauth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	domain "github.com/vpt/blog-backend/internal/oauth"
	"github.com/vpt/blog-backend/internal/repository"
	serviceoauth "github.com/vpt/blog-backend/internal/service/oauth"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/roles"
)

type fakeFlowManager struct {
	authorizeURL string
	callback     *domain.CallbackResult
	gotFlow      domain.FlowContext
}

func (m *fakeFlowManager) Sources() []string { return []string{"github"} }

func (m *fakeFlowManager) Authorize(ctx context.Context, source string, flow domain.FlowContext) (string, error) {
	m.gotFlow = flow
	return m.authorizeURL, nil
}

func (m *fakeFlowManager) Callback(ctx context.Context, source string, code string, state string) (*domain.CallbackResult, error) {
	return m.callback, nil
}

type fakeSocialRepo struct {
	socialUser      *model.SocialUser
	boundUser       *model.User
	binding         *repository.SocialBinding
	bindingCount    int64
	createdUser     *model.User
	createdRoleID   uint
	createdSocial   *model.SocialUser
	boundExistingID uint
	boundSocial     *model.SocialUser
	unboundUserID   uint
	unboundSource   string
}

func (r *fakeSocialRepo) FindSocialUser(source string, uuid string) (*model.SocialUser, error) {
	return r.socialUser, nil
}

func (r *fakeSocialRepo) FindUserBySocialUserID(socialUserID uint) (*model.User, error) {
	return r.boundUser, nil
}

func (r *fakeSocialRepo) CreateUserWithSocialAuth(user *model.User, roleID uint, socialUser *model.SocialUser) error {
	user.ID = 7
	socialUser.ID = 11
	r.createdUser = user
	r.createdRoleID = roleID
	r.createdSocial = socialUser
	return nil
}

func (r *fakeSocialRepo) BindExistingUser(userID uint, socialUser *model.SocialUser) error {
	r.boundExistingID = userID
	r.boundSocial = socialUser
	return nil
}

func (r *fakeSocialRepo) FindBindingByUserAndSource(userID uint, source string) (*repository.SocialBinding, error) {
	return r.binding, nil
}

func (r *fakeSocialRepo) ListBindings(userID uint) ([]repository.SocialBinding, error) {
	return nil, nil
}

func (r *fakeSocialRepo) CountBindings(userID uint) (int64, error) {
	return r.bindingCount, nil
}

func (r *fakeSocialRepo) Unbind(userID uint, source string) error {
	r.unboundUserID = userID
	r.unboundSource = source
	return nil
}

type fakeUserRepo struct {
	user        *model.User
	roles       []string
	emailExists bool
}

func (r *fakeUserRepo) FindByIdentifier(identifier string) (*model.User, error) { return r.user, nil }
func (r *fakeUserRepo) FindByID(id uint) (*model.User, error)                   { return r.user, nil }
func (r *fakeUserRepo) FindDetailByID(id uint) (*repository.UserDetailAggregate, error) {
	return nil, nil
}
func (r *fakeUserRepo) ExistsByEmail(email string) (bool, error)       { return r.emailExists, nil }
func (r *fakeUserRepo) ExistsByNickname(nickname string) (bool, error) { return false, nil }
func (r *fakeUserRepo) Create(user *model.User, roleID uint) error     { return nil }
func (r *fakeUserRepo) FindRolesByUserID(userID uint) ([]string, error) {
	return r.roles, nil
}
func (r *fakeUserRepo) FindRolesByUserIDs(userIDs []uint) (map[uint][]string, error) {
	return nil, nil
}
func (r *fakeUserRepo) UpdateLastLoginAt(userID uint) error { return nil }
func (r *fakeUserRepo) ListRecent(offset, limit int) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *fakeUserRepo) ListAll(offset, limit int) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *fakeUserRepo) Update(id uint, updates map[string]interface{}) error { return nil }

type fakeAvatarSaver struct {
	objectName string
	err        error
	gotURL     string
}

func (s *fakeAvatarSaver) SaveRemoteAvatar(ctx context.Context, avatarURL string) (string, error) {
	s.gotURL = avatarURL
	return s.objectName, s.err
}

func newTestService(flow *fakeFlowManager, social *fakeSocialRepo, user *fakeUserRepo) serviceoauth.OAuthService {
	return serviceoauth.NewOAuthService(flow, social, user, jwtpkg.NewManager("secret", 2, 168), nil, nil)
}

func TestOAuthService_AuthorizeBindRequiresUser(t *testing.T) {
	svc := newTestService(&fakeFlowManager{}, &fakeSocialRepo{}, &fakeUserRepo{})

	_, err := svc.Authorize(context.Background(), "github", domain.ActionBind, 0, "")

	assert.ErrorIs(t, err, serviceoauth.ErrLoginRequired)
}

func TestOAuthService_CallbackLoginCreatesUserAndBinding(t *testing.T) {
	email := "octo@example.com"
	nickname := "Octo"
	avatar := "https://avatars.example.com/octo.png"
	blog := "https://octo.example.com"
	flow := &fakeFlowManager{
		callback: &domain.CallbackResult{
			Flow: &domain.FlowContext{Action: domain.ActionLogin, Source: "github"},
			Token: &domain.TokenSet{
				AccessToken: "access-token",
			},
			Profile: &domain.Profile{
				Source:    "github",
				UUID:      "remote-123",
				Email:     &email,
				Nickname:  &nickname,
				AvatarURL: &avatar,
				BlogURL:   &blog,
			},
		},
	}
	social := &fakeSocialRepo{}
	user := &fakeUserRepo{roles: []string{roles.NormalRole}}
	avatarSaver := &fakeAvatarSaver{objectName: "avatar/user/md5.jpg"}
	svc := serviceoauth.NewOAuthService(flow, social, user, jwtpkg.NewManager("secret", 2, 168), nil, avatarSaver)

	resp, err := svc.Callback(context.Background(), "github", "code", "state")

	require.NoError(t, err)
	require.NotNil(t, resp.Login)
	require.NotNil(t, social.createdUser)
	require.NotNil(t, social.createdSocial)
	assert.Equal(t, "octo@example.com", social.createdUser.Username)
	assert.Equal(t, &email, social.createdUser.Email)
	assert.Equal(t, &nickname, social.createdUser.Nickname)
	assert.Equal(t, avatar, avatarSaver.gotURL)
	assert.Equal(t, "avatar/user/md5.jpg", *social.createdUser.AvatarUrl)
	assert.Equal(t, &blog, social.createdUser.Site)
	assert.Equal(t, roles.NormalRoleId, social.createdRoleID)
	assert.Equal(t, "github", social.createdSocial.Source)
	assert.Equal(t, "remote-123", social.createdSocial.UUID)
	assert.Equal(t, "access-token", social.createdSocial.AccessToken)
	assert.NotEmpty(t, resp.Login.AccessToken)
	assert.Equal(t, uint(7), resp.Login.User.ID)
}

func TestOAuthService_CallbackLoginIgnoresAvatarFailure(t *testing.T) {
	email := "octo@example.com"
	avatar := "https://avatars.example.com/octo.png"
	flow := &fakeFlowManager{
		callback: &domain.CallbackResult{
			Flow:  &domain.FlowContext{Action: domain.ActionLogin, Source: "github"},
			Token: &domain.TokenSet{AccessToken: "access-token"},
			Profile: &domain.Profile{
				Source:    "github",
				UUID:      "remote-123",
				Email:     &email,
				AvatarURL: &avatar,
			},
		},
	}
	social := &fakeSocialRepo{}
	user := &fakeUserRepo{roles: []string{roles.NormalRole}}
	avatarSaver := &fakeAvatarSaver{err: assert.AnError}
	svc := serviceoauth.NewOAuthService(flow, social, user, jwtpkg.NewManager("secret", 2, 168), nil, avatarSaver)

	resp, err := svc.Callback(context.Background(), "github", "code", "state")

	require.NoError(t, err)
	require.NotNil(t, resp.Login)
	require.NotNil(t, social.createdUser)
	assert.Nil(t, social.createdUser.AvatarUrl)
}

func TestOAuthService_CallbackLoginUsesExistingBinding(t *testing.T) {
	socialUser := &model.SocialUser{Base: model.Base{ID: 11}, Source: "github", UUID: "remote-123"}
	nickname := "Bound User"
	email := "bound@example.com"
	flow := &fakeFlowManager{
		callback: &domain.CallbackResult{
			Flow:    &domain.FlowContext{Action: domain.ActionLogin, Source: "github"},
			Token:   &domain.TokenSet{AccessToken: "new-access-token"},
			Profile: &domain.Profile{Source: "github", UUID: "remote-123"},
		},
	}
	social := &fakeSocialRepo{
		socialUser: socialUser,
		boundUser: &model.User{
			Base:     model.Base{ID: 7},
			Username: "bound",
			Email:    &email,
			Nickname: &nickname,
			Status:   1,
		},
	}
	user := &fakeUserRepo{roles: []string{roles.NormalRole}}
	svc := newTestService(flow, social, user)

	resp, err := svc.Callback(context.Background(), "github", "code", "state")

	require.NoError(t, err)
	require.NotNil(t, resp.Login)
	assert.Nil(t, social.createdUser)
	assert.Equal(t, uint(7), resp.Login.User.ID)
	assert.Equal(t, "bound", resp.Login.User.Username)
}

func TestOAuthService_CallbackBindRejectsAlreadyBoundIdentity(t *testing.T) {
	flow := &fakeFlowManager{
		callback: &domain.CallbackResult{
			Flow:    &domain.FlowContext{Action: domain.ActionBind, Source: "github", UserID: 7},
			Token:   &domain.TokenSet{AccessToken: "access-token"},
			Profile: &domain.Profile{Source: "github", UUID: "remote-123"},
		},
	}
	social := &fakeSocialRepo{
		socialUser: &model.SocialUser{Base: model.Base{ID: 11}, Source: "github", UUID: "remote-123"},
		boundUser:  &model.User{Base: model.Base{ID: 8}, Username: "other", Status: 1},
	}
	svc := newTestService(flow, social, &fakeUserRepo{})

	_, err := svc.Callback(context.Background(), "github", "code", "state")

	assert.ErrorIs(t, err, serviceoauth.ErrSocialIdentityBound)
}

func TestOAuthService_UnbindProtectsLastLoginMethod(t *testing.T) {
	user := &fakeUserRepo{user: &model.User{Base: model.Base{ID: 7}, Username: "oauth-only", Status: 1}}
	social := &fakeSocialRepo{bindingCount: 1}
	svc := newTestService(&fakeFlowManager{}, social, user)

	err := svc.Unbind(context.Background(), 7, "github")

	assert.ErrorIs(t, err, serviceoauth.ErrLastLoginMethod)
}

var _ = dto.OAuthCallbackResp{}
