package jwt

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Claims JWT 载荷，只存 userId 和 token 类型，不存可变字段（username/roles 随时可变）
type Claims struct {
	UserId    int64  `json:"uid"`
	TokenType string `json:"type"` // "access" | "refresh"
	jwtlib.RegisteredClaims
}

type contextKey string

const claimsKey contextKey = "claims"

var (
	ErrTokenExpired = errors.New("token 已过期")
	ErrTokenInvalid = errors.New("token 无效")
)

type Manager struct {
	secret             []byte
	expireHours        int
	refreshExpireHours int
}

func NewManager(secret string, expireHours int, refreshExpireHours int) *Manager {
	return &Manager{
		secret:             []byte(secret),
		expireHours:        expireHours,
		refreshExpireHours: refreshExpireHours,
	}
}

// GenerateAccess 签发短期 access token，只存 userId，不存可变字段
func (m *Manager) GenerateAccess(userId int64) (string, error) {
	return m.generate(userId, "access", m.expireHours)
}

// GenerateRefresh 签发长期 refresh token，只存 userId
func (m *Manager) GenerateRefresh(userId int64) (string, error) {
	return m.generate(userId, "refresh", m.refreshExpireHours)
}

func (m *Manager) generate(userId int64, tokenType string, hours int) (string, error) {
	claims := Claims{
		UserId:    userId,
		TokenType: tokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Duration(hours) * time.Hour)),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	token, err := jwtlib.ParseWithClaims(tokenStr, &Claims{}, func(token *jwtlib.Token) (interface{}, error) {
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

func SetClaims(c *gin.Context, claims *Claims) {
	c.Set(string(claimsKey), claims)
}

func GetClaims(c *gin.Context) *Claims {
	val, exists := c.Get(string(claimsKey))
	if !exists {
		return nil
	}
	claims, _ := val.(*Claims)
	return claims
}
