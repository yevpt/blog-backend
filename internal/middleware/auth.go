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

		// refresh token 只能用于刷新接口，不能访问普通接口
		if claims.TokenType != "access" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		// 将用户信息注入 context，handler 层通过 jwt.GetClaims(c) 读取
		jwt.SetClaims(c, claims)
		c.Next()
	}
}
