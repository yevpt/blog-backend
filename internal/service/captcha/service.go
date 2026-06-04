package captcha

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/wenlng/go-captcha/v2/slide"

	"github.com/vpt/blog-backend/internal/dto"
)

const (
	challengeTTL     = 5 * time.Minute
	tokenTTL         = 3 * time.Minute
	verifyPadding    = 6
	randomTokenBytes = 24
)

var (
	ErrInvalidCaptcha      = errors.New("图形验证码无效或已过期")
	ErrInvalidCaptchaToken = errors.New("请先完成图形验证码")
)

// Service GoCaptcha 业务接口，负责注册场景的挑战生成、校验和一次性票据消费。
type Service interface {
	GenerateRegistrationChallenge() (*dto.CaptchaChallengeResp, error)
	VerifyRegistrationChallenge(req *dto.CaptchaVerifyReq, ip string) (*dto.CaptchaVerifyResp, error)
	ConsumeRegistrationToken(token string, ip string) error
}

type service struct {
	rdb       *redis.Client
	generator slideGenerator
}

// NewService 创建 GoCaptcha service，启动时加载官方内嵌资源。
func NewService(rdb *redis.Client) (Service, error) {
	generator, err := newGoCaptchaSlideGenerator()
	if err != nil {
		return nil, err
	}

	return newServiceWithGenerator(rdb, generator), nil
}

func newServiceWithGenerator(rdb *redis.Client, generator slideGenerator) *service {
	return &service{rdb: rdb, generator: generator}
}

func (s *service) GenerateRegistrationChallenge() (*dto.CaptchaChallengeResp, error) {
	ctx := context.Background()

	// 生成图片和后端私有答案，响应只暴露渲染所需字段。
	challenge, err := s.generator.Generate()
	if err != nil {
		return nil, err
	}

	challengeID, err := randomHex(randomTokenBytes)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(challengePayload{X: challenge.X, Y: challenge.Y})
	if err != nil {
		return nil, err
	}

	if err := s.rdb.Set(ctx, challengeKey(challengeID), payload, challengeTTL).Err(); err != nil {
		return nil, err
	}

	return &dto.CaptchaChallengeResp{
		ChallengeID: challengeID,
		MasterImage: challenge.MasterImage,
		TileImage:   challenge.TileImage,
		TileX:       challenge.TileX,
		TileY:       challenge.TileY,
		TileWidth:   challenge.Width,
		TileHeight:  challenge.Height,
		ImageWidth:  challenge.ImageWidth,
		ImageHeight: challenge.ImageHeight,
	}, nil
}

func (s *service) VerifyRegistrationChallenge(req *dto.CaptchaVerifyReq, ip string) (*dto.CaptchaVerifyResp, error) {
	ctx := context.Background()

	// 读取并立即删除 challenge，保证每次挑战只能尝试一次。
	key := challengeKey(req.ChallengeID)
	raw, err := s.rdb.GetDel(ctx, key).Result()
	if err != nil {
		return nil, ErrInvalidCaptcha
	}

	var payload challengePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}

	if !slide.Validate(req.X, req.Y, payload.X, payload.Y, verifyPadding) {
		return nil, ErrInvalidCaptcha
	}

	token, err := randomHex(randomTokenBytes)
	if err != nil {
		return nil, err
	}

	tokenPayload, err := json.Marshal(registrationTokenPayload{IP: ip})
	if err != nil {
		return nil, err
	}

	if err := s.rdb.Set(ctx, tokenKey(token), tokenPayload, tokenTTL).Err(); err != nil {
		return nil, err
	}

	return &dto.CaptchaVerifyResp{CaptchaToken: token}, nil
}

func (s *service) ConsumeRegistrationToken(token string, ip string) error {
	if token == "" {
		return ErrInvalidCaptchaToken
	}

	ctx := context.Background()

	// token 读取后删除，实现“验证通过后只能发送一次邮件验证码”。
	raw, err := s.rdb.GetDel(ctx, tokenKey(token)).Result()
	if err != nil {
		return ErrInvalidCaptchaToken
	}

	var payload registrationTokenPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return err
	}
	if payload.IP != ip {
		return ErrInvalidCaptchaToken
	}

	return nil
}

type challengePayload struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type registrationTokenPayload struct {
	IP string `json:"ip"`
}

func challengeKey(challengeID string) string {
	return fmt.Sprintf("captcha:register:challenge:%s", challengeID)
}

func tokenKey(token string) string {
	return fmt.Sprintf("captcha:register:token:%s", token)
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
