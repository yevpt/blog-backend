package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	"github.com/vpt/blog-backend/internal/repository/user/mock"
	user "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/email"
)

// stubUserCacheService 最小实现 UserCacheService，用于测试 UserService 委托行为。
type stubUserCacheService struct {
	profile *dto.UserDetailResp
	err     error
}

func (s *stubUserCacheService) Get(_ context.Context, _ int64) (*dto.UserDetailResp, error) {
	return s.profile, s.err
}
func (s *stubUserCacheService) Set(_ context.Context, _ int64, _ *dto.UserDetailResp) error {
	return nil
}
func (s *stubUserCacheService) Invalidate(_ context.Context, _ int64) error { return nil }

func TestUserService_GetDetail_DelegatesToCache(t *testing.T) {
	nickname := "Alice"
	expected := &dto.UserDetailResp{
		ID:       7,
		Username: "alice",
		Nickname: &nickname,
		Roles:    []string{"ROLE_NORMAL", "ROLE_VIP"},
	}
	svc := user.NewUserService(&stubUserCacheService{profile: expected}, nil, nil, nil, nil)

	resp, err := svc.GetDetail(7)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint(7), resp.ID)
	assert.Equal(t, []string{"ROLE_NORMAL", "ROLE_VIP"}, resp.Roles)
}

func TestUserService_GetDetail_PropagatesNotFound(t *testing.T) {
	svc := user.NewUserService(&stubUserCacheService{err: user.ErrUserNotFound}, nil, nil, nil, nil)
	resp, err := svc.GetDetail(9)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, user.ErrUserNotFound)
}

type securityMailSender struct {
	sentTo      string
	sentCode    string
	sentPurpose email.Purpose
}

func (m *securityMailSender) SendVerificationCode(to, code string, purpose email.Purpose) error {
	m.sentTo = to
	m.sentCode = code
	m.sentPurpose = purpose
	return nil
}

func (m *securityMailSender) SendHTML(_ string, _ string, _ string, _ string) error {
	return nil
}

type securityCaptchaConsumer struct {
	token string
	ip    string
}

func (c *securityCaptchaConsumer) ConsumeRegistrationToken(token string, ip string) error {
	c.token = token
	c.ip = ip
	return nil
}

func newSecurityService(
	t *testing.T,
) (user.UserService, *mock.MockUserRepository, *redis.Client, *miniredis.Miniredis, *securityMailSender, *securityCaptchaConsumer) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mailer := &securityMailSender{}
	captcha := &securityCaptchaConsumer{}
	svc := user.NewUserService(nil, repo, nil, nil, nil, user.SecurityDeps{
		Redis:   rdb,
		Mailer:  mailer,
		Captcha: captcha,
	})
	return svc, repo, rdb, mr, mailer, captcha
}

func TestUserService_SendEmailCode_WritesScopedCodeAndSendsMail(t *testing.T) {
	svc, repo, rdb, mr, mailer, captcha := newSecurityService(t)
	defer mr.Close()

	repo.EXPECT().EmailInUseByOther("new@example.com", uint(7)).Return(false, nil)

	err := svc.SendEmailCode(7, "new@example.com", "captcha-token", "127.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, "new@example.com", mailer.sentTo)
	assert.Equal(t, "captcha-token", captcha.token)
	assert.Equal(t, "127.0.0.1", captcha.ip)
	code, redisErr := rdb.Get(context.Background(), "user:email:code:7:new@example.com").Result()
	require.NoError(t, redisErr)
	assert.Len(t, code, 6)
	assert.Equal(t, email.PurposeEmailBind, mailer.sentPurpose)
}

func TestUserService_UpdateEmail_MainConsumesCodeAndUpdatesUserEmail(t *testing.T) {
	svc, repo, rdb, mr, _, _ := newSecurityService(t)
	defer mr.Close()
	rdb.Set(context.Background(), "user:email:code:7:new@example.com", "123456", 0)

	repo.EXPECT().EmailInUseByOther("new@example.com", uint(7)).Return(false, nil)
	repo.EXPECT().Update(uint(7), gomock.Any()).DoAndReturn(func(_ uint, updates map[string]any) error {
		assert.Equal(t, "new@example.com", updates["email"])
		assert.IsType(t, time.Time{}, updates["email_verified_at"])
		return nil
	})

	err := svc.UpdateEmail(7, "main", "new@example.com", "123456")

	require.NoError(t, err)
	exists, redisErr := rdb.Exists(context.Background(), "user:email:code:7:new@example.com").Result()
	require.NoError(t, redisErr)
	assert.Equal(t, int64(0), exists)
}

func TestUserService_UpdateEmail_SubConsumesCodeAndUpdatesMeta(t *testing.T) {
	svc, repo, rdb, mr, _, _ := newSecurityService(t)
	defer mr.Close()
	rdb.Set(context.Background(), "user:email:code:7:sub@example.com", "123456", 0)

	repo.EXPECT().EmailInUseByOther("sub@example.com", uint(7)).Return(false, nil)
	repo.EXPECT().UpsertMeta(uint(7), gomock.Any()).DoAndReturn(func(_ uint, updates map[string]any) error {
		assert.Equal(t, "sub@example.com", updates["sub_email"])
		assert.IsType(t, time.Time{}, updates["sub_email_verified_at"])
		return nil
	})

	err := svc.UpdateEmail(7, "sub", "sub@example.com", "123456")

	require.NoError(t, err)
}

func TestUserService_SetInitialPassword_UsesCurrentEmailCode(t *testing.T) {
	svc, repo, rdb, mr, _, _ := newSecurityService(t)
	defer mr.Close()
	email := "oauth@example.com"
	rdb.Set(context.Background(), "user:email:code:7:oauth@example.com", "123456", 0)

	repo.EXPECT().FindByID(uint(7)).Return(&model.User{Email: &email}, nil)
	repo.EXPECT().UpdatePassword(uint(7), gomock.Any()).DoAndReturn(func(_ uint, hash string) error {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte("new-password"))
	})
	repo.EXPECT().Update(uint(7), gomock.Any()).DoAndReturn(func(_ uint, updates map[string]any) error {
		assert.IsType(t, time.Time{}, updates["email_verified_at"])
		return nil
	})

	err := svc.SetInitialPassword(7, "new-password", "123456")

	require.NoError(t, err)
}

func TestUserService_SetInitialPassword_RejectsExistingPassword(t *testing.T) {
	svc, repo, rdb, mr, _, _ := newSecurityService(t)
	defer mr.Close()
	email := "user@example.com"
	rdb.Set(context.Background(), "user:email:code:7:user@example.com", "123456", 0)

	repo.EXPECT().FindByID(uint(7)).Return(&model.User{Email: &email, PasswordSet: true}, nil)

	err := svc.SetInitialPassword(7, "new-password", "123456")

	assert.ErrorIs(t, err, user.ErrPasswordAlreadySet)
}

func TestUserService_ListLikedContent_MapsReplyAsCommentFilterWithContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	svc := user.NewUserService(nil, repo, nil, nil, nil)
	now := time.Date(2026, 6, 25, 10, 24, 0, 0, time.UTC)
	nickname := "阿澈"
	avatar := "avatars/a.png"

	repo.EXPECT().ListLikedContent(userrepo.LikedContentFilter{
		UserID:   7,
		Type:     dto.UserLikedContentFilterComment,
		Page:     1,
		PageSize: 20,
	}).Return(&userrepo.LikedContentPageResult{
		Total:    1,
		Page:     1,
		PageSize: 20,
		Items: []userrepo.LikedContentAggregate{
			{
				ID:      99,
				LikedAt: now,
				Kind:    userrepo.LikedContentKindReply,
				Filter:  userrepo.LikedContentFilterComment,
				Author: &model.User{
					Base:      model.Base{ID: 12},
					Username:  "ache",
					Nickname:  &nickname,
					AvatarUrl: &avatar,
				},
				Content: userrepo.LikedContentObject{
					ID:      88,
					Excerpt: "对，这里用乐观更新会更顺手",
				},
				ToUser: &model.User{
					Base:     model.Base{ID: 2},
					Username: "vpt",
					Nickname: ptrString("VPT"),
				},
				Parent: &userrepo.LikedContentObject{
					Kind:    userrepo.LikedContentKindComment,
					ID:      66,
					Excerpt: "点赞状态最好由服务端返回最终计数",
				},
				Root: &userrepo.LikedContentObject{
					Kind:  userrepo.LikedContentRootArticle,
					ID:    5,
					Title: ptrString("React Aria 组件实践"),
				},
			},
		},
	}, nil)
	repo.EXPECT().FindRolesByUserIDs([]uint{12}).Return(map[uint][]string{12: []string{"ROLE_VIP"}}, nil)

	resp, err := svc.ListLikedContent(7, dto.UserLikedContentListReq{
		Type:     dto.UserLikedContentFilterComment,
		Page:     1,
		PageSize: 200,
	})

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	item := resp.List[0]
	assert.Equal(t, dto.UserLikedContentKindReply, item.Kind)
	assert.Equal(t, dto.UserLikedContentFilterComment, item.Filter)
	assert.Equal(t, "阿澈", *item.Author.Nickname)
	assert.Equal(t, []string{"ROLE_VIP"}, item.Author.Roles)
	assert.Equal(t, "点赞状态最好由服务端返回最终计数", item.Parent.Excerpt)
	require.NotNil(t, item.ToUser)
	assert.Equal(t, uint(2), item.ToUser.ID)
	assert.Equal(t, "VPT", *item.ToUser.Nickname)
	assert.Equal(t, "article", item.Root.Kind)
	assert.Equal(t, "React Aria 组件实践", *item.Root.Title)
	assert.Equal(t, 20, resp.PageSize)
}

func TestUserService_CountLikedContent_ReturnsTotal(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)
	svc := user.NewUserService(nil, repo, nil, nil, nil)

	repo.EXPECT().CountLikedContent(uint(7)).Return(int64(12), nil)

	resp, err := svc.CountLikedContent(7)

	require.NoError(t, err)
	assert.Equal(t, int64(12), resp.Count)
}

func ptrString(value string) *string {
	return &value
}
