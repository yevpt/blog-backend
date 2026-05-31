package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

// Auth 校验 Bearer token，并要求 TokenType == "access"，防止 refresh token 被误用
func Auth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查 Authorization header 是否存在
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		// 拆分 "Bearer <token>" 格式，确保前缀为 Bearer
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		// 解析 JWT，验证签名和过期时间
		claims, err := jwtManager.Parse(parts[1])
		if err != nil {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		// refresh token 仅允许访问 /auth/refresh，拒绝其用于业务接口
		if claims.TokenType != "access" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		// 将已验证的 claims 写入 context，供后续 handler 通过 jwt.GetClaims(c) 读取
		jwt.SetClaims(c, claims)
		c.Next()
	}
}

// OptionalAuth 可选解析 Bearer token：无 token 直接放行，有 token 则必须合法并写入 claims。
// 用于公开接口在登录用户访问时提供个性化字段，但不强制匿名用户登录。
func OptionalAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 没有 Authorization header 时保持匿名访问语义
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		// 有 header 时按严格 Auth 规则解析，避免客户端携带坏 token 却被静默当匿名处理
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		claims, err := jwtManager.Parse(parts[1])
		if err != nil || claims.TokenType != "access" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		jwt.SetClaims(c, claims)
		c.Next()
	}
}
