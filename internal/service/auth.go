package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/pkg/email"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/roles"
)

var (
	ErrInvalidCode        = errors.New("验证码无效或已过期")
	ErrEmailTaken         = errors.New("该邮箱已被注册")
	ErrInvalidCredential  = errors.New("账号或密码错误")
	ErrUserDisabled       = errors.New("账号已被禁用")
	ErrInvalidToken       = errors.New("token 无效或已过期")
	ErrTooManyRequests    = errors.New("发送过于频繁，请稍后再试")
	ErrDailyLimitExceeded = errors.New("今日发送次数已达上限")
	ErrNicknameGenFailed  = errors.New("昵称生成失败，请手动指定昵称")
)

// 预生成的 dummy hash，用于用户不存在时的时序攻击防护
// 在包加载时计算一次，避免每次登录都生成
var dummyHashForTimingProtection, _ = bcrypt.GenerateFromPassword(
	[]byte("dummy-timing-protection-password"), bcrypt.DefaultCost,
)

// AuthService 认证业务接口
type AuthService interface {
	SendCode(email string, ip string) error
	Register(req *dto.RegisterReq) (*dto.UserResp, error)
	Login(req *dto.LoginReq, ip string) (*dto.LoginResp, error)
	Refresh(refreshToken string) (*dto.TokenResp, error)
}

type authService struct {
	repo   repository.UserRepository
	jwt    *jwtpkg.Manager
	rdb    *redis.Client
	mailer email.MailSender
}

func NewAuthService(
	repo repository.UserRepository,
	jwt *jwtpkg.Manager,
	rdb *redis.Client,
	mailer email.MailSender,
) AuthService {
	return &authService{repo: repo, jwt: jwt, rdb: rdb, mailer: mailer}
}

func (s *authService) SendCode(to string, ip string) error {
	ctx := context.Background()

	// 邮箱维度三层频率控制
	cdKey := fmt.Sprintf("email:cd:%s", to)
	if n, _ := s.rdb.Exists(ctx, cdKey).Result(); n > 0 {
		return ErrTooManyRequests
	}

	key10m := fmt.Sprintf("email:10m:%s", to)
	c10m, _ := s.rdb.Incr(ctx, key10m).Result()
	if c10m == 1 {
		s.rdb.Expire(ctx, key10m, 10*time.Minute)
	}
	if c10m > 2 {
		return ErrTooManyRequests
	}

	key1d := fmt.Sprintf("email:1d:%s", to)
	c1d, _ := s.rdb.Incr(ctx, key1d).Result()
	if c1d == 1 {
		s.rdb.Expire(ctx, key1d, 24*time.Hour)
	}
	if c1d > 7 {
		return ErrDailyLimitExceeded
	}

	// 生成 6 位数字验证码
	code, err := generateNumericCode(6)
	if err != nil {
		return err
	}

	// 存入 Redis，TTL=5min
	codeKey := fmt.Sprintf("email:code:%s", to)
	s.rdb.Set(ctx, codeKey, code, 5*time.Minute)

	// 设置冷却（60s 内不能重发）
	s.rdb.Set(ctx, cdKey, 1, 60*time.Second)

	return s.mailer.SendVerificationCode(to, code)
}

func (s *authService) Register(req *dto.RegisterReq) (*dto.UserResp, error) {
	ctx := context.Background()

	// 校验验证码（比对后删除，一次性）
	codeKey := fmt.Sprintf("email:code:%s", req.Email)
	stored, err := s.rdb.Get(ctx, codeKey).Result()
	if err != nil || stored != req.Code {
		return nil, ErrInvalidCode
	}
	s.rdb.Del(ctx, codeKey)

	// 检查邮箱唯一性
	taken, err := s.repo.ExistsByEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrEmailTaken
	}

	// 处理 nickname
	nickname, err := s.resolveNickname(req.Nickname, req.Email)
	if err != nil {
		return nil, err
	}

	// bcrypt hash 密码（cost=12）
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username: req.Email, // 邮箱注册时 username 初始值为 email，后续可修改
		Password: string(hash),
		Email:    &req.Email,
		Nickname: &nickname,
		Status:   1,
	}

	if err := s.repo.Create(user, roles.NormalRoleId); err != nil {
		return nil, err
	}

	return &dto.UserResp{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Nickname: user.Nickname,
	}, nil
}

func (s *authService) Login(req *dto.LoginReq, ip string) (*dto.LoginResp, error) {
	user, err := s.repo.FindByIdentifier(req.Identifier)
	if err != nil {
		return nil, err
	}

	// 用户不存在时仍执行 bcrypt 比对，防止时序攻击
	if user == nil {
		bcrypt.CompareHashAndPassword(dummyHashForTimingProtection, []byte(req.Password))
		return nil, ErrInvalidCredential
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredential
	}

	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	userRoles, err := s.repo.FindRolesByUserID(user.ID)
	if err != nil {
		return nil, err
	}

	userId := int64(user.ID)
	accessToken, err := s.jwt.GenerateAccess(userId, user.Username, userRoles)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.jwt.GenerateRefresh(userId, user.Username, userRoles)
	if err != nil {
		return nil, err
	}

	// 异步更新最后登录时间，不阻塞响应
	go func() { s.repo.UpdateLastLoginAt(user.ID) }()

	return &dto.LoginResp{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    7200,
		User: dto.UserResp{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Nickname: user.Nickname,
			Roles:    userRoles,
		},
	}, nil
}

func (s *authService) Refresh(refreshToken string) (*dto.TokenResp, error) {
	claims, err := s.jwt.Parse(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if claims.TokenType != "refresh" {
		return nil, ErrInvalidToken
	}

	newAccess, err := s.jwt.GenerateAccess(claims.UserId, claims.Username, claims.Roles)
	if err != nil {
		return nil, err
	}
	newRefresh, err := s.jwt.GenerateRefresh(claims.UserId, claims.Username, claims.Roles)
	if err != nil {
		return nil, err
	}

	return &dto.TokenResp{
		AccessToken:  newAccess,
		RefreshToken: newRefresh,
		ExpiresIn:    7200,
	}, nil
}

// resolveNickname 处理昵称：有传则用传入的，否则自动生成（邮箱前缀 ≤6 字符 + 4 位随机字符）
func (s *authService) resolveNickname(nickname *string, emailAddr string) (string, error) {
	if nickname != nil && strings.TrimSpace(*nickname) != "" {
		return *nickname, nil
	}

	prefix := emailAddr
	if idx := strings.Index(emailAddr, "@"); idx > 0 {
		prefix = emailAddr[:idx]
	}
	if len(prefix) > 6 {
		prefix = prefix[:6]
	}

	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	for i := 0; i < 10; i++ {
		suffix := make([]byte, 4)
		for j := range suffix {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return "", err
			}
			suffix[j] = charset[n.Int64()]
		}
		candidate := prefix + string(suffix)
		exists, err := s.repo.ExistsByNickname(candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", ErrNicknameGenFailed
}

// generateNumericCode 生成指定位数的数字验证码
func generateNumericCode(length int) (string, error) {
	digits := make([]byte, length)
	for i := range digits {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		digits[i] = byte('0') + byte(n.Int64())
	}
	return string(digits), nil
}
