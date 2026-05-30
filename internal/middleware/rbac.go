package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
	"github.com/vpt/blog-backend/pkg/roles"
)

// RequireRole 是角色权限中间件工厂函数，类似 Spring 的 @PreAuthorize("hasRole('XXX')")。
// minRole 是访问该接口所需的最低角色，权重越高（数字越小）的角色包含低权重角色的权限。
//
// 使用示例：
//
//	admin := r.Group("/admin", middleware.Auth(jwtMgr), middleware.RequireRole(roles.AdminRole))
//	vip   := r.Group("/",      middleware.Auth(jwtMgr), middleware.RequireRole(roles.VipRole))
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
