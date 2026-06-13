package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository/mock"
	authservice "github.com/vpt/blog-backend/internal/service/auth"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/roles"
)

// mockMailSender 测试用邮件发送 mock
type mockMailSender struct {
	err      error
	sentTo   string
	sentCode string
}

func (m *mockMailSender) SendVerificationCode(to, code string) error {
	m.sentTo = to
	m.sentCode = code
	return m.err
}

type mockCaptchaTokenConsumer struct {
	err           error
	consumedToken string
	consumedIP    string
}

func (m *mockCaptchaTokenConsumer) ConsumeRegistrationToken(token string, ip string) error {
	m.consumedToken = token
	m.consumedIP = ip
	return m.err
}

func setupService(t *testing.T) (authservice.AuthService, *mock.MockUserRepository, *redis.Client, *miniredis.Miniredis, *mockMailSender, *mockCaptchaTokenConsumer) {
	ctrl := gomock.NewController(t)
	repo := mock.NewMockUserRepository(ctrl)

	mr, err := miniredis.Run()
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	mailer := &mockMailSender{}
	captchaConsumer := &mockCaptchaTokenConsumer{}
	jwtMgr := jwtpkg.NewManager("secret", 2, 168)

	svc := authservice.NewAuthService(repo, jwtMgr, rdb, mailer, captchaConsumer, nil)
	return svc, repo, rdb, mr, mailer, captchaConsumer
}

func TestAuthService_SendCode_Success(t *testing.T) {
	svc, _, rdb, mr, mailer, captchaConsumer := setupService(t)
	defer mr.Close()

	err := svc.SendCode("user@example.com", "127.0.0.1", "captcha-token")
	require.NoError(t, err)

	// 验证码已写入 Redis
	val, redisErr := rdb.Get(context.Background(), "email:code:user@example.com").Result()
	require.NoError(t, redisErr)
	assert.Len(t, val, 6)
	assert.Equal(t, "user@example.com", mailer.sentTo)
	assert.Equal(t, "captcha-token", captchaConsumer.consumedToken)
	assert.Equal(t, "127.0.0.1", captchaConsumer.consumedIP)
}

func TestAuthService_SendCode_CooldownBlocks(t *testing.T) {
	svc, _, rdb, mr, _, _ := setupService(t)
	defer mr.Close()

	// 预写入冷却 key（TTL=0 表示永不过期，仅测试用）
	rdb.Set(context.Background(), "email:cd:user@example.com", 1, 0)

	err := svc.SendCode("user@example.com", "127.0.0.1", "captcha-token")
	assert.Error(t, err)
}

func TestAuthService_SendCode_InvalidCaptchaToken(t *testing.T) {
	svc, _, rdb, mr, mailer, captchaConsumer := setupService(t)
	defer mr.Close()
	captchaConsumer.err = errors.New("请先完成图形验证码")

	err := svc.SendCode("user@example.com", "127.0.0.1", "bad-token")

	assert.Error(t, err)
	assert.Empty(t, mailer.sentTo)
	exists, redisErr := rdb.Exists(context.Background(), "email:code:user@example.com").Result()
	require.NoError(t, redisErr)
	assert.Equal(t, int64(0), exists)
}

func TestAuthService_Register_Success(t *testing.T) {
	svc, repo, rdb, mr, _, _ := setupService(t)
	defer mr.Close()

	// 预写入验证码
	rdb.Set(context.Background(), "email:code:new@example.com", "123456", 0)

	repo.EXPECT().ExistsByEmail("new@example.com").Return(false, nil)
	repo.EXPECT().ExistsByNickname(gomock.Any()).Return(false, nil).AnyTimes()
	repo.EXPECT().Create(gomock.Any(), roles.NormalRoleId).Return(nil)

	nickname := "mynick"
	resp, err := svc.Register(&dto.RegisterReq{
		Email:    "new@example.com",
		Password: "password123",
		Code:     "123456",
		Nickname: &nickname,
	})
	require.NoError(t, err)
	assert.Equal(t, "new@example.com", resp.Username)
}

func TestAuthService_Register_WrongCode(t *testing.T) {
	svc, _, rdb, mr, _, _ := setupService(t)
	defer mr.Close()

	rdb.Set(context.Background(), "email:code:x@example.com", "999999", 0)

	_, err := svc.Register(&dto.RegisterReq{
		Email:    "x@example.com",
		Password: "password123",
		Code:     "111111",
	})
	assert.Error(t, err)
}

func TestAuthService_Login_Success(t *testing.T) {
	svc, repo, _, mr, _, _ := setupService(t)
	defer mr.Close()

	// 用 bcrypt 动态生成 hash，避免硬编码失效
	rawPwd := "password123"
	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(rawPwd), bcrypt.MinCost)
	hashedPwd := string(hashedBytes)

	email := "user@example.com"
	nickname := "Alice"
	gomock.InOrder(
		repo.EXPECT().FindByIdentifier("user@example.com").Return(&model.User{
			Username: email,
			Password: hashedPwd,
			Email:    &email,
			Nickname: &nickname,
			Status:   1,
		}, nil),
		repo.EXPECT().UpdateLastLoginAt(uint(0)).Return(nil),
		repo.EXPECT().FindRolesByUserID(uint(0)).Return([]string{"ROLE_NORMAL"}, nil),
	)

	resp, err := svc.Login(&dto.LoginReq{
		Identifier: "user@example.com",
		Password:   "password123",
	}, "127.0.0.1")
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	assert.Equal(t, 7200, resp.ExpiresIn)
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	svc, repo, _, mr, _, _ := setupService(t)
	defer mr.Close()

	rawPwd := "password123"
	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(rawPwd), bcrypt.MinCost)
	email := "user@example.com"
	repo.EXPECT().FindByIdentifier("user@example.com").Return(&model.User{
		Password: string(hashedBytes),
		Email:    &email,
		Status:   1,
	}, nil)

	_, err := svc.Login(&dto.LoginReq{
		Identifier: "user@example.com",
		Password:   "wrongpassword",
	}, "127.0.0.1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, authservice.ErrWrongPassword)
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	svc, repo, _, mr, _, _ := setupService(t)
	defer mr.Close()

	repo.EXPECT().FindByIdentifier("nobody").Return(nil, nil)

	_, err := svc.Login(&dto.LoginReq{
		Identifier: "nobody",
		Password:   "anypassword",
	}, "127.0.0.1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, authservice.ErrUserNotFound)
}

func TestAuthService_Refresh_Success(t *testing.T) {
	svc, _, _, mr, _, _ := setupService(t)
	defer mr.Close()

	jwtMgr := jwtpkg.NewManager("secret", 2, 168)
	refreshToken, _ := jwtMgr.GenerateRefresh(1)

	resp, err := svc.Refresh(refreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
}

func TestAuthService_Refresh_AccessTokenRejected(t *testing.T) {
	svc, _, _, mr, _, _ := setupService(t)
	defer mr.Close()

	jwtMgr := jwtpkg.NewManager("secret", 2, 168)
	// access token 不能用于 refresh
	accessToken, _ := jwtMgr.GenerateAccess(1)

	_, err := svc.Refresh(accessToken)
	assert.Error(t, err)
}
