package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/service"
	"github.com/vpt/blog-backend/pkg/response"
)

// AuthHandler 认证模块 handler，对应 /auth 路由组
type AuthHandler struct {
	svc service.AuthService
}

func NewAuthHandler(svc service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// SendCode 发送邮箱验证码，频率超限时返回 429 而非 400
// POST /auth/send-code
func (h *AuthHandler) SendCode(c *gin.Context) {
	var req dto.SendCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	if err := h.svc.SendCode(req.Email, c.ClientIP()); err != nil {
		if isTooManyRequests(err) {
			response.TooManyRequests(c, err.Error(), 0)
			return
		}
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

	response.Success(c, nil)
}

// Register 邮箱注册，验证码一次性消费，注册成功后直接返回用户信息（无 token）
// POST /auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	user, err := h.svc.Register(&req)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

	response.Success(c, user)
}

// Login 三合一登录（username / email / phone），账号禁用返回 403，其余错误一律 401
// POST /auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	resp, err := h.svc.Login(&req, c.ClientIP())
	if err != nil {
		if errors.Is(err, service.ErrUserDisabled) {
			response.Forbidden(c)
			return
		}
		response.Unauthorized(c)
		return
	}

	response.Success(c, resp)
}

// Refresh 用 refresh token 换发新的 access + refresh token（token rotation），旧 refresh 自动失效
// POST /auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	resp, err := h.svc.Refresh(req.RefreshToken)
	if err != nil {
		response.Unauthorized(c)
		return
	}

	response.Success(c, resp)
}

// isTooManyRequests 合并短期限流与日限两种错误，统一映射到 429 响应
func isTooManyRequests(err error) bool {
	return errors.Is(err, service.ErrTooManyRequests) ||
		errors.Is(err, service.ErrDailyLimitExceeded)
}
