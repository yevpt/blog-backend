package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
	"github.com/vpt/blog-backend/pkg/roles"
)

// RequireRole 角色权限中间件工厂，须在 Auth 中间件之后使用。
// minRole 为访问所需的最低角色，权重更高的角色（如 Admin）自动覆盖低权重接口的权限。
func RequireRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := jwt.GetClaims(c)
		if claims == nil {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		if !roles.HasPermission(claims.Roles, minRole) {
			response.Forbidden(c)
			c.Abort()
			return
		}

		c.Next()
	}
}
