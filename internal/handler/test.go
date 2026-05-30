package handler

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
	"github.com/vpt/blog-backend/pkg/roles"
)

// TestHandler 提供开发阶段的测试接口，用于验证中间件和权限体系是否正常工作
type TestHandler struct {
	jwtManager *jwt.Manager
}

// NewTestHandler 创建测试 handler
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
	response.Success(c, gin.H{
		"message":  "登录接口，已验证身份",
		"user_id":  claims.UserId,
		"username": claims.Username,
		"roles":    claims.Roles,
	})
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

// GenToken 生成指定角色的测试 JWT，仅在非生产环境可用
// POST /test/token
// Body: {"user_id": 1, "username": "test", "roles": ["ROLE_ADMIN"]}
func (h *TestHandler) GenToken(c *gin.Context) {
	// 安全起见，生产环境禁用此接口
	if os.Getenv("APP_ENV") == "prod" {
		response.Forbidden(c)
		return
	}

	var req struct {
		UserId   int64    `json:"user_id"`
		Username string   `json:"username"`
		Roles    []string `json:"roles"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	// 默认给 Normal 角色，防止空角色
	if len(req.Roles) == 0 {
		req.Roles = []string{roles.NormalRole}
	}
	if req.UserId == 0 {
		req.UserId = 1
	}
	if req.Username == "" {
		req.Username = "test_user"
	}

	token, err := h.jwtManager.GenerateAccess(req.UserId, req.Username, req.Roles)
	if err != nil {
		response.Fail(c, response.CodeServerError, "token 生成失败")
		return
	}

	response.Success(c, gin.H{
		"token":    token,
		"user_id":  req.UserId,
		"username": req.Username,
		"roles":    req.Roles,
	})
}
