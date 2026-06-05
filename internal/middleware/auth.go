package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/service"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

type userDetailContextKey string

const userDetailKey userDetailContextKey = "userDetail"

// GetUserDetail 从 gin.Context 读取已认证用户资料，须在 Auth 中间件之后调用。
// 返回 nil 时表示未经过 Auth 中间件或用户加载失败。
func GetUserDetail(c *gin.Context) *dto.UserDetailResp {
	val, exists := c.Get(userDetailKey)
	if !exists {
		return nil
	}
	detail, _ := val.(*dto.UserDetailResp)
	return detail
}

// Auth 校验 Bearer access token，并从 Redis/DB 加载完整用户资料写入 Context。
// userCache 为 nil 时跳过缓存加载（仅用于测试）。
// 用户被禁用（Status != 1）时也返回 401。
func Auth(jwtManager *jwt.Manager, userCache service.UserCacheService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		claims, err := jwtManager.Parse(parts[1])
		if err != nil {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		if claims.TokenType != "access" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		if userCache != nil {
			detail, cacheErr := userCache.Get(context.Background(), claims.UserId)
			if cacheErr != nil || detail == nil || detail.Status != 1 {
				response.Unauthorized(c)
				c.Abort()
				return
			}
			c.Set(userDetailKey, detail)
		}

		jwt.SetClaims(c, claims)
		c.Next()
	}
}

// OptionalAuth 可选解析 Bearer token：无 token 直接放行，有 token 则必须合法。
// 只设置 JWT claims（userId），不加载完整用户资料。
func OptionalAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

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
