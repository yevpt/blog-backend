package user

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

	"github.com/vpt/blog-backend/pkg/email"
)

var (
	ErrEmailTaken         = errors.New("该邮箱已被其他账号使用")
	ErrInvalidEmailCode   = errors.New("验证码无效或已过期")
	ErrEmailRequired      = errors.New("当前账号未绑定邮箱")
	ErrSecurityDisabled   = errors.New("账号安全服务不可用")
	ErrPasswordAlreadySet = errors.New("当前账号已设置密码，请使用修改密码")
)

// SecurityDeps 是账号安全功能依赖，保持邮件、Redis 和验证码能力由外部注入。
type SecurityDeps struct {
	Redis   *redis.Client
	Mailer  email.MailSender
	Captcha CaptchaTokenConsumer
}

// CaptchaTokenConsumer 消费一次性图形验证码票据。
type CaptchaTokenConsumer interface {
	ConsumeRegistrationToken(token string, ip string) error
}

func (s *userService) SendEmailCode(userID uint, emailAddr, captchaToken, ip string) error {
	if s.security.Redis == nil || s.security.Mailer == nil || s.security.Captcha == nil {
		return ErrSecurityDisabled
	}

	normalized := normalizeEmail(emailAddr)
	taken, err := s.repo.EmailInUseByOther(normalized, userID)
	if err != nil {
		return err
	}
	if taken {
		return ErrEmailTaken
	}

	if err := s.security.Captcha.ConsumeRegistrationToken(captchaToken, ip); err != nil {
		return err
	}
	if err := s.ensureEmailCodeQuota(normalized); err != nil {
		return err
	}

	code, err := generateSecurityEmailCode()
	if err != nil {
		return err
	}

	ctx := context.Background()
	s.security.Redis.Set(ctx, emailCodeKey(userID, normalized), code, 5*time.Minute)
	return s.security.Mailer.SendVerificationCode(normalized, code)
}

func (s *userService) UpdateEmail(userID uint, target, emailAddr, code string) error {
	if s.security.Redis == nil {
		return ErrSecurityDisabled
	}

	normalized := normalizeEmail(emailAddr)
	if err := s.verifyEmailCode(userID, normalized, code); err != nil {
		return err
	}
	taken, err := s.repo.EmailInUseByOther(normalized, userID)
	if err != nil {
		return err
	}
	if taken {
		return ErrEmailTaken
	}

	switch target {
	case "main":
		err = s.repo.Update(userID, map[string]any{"email": normalized})
	case "sub":
		err = s.repo.UpsertMeta(userID, map[string]any{"sub_email": normalized})
	default:
		return fmt.Errorf("邮箱目标无效")
	}
	if err != nil {
		return err
	}

	s.consumeEmailCode(userID, normalized)
	s.invalidateUserCache(userID)
	return nil
}

func (s *userService) SetInitialPassword(userID uint, newPwd, code string) error {
	if s.security.Redis == nil {
		return ErrSecurityDisabled
	}

	user, err := s.repo.FindByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}
	if user.PasswordSet {
		return ErrPasswordAlreadySet
	}
	if user.Email == nil || normalizeEmail(*user.Email) == "" {
		return ErrEmailRequired
	}

	emailAddr := normalizeEmail(*user.Email)
	if err := s.verifyEmailCode(userID, emailAddr, code); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(userID, string(hash)); err != nil {
		return err
	}

	s.consumeEmailCode(userID, emailAddr)
	s.invalidateUserCache(userID)
	return nil
}

func (s *userService) verifyEmailCode(userID uint, emailAddr, code string) error {
	stored, err := s.security.Redis.Get(context.Background(), emailCodeKey(userID, emailAddr)).Result()
	if err != nil || stored != code {
		return ErrInvalidEmailCode
	}
	return nil
}

func (s *userService) consumeEmailCode(userID uint, emailAddr string) {
	s.security.Redis.Del(context.Background(), emailCodeKey(userID, emailAddr))
}

func (s *userService) ensureEmailCodeQuota(emailAddr string) error {
	ctx := context.Background()
	cdKey := fmt.Sprintf("user:email:cd:%s", emailAddr)
	if n, _ := s.security.Redis.Exists(ctx, cdKey).Result(); n > 0 {
		return ErrTooManyEmailCodeRequests
	}

	countKey := fmt.Sprintf("user:email:10m:%s", emailAddr)
	count, _ := s.security.Redis.Incr(ctx, countKey).Result()
	if count == 1 {
		s.security.Redis.Expire(ctx, countKey, 10*time.Minute)
	}
	if count > 2 {
		return ErrTooManyEmailCodeRequests
	}

	s.security.Redis.Set(ctx, cdKey, 1, 60*time.Second)
	return nil
}

func emailCodeKey(userID uint, emailAddr string) string {
	return fmt.Sprintf("user:email:code:%d:%s", userID, emailAddr)
}

func normalizeEmail(emailAddr string) string {
	return strings.ToLower(strings.TrimSpace(emailAddr))
}

func generateSecurityEmailCode() (string, error) {
	var b strings.Builder
	b.Grow(6)
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + n.Int64()))
	}
	return b.String(), nil
}

func (s *userService) invalidateUserCache(userID uint) {
	if s.cache != nil {
		_ = s.cache.Invalidate(context.Background(), int64(userID))
	}
}

var ErrTooManyEmailCodeRequests = errors.New("发送过于频繁，请稍后再试")
