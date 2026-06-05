package handler

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

// TestHandler 开发调试用 handler，用于验证中间件和权限体系，不对外暴露业务数据
type TestHandler struct {
	jwtManager *jwt.Manager
}

func NewTestHandler(jwtManager *jwt.Manager) *TestHandler {
	return &TestHandler{jwtManager: jwtManager}
}

// Public 公开接口，无需登录，任何人可访问
// GET /test/public
func (h *TestHandler) Public(c *gin.Context) {
	response.Success(c, gin.H{"message": "公开接口，无需登录"})
}

// Authed 需要登录才能访问，返回当前登录用户信息
// GET /test/authed
func (h *TestHandler) Authed(c *gin.Context) {
	claims := jwt.GetClaims(c)
	detail := middleware.GetUserDetail(c)
	resp := gin.H{
		"message": "登录接口，已验证身份",
		"user_id": claims.UserId,
	}
	if detail != nil {
		resp["username"] = detail.Username
		resp["roles"] = detail.Roles
	}
	response.Success(c, resp)
}

// Vip 需要 VIP 或更高权限（Admin 也可以访问）
// GET /test/vip
func (h *TestHandler) Vip(c *gin.Context) {
	response.Success(c, gin.H{"message": "VIP 接口，你有 VIP 或 ADMIN 权限"})
}

// Admin 仅管理员可访问
// GET /test/admin
func (h *TestHandler) Admin(c *gin.Context) {
	response.Success(c, gin.H{"message": "管理员接口，仅 ADMIN 可访问"})
}

// GenToken 生成测试 JWT，生产环境（APP_ENV=prod）强制返回 403
// POST /test/token
func (h *TestHandler) GenToken(c *gin.Context) {
	if os.Getenv("APP_ENV") == "prod" {
		response.Forbidden(c)
		return
	}

	var req struct {
		UserId int64 `json:"user_id"`
	}
	if !reqbind.JSON(c, &req) {
		return
	}

	if req.UserId == 0 {
		req.UserId = 1
	}

	token, err := h.jwtManager.GenerateAccess(req.UserId)
	if err != nil {
		response.Fail(c, response.CodeServerError, "token 生成失败")
		return
	}

	response.Success(c, gin.H{
		"token":   token,
		"user_id": req.UserId,
	})
}
