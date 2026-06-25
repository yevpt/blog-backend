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
	userrepo "github.com/vpt/blog-backend/internal/repository/user"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/email"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/roles"
	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	ErrInvalidCode       = errors.New("验证码无效或已过期")
	ErrEmailTaken        = errors.New("该邮箱已被注册")
	ErrUserNotFound      = errors.New("账号不存在")
	ErrWrongPassword     = errors.New("密码错误")
	ErrInvalidCredential = errors.New("账号或密码错误")
	ErrUserDisabled      = errors.New("账号已被禁用")
	ErrAdminRequired     = errors.New("仅管理员可登录管理后台")
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
	// Register 校验验证码并创建用户，验证码一次性消费，邮箱全局唯一；成功后签发登录 token
	Register(req *dto.RegisterReq, avatar *dto.UploadedImageFile) (*dto.LoginResp, error)
	// Login 三合一登录（username / email / phone），用户不存在时仍执行 bcrypt 防止时序攻击
	Login(req *dto.LoginReq, ip string) (*dto.LoginResp, error)
	// AdminLogin 管理后台登录，仅允许 username + password，且用户必须持有管理员角色
	AdminLogin(req *dto.AdminLoginReq, ip string) (*dto.LoginResp, error)
	// Refresh 用 refresh token 同时换发新的 access + refresh token（token rotation）
	Refresh(refreshToken string) (*dto.TokenResp, error)
	// SendPasswordResetCode 发送忘记密码验证码，不向调用方暴露邮箱是否存在。
	SendPasswordResetCode(email string, ip string, captchaToken string) error
	// ResetPassword 使用邮箱验证码重置登录密码。
	ResetPassword(req *dto.PasswordResetReq) error
}

type authService struct {
	repo            userrepo.UserRepository
	jwt             *jwtpkg.Manager
	rdb             *redis.Client
	mailer          email.MailSender
	captchaConsumer CaptchaTokenConsumer
	cache           userservice.UserCacheService
	avatar          userservice.AvatarUploader
	store           storage.ObjectStore
	resolver        storage.ObjectURLResolver
}

// CaptchaTokenConsumer 消费注册图形验证码票据，避免 auth 直接了解 captcha 内部存储细节。
type CaptchaTokenConsumer interface {
	ConsumeRegistrationToken(token string, ip string) error
}

func NewAuthService(
	repo userrepo.UserRepository,
	jwt *jwtpkg.Manager,
	rdb *redis.Client,
	mailer email.MailSender,
	captchaConsumer CaptchaTokenConsumer,
	cache userservice.UserCacheService,
	avatar userservice.AvatarUploader,
	store storage.ObjectStore,
) AuthService {
	return &authService{
		repo:            repo,
		jwt:             jwt,
		rdb:             rdb,
		mailer:          mailer,
		captchaConsumer: captchaConsumer,
		cache:           cache,
		avatar:          avatar,
		store:           store,
		resolver:        store,
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

func (s *authService) SendPasswordResetCode(to string, ip string, captchaToken string) error {
	ctx := context.Background()
	emailAddr := normalizeEmail(to)

	cdKey := fmt.Sprintf("password:reset:cd:%s", emailAddr)
	if n, _ := s.rdb.Exists(ctx, cdKey).Result(); n > 0 {
		return ErrTooManyRequests
	}

	if err := s.captchaConsumer.ConsumeRegistrationToken(captchaToken, ip); err != nil {
		return err
	}

	key10m := fmt.Sprintf("password:reset:10m:%s", emailAddr)
	c10m, _ := s.rdb.Incr(ctx, key10m).Result()
	if c10m == 1 {
		s.rdb.Expire(ctx, key10m, 10*time.Minute)
	}
	if c10m > 2 {
		return ErrTooManyRequests
	}

	s.rdb.Set(ctx, cdKey, 1, 60*time.Second)

	user, err := s.repo.FindByEmail(emailAddr)
	if err != nil {
		return err
	}
	if user == nil {
		return nil
	}
	if user.EmailVerifiedAt == nil {
		return nil
	}

	code, err := generateNumericCode(6)
	if err != nil {
		return err
	}

	codeKey := passwordResetCodeKey(emailAddr)
	s.rdb.Set(ctx, codeKey, code, 5*time.Minute)
	return s.mailer.SendVerificationCode(emailAddr, code)
}

func (s *authService) ResetPassword(req *dto.PasswordResetReq) error {
	ctx := context.Background()
	emailAddr := normalizeEmail(req.Email)
	codeKey := passwordResetCodeKey(emailAddr)

	stored, err := s.rdb.Get(ctx, codeKey).Result()
	if err != nil || stored != req.Code {
		return ErrInvalidCode
	}

	user, err := s.repo.FindByEmail(emailAddr)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrInvalidCode
	}
	if user.EmailVerifiedAt == nil {
		return ErrInvalidCode
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(user.ID, string(hash)); err != nil {
		return err
	}

	s.rdb.Del(ctx, codeKey)
	if s.cache != nil {
		_ = s.cache.Invalidate(ctx, int64(user.ID))
	}
	return nil
}

func (s *authService) Register(req *dto.RegisterReq, avatar *dto.UploadedImageFile) (*dto.LoginResp, error) {
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

	var avatarKey *string
	var avatarCreated bool
	if avatar != nil && len(avatar.Data) > 0 {
		if s.avatar == nil {
			return nil, fmt.Errorf("头像上传不可用")
		}
		saved, saveErr := s.avatar.SaveUploadedAvatar(ctx, avatar.Name, avatar.Data)
		if saveErr != nil {
			return nil, saveErr
		}
		avatarKey = &saved.ObjectKey
		avatarCreated = saved.Created
	}

	// cost=12 高于默认值 10，在安全性和性能间取平衡
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		if avatarCreated && avatarKey != nil {
			_ = s.store.DeleteObject(ctx, *avatarKey)
		}
		return nil, err
	}

	user := &model.User{
		// 邮箱注册时 username 初始值等于 email，用户后续可自行修改
		Username:        req.Email,
		Password:        string(hash),
		PasswordSet:     true,
		Email:           &req.Email,
		EmailVerifiedAt: ptrTime(time.Now()),
		Nickname:        &nickname,
		AvatarUrl:       avatarKey,
		Status:          1,
	}

	// 在事务中同时写入用户记录和角色关联，保证两张表数据一致
	if err := s.repo.Create(user, roles.NormalRoleId); err != nil {
		if avatarCreated && avatarKey != nil && s.store != nil {
			_ = s.store.DeleteObject(ctx, *avatarKey)
		}
		return nil, err
	}

	return s.issueLoginResp(user)
}

func (s *authService) issueLoginResp(user *model.User) (*dto.LoginResp, error) {
	userId := int64(user.ID)
	accessToken, err := s.jwt.GenerateAccess(userId)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.jwt.GenerateRefresh(userId)
	if err != nil {
		return nil, err
	}

	_ = s.repo.UpdateLastLoginAt(user.ID)

	if s.cache != nil {
		go func() {
			_ = s.cache.Invalidate(context.Background(), userId)
		}()
	}

	userRoles, err := s.repo.FindRolesByUserID(user.ID)
	if err != nil {
		return nil, err
	}

	return &dto.LoginResp{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.jwt.AccessExpiresInSeconds(),
		User: dto.UserResp{
			ID:            user.ID,
			Username:      user.Username,
			Email:         user.Email,
			EmailVerified: user.EmailVerifiedAt != nil,
			Nickname:      user.Nickname,
			AvatarUrl:     storage.ResolvePtrURL(s.resolver, user.AvatarUrl),
			Roles:         userRoles,
		},
	}, nil
}

func passwordResetCodeKey(emailAddr string) string {
	return fmt.Sprintf("password:reset:code:%s", emailAddr)
}

func normalizeEmail(emailAddr string) string {
	return strings.ToLower(strings.TrimSpace(emailAddr))
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

	return s.issueLoginResp(user)
}

func (s *authService) AdminLogin(req *dto.AdminLoginReq, ip string) (*dto.LoginResp, error) {
	// 管理后台入口只按 username 查询，不接受邮箱或手机号作为登录标识。
	user, err := s.repo.FindByUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		bcrypt.CompareHashAndPassword(dummyHashForTimingProtection, []byte(req.Password))
		return nil, ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrWrongPassword
	}
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	// 先读取角色并校验管理员权限，非管理员不签发管理后台登录 token。
	userRoles, err := s.repo.FindRolesByUserID(user.ID)
	if err != nil {
		return nil, err
	}
	if !roles.HasPermission(userRoles, roles.AdminRole) {
		return nil, ErrAdminRequired
	}

	userId := int64(user.ID)
	accessToken, err := s.jwt.GenerateAccess(userId)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.jwt.GenerateRefresh(userId)
	if err != nil {
		return nil, err
	}

	_ = s.repo.UpdateLastLoginAt(user.ID)
	if s.cache != nil {
		go func() {
			_ = s.cache.Invalidate(context.Background(), userId)
		}()
	}

	return &dto.LoginResp{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.jwt.AccessExpiresInSeconds(),
		User: dto.UserResp{
			ID:            user.ID,
			Username:      user.Username,
			Email:         user.Email,
			EmailVerified: user.EmailVerifiedAt != nil,
			Nickname:      user.Nickname,
			Roles:         userRoles,
		},
	}, nil
}

func ptrTime(t time.Time) *time.Time {
	return &t
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

	// 验证用户仍然有效，防止被禁用的用户通过 refresh token 续签
	if s.cache != nil {
		detail, cacheErr := s.cache.Get(context.Background(), claims.UserId)
		if cacheErr != nil || detail == nil || detail.Status != 1 {
			return nil, ErrInvalidToken
		}
	}

	// 用 userId 签发新的双 token（token rotation），角色由 Redis 缓存动态加载
	newAccess, err := s.jwt.GenerateAccess(claims.UserId)
	if err != nil {
		return nil, err
	}
	newRefresh, err := s.jwt.GenerateRefresh(claims.UserId)
	if err != nil {
		return nil, err
	}

	// 返回新双 token；当前 refresh token 是无状态 JWT，旧 token 会按自身 exp 自然过期。
	return &dto.TokenResp{
		AccessToken:  newAccess,
		RefreshToken: newRefresh,
		ExpiresIn:    s.jwt.AccessExpiresInSeconds(),
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
