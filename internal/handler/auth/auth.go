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

// SendPasswordResetCode 发送忘记密码验证码，不暴露目标邮箱是否存在。
// @Summary 发送忘记密码验证码
// @Description 消费 GoCaptcha 一次性票据后尝试向邮箱发送重置密码验证码；无论邮箱是否存在，成功响应都不暴露账号状态。
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body dto.PasswordResetCodeReq true "忘记密码验证码请求"
// @Success 200 {object} response.Response "统一响应；code=0 表示请求已受理，code=400 表示参数错误或业务错误"
// @Failure 429 {object} response.Response "发送频率超限"
// @Router /auth/password-reset/code [post]
func (h *AuthHandler) SendPasswordResetCode(c *gin.Context) {
	var req dto.PasswordResetCodeReq
	if !reqbind.JSON(c, &req) {
		return
	}

	if err := h.svc.SendPasswordResetCode(req.Email, c.ClientIP(), req.CaptchaToken); err != nil {
		if isTooManyRequests(err) {
			response.TooManyRequests(c, err.Error(), 0)
			return
		}
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

	response.Success(c, nil)
}

// ResetPassword 使用邮箱验证码重置登录密码。
// @Summary 忘记密码重置
// @Description 使用邮箱验证码设置新密码；成功后旧密码失效，新密码可用于登录。
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body dto.PasswordResetReq true "忘记密码重置请求"
// @Success 200 {object} response.Response "统一响应；code=0 表示重置成功，code=400 表示验证码无效或参数错误"
// @Router /auth/password-reset [post]
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req dto.PasswordResetReq
	if !reqbind.JSON(c, &req) {
		return
	}

	if err := h.svc.ResetPassword(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

	response.Success(c, nil)
}

// Register 邮箱注册，验证码一次性消费，注册成功后直接返回用户信息（无 token）。
// @Summary 邮箱注册
// @Description 使用邮箱、密码和验证码创建用户；可选上传头像（JPG、PNG、WebP，原始最大 2MB，不支持 GIF）。参数错误、验证码错误或邮箱已存在通过统一响应 code 表达。
// @Tags 认证
// @Accept multipart/form-data
// @Produce json
// @Param email formData string true "邮箱"
// @Param password formData string true "密码（至少 8 位）"
// @Param code formData string true "6 位邮箱验证码"
// @Param nickname formData string false "昵称"
// @Param avatar formData file false "可选头像图片"
// @Success 200 {object} response.Response{data=dto.UserResp} "统一响应；code=0 表示注册成功，code=400 表示参数错误或业务错误"
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterReq
	if !reqbind.Form(c, &req) {
		return
	}

	avatar, err := readRegisterAvatar(c)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

	user, err := h.svc.Register(&req, avatar)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

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

// AdminLogin 管理后台登录，仅允许用户名和密码，且账号必须持有管理员角色。
// @Summary 管理后台登录
// @Description 管理页入口登录，仅支持用户名和密码；成功后返回 access token、refresh token 和用户信息，非管理员拒绝登录。
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body dto.AdminLoginReq true "管理后台登录请求"
// @Success 200 {object} response.Response{data=dto.LoginResp} "登录成功"
// @Failure 401 {object} response.Response "账号不存在或密码错误"
// @Failure 403 {object} response.Response "账号已被禁用或非管理员"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/auth/login [post]
func (h *AuthHandler) AdminLogin(c *gin.Context) {
	var req dto.AdminLoginReq
	if !reqbind.JSON(c, &req) {
		return
	}

	resp, err := h.svc.AdminLogin(&req, c.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrUserNotFound):
			response.AuthFailed(c, authservice.ErrUserNotFound.Error())
			return
		case errors.Is(err, authservice.ErrWrongPassword):
			response.AuthFailed(c, authservice.ErrWrongPassword.Error())
			return
		case errors.Is(err, authservice.ErrUserDisabled):
			response.ForbiddenWithMessage(c, authservice.ErrUserDisabled.Error())
			return
		case errors.Is(err, authservice.ErrAdminRequired):
			response.ForbiddenWithMessage(c, authservice.ErrAdminRequired.Error())
			return
		}
		response.ServerError(c)
		return
	}

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
