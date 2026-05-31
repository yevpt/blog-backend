package jwt

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Claims JWT 载荷，TokenType 区分 access / refresh，防止 refresh token 被直接用于访问接口
type Claims struct {
	UserId    int64    `json:"uid"`
	Username  string   `json:"username"`
	Roles     []string `json:"roles"`
	TokenType string   `json:"type"` // "access" | "refresh"
	jwtlib.RegisteredClaims
}

// contextKey 强类型 context key，防止与其他包的字符串 key 碰撞
type contextKey string

const claimsKey contextKey = "claims"

var (
	ErrTokenExpired = errors.New("token 已过期")
	ErrTokenInvalid = errors.New("token 无效")
)

// Manager 持有签名密钥和过期时长，是 JWT 生成与解析的唯一入口
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

// GenerateAccess 生成短期 access token，有效期由 expireHours 决定
func (m *Manager) GenerateAccess(userId int64, username string, roles []string) (string, error) {
	return m.generate(userId, username, roles, "access", m.expireHours)
}

// GenerateRefresh 生成长期 refresh token，有效期由 refreshExpireHours 决定，
// 仅用于换发新 token，不得用于访问业务接口
func (m *Manager) GenerateRefresh(userId int64, username string, roles []string) (string, error) {
	return m.generate(userId, username, roles, "refresh", m.refreshExpireHours)
}

func (m *Manager) generate(userId int64, username string, roles []string, tokenType string, hours int) (string, error) {
	// 填充自定义 claims：用户信息、角色列表、token 类型标识
	claims := Claims{
		UserId:    userId,
		Username:  username,
		Roles:     roles,
		TokenType: tokenType,
		// 标准 claims：签发时间（iat）和过期时间（exp），过期时间 = 当前时间 + hours 小时
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Duration(hours) * time.Hour)),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
		},
	}
	// 使用 HS256 对称签名算法生成 JWT 字符串
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Parse 解析并验证 token 签名与过期时间，不校验 TokenType，由调用方自行区分 access / refresh
func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	// 解析 token，keyFunc 同时验证算法类型，防止签名绕过
	token, err := jwtlib.ParseWithClaims(tokenStr, &Claims{}, func(token *jwtlib.Token) (interface{}, error) {
		// 拒绝非 HMAC 算法，防止 alg:none 攻击（攻击者将 alg 设为 none 可绕过签名验证）
		if _, ok := token.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, ErrTokenInvalid
		}
		return m.secret, nil
	})

	// 区分过期错误和其他无效错误，便于调用方给前端提供不同的提示
	if err != nil {
		if errors.Is(err, jwtlib.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	// 二次验证：类型断言确认 claims 格式，token.Valid 确认整体校验通过
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}

// SetClaims 将已验证的 Claims 写入 gin.Context，由 Auth 中间件调用
func SetClaims(c *gin.Context, claims *Claims) {
	c.Set(string(claimsKey), claims)
}

// GetClaims 从 gin.Context 读取 Claims，须在 Auth 中间件之后的 handler 中调用，否则返回 nil
func GetClaims(c *gin.Context) *Claims {
	val, exists := c.Get(string(claimsKey))
	if !exists {
		return nil
	}
	claims, _ := val.(*Claims)
	return claims
}
