package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/pkg/response"
	"github.com/vpt/blog-backend/pkg/roles"
)

// RequireRole 角色权限中间件，须在 Auth 中间件之后使用。
// 从 Context 中读取完整用户资料（由 Auth 中间件写入），检查角色权重。
func RequireRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		detail := GetUserDetail(c)
		if detail == nil {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		if !roles.HasPermission(detail.Roles, minRole) {
			response.Forbidden(c)
			c.Abort()
			return
		}

		c.Next()
	}
}
