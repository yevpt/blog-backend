package auth

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	authservice "github.com/vpt/blog-backend/internal/service/auth"
	"github.com/vpt/blog-backend/pkg/response"
)

// AuthHandler 认证模块 handler，对应 /auth 路由组
type AuthHandler struct {
	svc authservice.AuthService
}

func NewAuthHandler(svc authservice.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// SendCode 发送邮箱验证码，要求先完成 GoCaptcha 图形验证，频率超限时返回 429 而非 400。
// @Summary 发送邮箱验证码
// @Description 消费 GoCaptcha 一次性票据后向指定邮箱发送注册验证码；参数错误或普通业务错误通过统一响应 code 表达，发送频率超限时返回 HTTP 429。
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body dto.SendCodeReq true "发送验证码请求"
// @Success 200 {object} response.Response "统一响应；code=0 表示发送成功，code=400 表示参数错误或业务错误"
// @Failure 429 {object} response.Response "发送频率超限"
// @Router /auth/send-code [post]
func (h *AuthHandler) SendCode(c *gin.Context) {
	// 绑定并校验请求参数（email 为 required 且格式合法）
	var req dto.SendCodeReq
	if !reqbind.JSON(c, &req) {
		return
	}

	// 调用 service 发送验证码，IP 透传用于校验图形验证码票据归属
	if err := h.svc.SendCode(req.Email, c.ClientIP(), req.CaptchaToken); err != nil {
		// 频率超限（冷却/10分钟/日限）映射到 429，其余业务错误映射到 400
		if isTooManyRequests(err) {
			response.TooManyRequests(c, err.Error(), 0)
			return
		}
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

	// 发送成功，data 为 null，客户端只需判断 code 是否为 0
	response.Success(c, nil)
}

// Register 邮箱注册，验证码一次性消费，注册成功后直接返回用户信息（无 token）。
// @Summary 邮箱注册
// @Description 使用邮箱、密码和验证码创建用户；参数错误、验证码错误或邮箱已存在通过统一响应 code 表达。
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body dto.RegisterReq true "注册请求"
// @Success 200 {object} response.Response{data=dto.UserResp} "统一响应；code=0 表示注册成功，code=400 表示参数错误或业务错误"
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	// 绑定并校验请求参数（email、password、code 均为 required）
	var req dto.RegisterReq
	if !reqbind.JSON(c, &req) {
		return
	}

	// 调用 service 完成注册流程：验证码校验 → 邮箱唯一性检查 → 创建用户
	user, err := h.svc.Register(&req)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

	// 注册成功返回用户信息，不含 token，用户需单独调用登录接口获取
	response.Success(c, user)
}

// Login 三合一登录（username / email / phone），按失败原因返回前端可展示文案。
// @Summary 用户登录
// @Description 支持用户名、邮箱或手机号作为登录标识，成功后返回 access token、refresh token 和用户信息。
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body dto.LoginReq true "登录请求"
// @Success 200 {object} response.Response{data=dto.LoginResp} "登录成功"
// @Failure 401 {object} response.Response "账号不存在或密码错误"
// @Failure 403 {object} response.Response "账号已被禁用"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	// 绑定并校验请求参数（identifier、password 均为 required）
	var req dto.LoginReq
	if !reqbind.JSON(c, &req) {
		return
	}

	// 调用 service 执行登录：查用户 → 比对密码 → 校验状态 → 签发双 token
	resp, err := h.svc.Login(&req, c.ClientIP())
	if err != nil {
		// 按 service 返回的明确错误选择响应，避免登录页显示 token 相关文案
		switch {
		case errors.Is(err, authservice.ErrUserNotFound):
			response.AuthFailed(c, authservice.ErrUserNotFound.Error())
			return
		case errors.Is(err, authservice.ErrWrongPassword):
			response.AuthFailed(c, authservice.ErrWrongPassword.Error())
			return
		case errors.Is(err, authservice.ErrInvalidCredential):
			response.AuthFailed(c, authservice.ErrInvalidCredential.Error())
			return
		case errors.Is(err, authservice.ErrUserDisabled):
			response.ForbiddenWithMessage(c, authservice.ErrUserDisabled.Error())
			return
		}
		response.ServerError(c)
		return
	}

	// 登录成功返回双 token 和用户基本信息
	response.Success(c, resp)
}

// Refresh 用 refresh token 换发新的 access + refresh token（token rotation），旧 refresh 自动失效。
// @Summary 刷新令牌
// @Description 使用 refresh token 换发新的 access token 和 refresh token；旧 refresh token 会在业务层失效。
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body dto.RefreshReq true "刷新令牌请求"
// @Success 200 {object} response.Response{data=dto.TokenResp} "刷新成功"
// @Failure 401 {object} response.Response "refresh token 非法、过期或类型不匹配"
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	// 绑定并校验请求参数（refresh_token 为 required）
	var req dto.RefreshReq
	if !reqbind.JSON(c, &req) {
		return
	}

	// 调用 service 换发新双 token，任何错误（格式非法、已过期、类型不匹配）均返回 401
	resp, err := h.svc.Refresh(req.RefreshToken)
	if err != nil {
		response.Unauthorized(c)
		return
	}

	// 返回新双 token，客户端应替换本地保存的旧 token
	response.Success(c, resp)
}

// isTooManyRequests 合并短期限流与日限两种错误，统一映射到 429 响应
func isTooManyRequests(err error) bool {
	return errors.Is(err, authservice.ErrTooManyRequests) ||
		errors.Is(err, authservice.ErrDailyLimitExceeded)
}
