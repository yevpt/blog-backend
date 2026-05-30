package jwt

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Claims 是 JWT 载荷，TokenType 区分 access / refresh，防止两种 token 混用
type Claims struct {
	UserId    int64    `json:"uid"`
	Username  string   `json:"username"`
	Roles     []string `json:"roles"`
	TokenType string   `json:"type"` // "access" | "refresh"
	jwtlib.RegisteredClaims
}

// contextKey 用于在 gin.Context 中存取 Claims，避免与其他 key 冲突
type contextKey string

const claimsKey contextKey = "claims"

var (
	ErrTokenExpired = errors.New("token 已过期")
	ErrTokenInvalid = errors.New("token 无效")
)

// Manager 持有 JWT 配置，负责生成和解析 token
type Manager struct {
	secret             []byte
	expireHours        int
	refreshExpireHours int
}

// NewManager 创建 JWT 管理器
func NewManager(secret string, expireHours int, refreshExpireHours int) *Manager {
	return &Manager{
		secret:             []byte(secret),
		expireHours:        expireHours,
		refreshExpireHours: refreshExpireHours,
	}
}

// GenerateAccess 生成短期 access token（expireHours）
func (m *Manager) GenerateAccess(userId int64, username string, roles []string) (string, error) {
	return m.generate(userId, username, roles, "access", m.expireHours)
}

// GenerateRefresh 生成长期 refresh token（refreshExpireHours）
func (m *Manager) GenerateRefresh(userId int64, username string, roles []string) (string, error) {
	return m.generate(userId, username, roles, "refresh", m.refreshExpireHours)
}

func (m *Manager) generate(userId int64, username string, roles []string, tokenType string, hours int) (string, error) {
	claims := Claims{
		UserId:    userId,
		Username:  username,
		Roles:     roles,
		TokenType: tokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Duration(hours) * time.Hour)),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Parse 解析并验证 token，不校验 TokenType（由调用方决定）
func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	token, err := jwtlib.ParseWithClaims(tokenStr, &Claims{}, func(token *jwtlib.Token) (interface{}, error) {
		// 只接受 HMAC 签名算法
		if _, ok := token.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, ErrTokenInvalid
		}
		return m.secret, nil
	})

	if err != nil {
		if errors.Is(err, jwtlib.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}

// SetClaims 将 Claims 写入 gin.Context，供后续中间件和 handler 读取
func SetClaims(c *gin.Context, claims *Claims) {
	c.Set(string(claimsKey), claims)
}

// GetClaims 从 gin.Context 中读取 Claims，需在 Auth 中间件之后调用
func GetClaims(c *gin.Context) *Claims {
	val, exists := c.Get(string(claimsKey))
	if !exists {
		return nil
	}
	claims, _ := val.(*Claims)
	return claims
}
