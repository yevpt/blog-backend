package auth

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
	ErrInvalidCode       = errors.New("验证码无效或已过期")
	ErrEmailTaken        = errors.New("该邮箱已被注册")
	ErrUserNotFound      = errors.New("账号不存在")
	ErrWrongPassword     = errors.New("密码错误")
	ErrInvalidCredential = errors.New("账号或密码错误")
	ErrUserDisabled      = errors.New("账号已被禁用")
	ErrInvalidToken      = errors.New("token 无效或已过期")
	// ErrTooManyRequests 短期发送频率超限，区别于日频次耗尽的 ErrDailyLimitExceeded
	ErrTooManyRequests = errors.New("发送过于频繁，请稍后再试")
	// ErrDailyLimitExceeded 当日发送次数达到上限（7次），次日自动重置
	ErrDailyLimitExceeded = errors.New("今日发送次数已达上限")
	ErrNicknameGenFailed  = errors.New("昵称生成失败，请手动指定昵称")
)

// dummyHashForTimingProtection 用于用户不存在时执行无意义的 bcrypt 比对，消除响应时差。
// 包加载时预生成一次，避免每次请求临时生成带来额外开销。
var dummyHashForTimingProtection, _ = bcrypt.GenerateFromPassword(
	[]byte("dummy-timing-protection-password"), bcrypt.DefaultCost,
)

// AuthService 认证业务接口，涵盖验证码发送、注册、登录、token 刷新全链路
type AuthService interface {
	// SendCode 向邮箱发送验证码，内置三层频率控制（冷却 / 10分钟 / 日限）
	SendCode(email string, ip string, captchaToken string) error
	// Register 校验验证码并创建用户，验证码一次性消费，邮箱全局唯一
	Register(req *dto.RegisterReq) (*dto.UserResp, error)
	// Login 三合一登录（username / email / phone），用户不存在时仍执行 bcrypt 防止时序攻击
	Login(req *dto.LoginReq, ip string) (*dto.LoginResp, error)
	// Refresh 用 refresh token 同时换发新的 access + refresh token（token rotation）
	Refresh(refreshToken string) (*dto.TokenResp, error)
}

type authService struct {
	repo            repository.UserRepository
	jwt             *jwtpkg.Manager
	rdb             *redis.Client
	mailer          email.MailSender
	captchaConsumer CaptchaTokenConsumer
}

// CaptchaTokenConsumer 消费注册图形验证码票据，避免 auth 直接了解 captcha 内部存储细节。
type CaptchaTokenConsumer interface {
	ConsumeRegistrationToken(token string, ip string) error
}

func NewAuthService(
	repo repository.UserRepository,
	jwt *jwtpkg.Manager,
	rdb *redis.Client,
	mailer email.MailSender,
	captchaConsumer CaptchaTokenConsumer,
) AuthService {
	return &authService{
		repo:            repo,
		jwt:             jwt,
		rdb:             rdb,
		mailer:          mailer,
		captchaConsumer: captchaConsumer,
	}
}

func (s *authService) SendCode(to string, ip string, captchaToken string) error {
	ctx := context.Background()

	// 冷却检查优先，避免后续 Incr 在冷却期内重复计数
	cdKey := fmt.Sprintf("email:cd:%s", to)
	if n, _ := s.rdb.Exists(ctx, cdKey).Result(); n > 0 {
		return ErrTooManyRequests
	}

	// 发送邮件验证码前必须消费一次性图形验证码票据，防止绕过前端直接刷邮件接口
	if err := s.captchaConsumer.ConsumeRegistrationToken(captchaToken, ip); err != nil {
		return err
	}

	// 10分钟内发送次数检查（上限2次），首次 Incr 后立即设过期时间，避免 key 永久存在
	key10m := fmt.Sprintf("email:10m:%s", to)
	c10m, _ := s.rdb.Incr(ctx, key10m).Result()
	if c10m == 1 {
		s.rdb.Expire(ctx, key10m, 10*time.Minute)
	}
	if c10m > 2 {
		return ErrTooManyRequests
	}

	// 当日发送次数检查（上限7次），键不存在时首次计数后设24小时过期，次日自动重置
	key1d := fmt.Sprintf("email:1d:%s", to)
	c1d, _ := s.rdb.Incr(ctx, key1d).Result()
	if c1d == 1 {
		s.rdb.Expire(ctx, key1d, 24*time.Hour)
	}
	if c1d > 7 {
		return ErrDailyLimitExceeded
	}

	// 所有频率限制通过，生成6位密码学安全随机验证码
	code, err := generateNumericCode(6)
	if err != nil {
		return err
	}

	// 写入验证码（5分钟有效）和冷却标记（60秒），两个 key 独立管理生命周期
	codeKey := fmt.Sprintf("email:code:%s", to)
	s.rdb.Set(ctx, codeKey, code, 5*time.Minute)
	s.rdb.Set(ctx, cdKey, 1, 60*time.Second)

	// 发送验证码邮件，SMTP 失败时错误直接返回给调用方，不做重试
	return s.mailer.SendVerificationCode(to, code)
}

func (s *authService) Register(req *dto.RegisterReq) (*dto.UserResp, error) {
	ctx := context.Background()

	// 从 Redis 读取存储的验证码并与用户提交值对比
	codeKey := fmt.Sprintf("email:code:%s", req.Email)
	stored, err := s.rdb.Get(ctx, codeKey).Result()
	if err != nil || stored != req.Code {
		return nil, ErrInvalidCode
	}
	// 验证码比对成功后立即删除，确保一次性语义
	s.rdb.Del(ctx, codeKey)

	// 检查邮箱是否已被其他账号占用
	taken, err := s.repo.ExistsByEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrEmailTaken
	}

	// 解析昵称：用户填写则直接用，未填写则以邮箱前缀+随机串自动生成
	nickname, err := s.resolveNickname(req.Nickname, req.Email)
	if err != nil {
		return nil, err
	}

	// cost=12 高于默认值 10，在安全性和性能间取平衡
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		// 邮箱注册时 username 初始值等于 email，用户后续可自行修改
		Username: req.Email,
		Password: string(hash),
		Email:    &req.Email,
		Nickname: &nickname,
		Status:   1,
	}

	// 在事务中同时写入用户记录和角色关联，保证两张表数据一致
	if err := s.repo.Create(user, roles.NormalRoleId); err != nil {
		return nil, err
	}

	// 组装响应 DTO，不暴露密码等敏感字段
	return &dto.UserResp{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Nickname: user.Nickname,
	}, nil
}

func (s *authService) Login(req *dto.LoginReq, ip string) (*dto.LoginResp, error) {
	// 支持 username / email / phone 三合一查询用户
	user, err := s.repo.FindByIdentifier(req.Identifier)
	if err != nil {
		return nil, err
	}

	// 用户不存在时仍执行 bcrypt，使不存在与密码错误两种情况的响应时间尽量一致
	if user == nil {
		bcrypt.CompareHashAndPassword(dummyHashForTimingProtection, []byte(req.Password))
		return nil, ErrUserNotFound
	}

	// 用户存在时比对密码哈希，不匹配则拒绝
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrWrongPassword
	}

	// 密码正确后再检查账号状态，避免通过错误类型泄露账号是否存在
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	// 查询用户所有角色名称，用于写入 JWT claims
	userRoles, err := s.repo.FindRolesByUserID(user.ID)
	if err != nil {
		return nil, err
	}

	// 签发 access + refresh 双 token，access 短期用于接口访问，refresh 长期用于续签
	userId := int64(user.ID)
	accessToken, err := s.jwt.GenerateAccess(userId, user.Username, userRoles)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.jwt.GenerateRefresh(userId, user.Username, userRoles)
	if err != nil {
		return nil, err
	}

	// 异步写入，不让非关键操作拖慢登录响应
	go func() { s.repo.UpdateLastLoginAt(user.ID) }()

	// 组装登录响应，含双 token 和用户基本信息
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
	// 解析并验证 token 签名与过期时间
	claims, err := s.jwt.Parse(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}
	// 拒绝用 access token 来换发，只允许 refresh token 进入此接口
	if claims.TokenType != "refresh" {
		return nil, ErrInvalidToken
	}

	// 用原 claims 中的用户信息签发新的双 token（token rotation）
	newAccess, err := s.jwt.GenerateAccess(claims.UserId, claims.Username, claims.Roles)
	if err != nil {
		return nil, err
	}
	newRefresh, err := s.jwt.GenerateRefresh(claims.UserId, claims.Username, claims.Roles)
	if err != nil {
		return nil, err
	}

	// 返回新双 token，旧 refresh token 从此不再有效（无状态 rotation，旧 token 自然过期）
	return &dto.TokenResp{
		AccessToken:  newAccess,
		RefreshToken: newRefresh,
		ExpiresIn:    7200,
	}, nil
}

// resolveNickname 优先使用用户指定昵称；未指定时以邮箱前缀（≤6字符）+ 4位随机串自动生成，
// 最多重试 10 次避免极端碰撞情况。
func (s *authService) resolveNickname(nickname *string, emailAddr string) (string, error) {
	// 用户已填写昵称时直接使用，不走自动生成流程
	if nickname != nil && strings.TrimSpace(*nickname) != "" {
		return *nickname, nil
	}

	// 以 @ 前的邮箱前缀作为自动昵称的可读部分
	prefix := emailAddr
	if idx := strings.Index(emailAddr, "@"); idx > 0 {
		prefix = emailAddr[:idx]
	}
	// 截断至6字符，避免昵称过长影响展示
	if len(prefix) > 6 {
		prefix = prefix[:6]
	}

	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	// 最多重试10次避免极端碰撞（碰撞概率极低，重试上限作为兜底保险）
	for i := 0; i < 10; i++ {
		// 生成4位随机字母数字后缀，拼接 prefix 构成候选昵称
		suffix := make([]byte, 4)
		for j := range suffix {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return "", err
			}
			suffix[j] = charset[n.Int64()]
		}
		candidate := prefix + string(suffix)
		// 检查候选昵称是否已被占用，未占用则直接返回
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

// generateNumericCode 使用 crypto/rand 生成指定位数的纯数字验证码，保证密码学随机性
func generateNumericCode(length int) (string, error) {
	digits := make([]byte, length)
	for i := range digits {
		// 从 [0, 10) 范围内取密码学安全随机整数，保证不可预测性
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		// 将整数转换为对应的 ASCII 数字字符（'0'=48, '9'=57）
		digits[i] = byte('0') + byte(n.Int64())
	}
	return string(digits), nil
}
