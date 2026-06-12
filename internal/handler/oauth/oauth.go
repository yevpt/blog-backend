package oauth

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/middleware"
	domain "github.com/vpt/blog-backend/internal/oauth"
	serviceoauth "github.com/vpt/blog-backend/internal/service/oauth"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

// OAuthHandler 第三方登录 HTTP 入口，只负责参数、登录态和统一响应。
type OAuthHandler struct {
	svc serviceoauth.OAuthService
}

func NewOAuthHandler(svc serviceoauth.OAuthService) *OAuthHandler {
	return &OAuthHandler{svc: svc}
}

// Providers 返回当前启用的 OAuth 平台列表。
// @Summary 获取第三方登录平台
// @Description 返回当前后端已启用的第三方登录平台。
// @Tags 第三方登录
// @Accept json
// @Produce json
// @Success 200 {object} response.Response "统一响应；code=0 表示查询成功"
// @Router /oauth/providers [get]
func (h *OAuthHandler) Providers(c *gin.Context) {
	response.Success(c, h.svc.Providers(c.Request.Context()))
}

// Authorize 创建第三方授权 URL。
// @Summary 创建第三方授权地址
// @Description action=login 表示第三方登录；action=bind 表示绑定到当前登录用户，绑定动作必须携带 access token。
// @Tags 第三方登录
// @Accept json
// @Produce json
// @Param source path string true "平台标识，如 github"
// @Param action query string true "授权动作：login 或 bind"
// @Param redirect_uri query string false "前端回跳地址"
// @Success 200 {object} response.Response{data=dto.OAuthAuthorizeResp} "授权地址创建成功"
// @Failure 401 {object} response.Response "绑定动作未登录"
// @Router /oauth/{source}/authorize [get]
func (h *OAuthHandler) Authorize(c *gin.Context) {
	action, err := domain.ParseAction(c.Query("action"))
	if err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}

	resp, err := h.svc.Authorize(
		c.Request.Context(),
		c.Param("source"),
		action,
		currentUserID(c),
		c.Query("redirect_uri"),
	)
	if err != nil {
		writeOAuthError(c, err)
		return
	}

	response.Success(c, resp)
}

// Callback 处理第三方平台回调。
// @Summary 处理第三方登录回调
// @Description 校验一次性 state，使用 code 换取第三方 token，再完成登录或绑定。
// @Tags 第三方登录
// @Accept json
// @Produce json
// @Param source path string true "平台标识，如 github"
// @Param code query string true "第三方授权码"
// @Param state query string true "后端生成的一次性 state"
// @Success 200 {object} response.Response{data=dto.OAuthCallbackResp} "callback 处理成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /oauth/{source}/callback [get]
func (h *OAuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" || state == "" {
		response.Fail(c, response.CodeBadRequest, "缺少 OAuth callback 参数")
		return
	}

	resp, err := h.svc.Callback(c.Request.Context(), c.Param("source"), code, state)
	if err != nil {
		writeOAuthError(c, err)
		return
	}

	response.Success(c, resp)
}

// ListBindings 查询当前用户的第三方账号绑定。
// @Summary 查询第三方账号绑定
// @Description 返回当前登录用户已绑定的第三方平台。
// @Tags 第三方登录
// @Accept json
// @Produce json
// @Success 200 {object} response.Response{data=[]dto.OAuthBindingResp} "查询成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Router /oauth/bindings [get]
func (h *OAuthHandler) ListBindings(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		response.Unauthorized(c)
		return
	}
	resp, err := h.svc.ListBindings(c.Request.Context(), userID)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// Unbind 解绑当前用户的指定第三方平台。
// @Summary 解绑第三方账号
// @Description 软删除当前用户与指定平台的绑定关系；如果会导致无可用登录方式则拒绝。
// @Tags 第三方登录
// @Accept json
// @Produce json
// @Param source path string true "平台标识，如 github"
// @Success 200 {object} response.Response "解绑成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Router /oauth/bindings/{source} [delete]
func (h *OAuthHandler) Unbind(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.Unbind(c.Request.Context(), userID, c.Param("source")); err != nil {
		writeOAuthError(c, err)
		return
	}
	response.Success(c, nil)
}

func currentUserID(c *gin.Context) uint {
	if detail := middleware.GetUserDetail(c); detail != nil {
		return detail.ID
	}
	if claims := jwtpkg.GetClaims(c); claims != nil {
		return uint(claims.UserId)
	}
	return 0
}

func writeOAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, serviceoauth.ErrLoginRequired):
		response.Unauthorized(c)
	case errors.Is(err, serviceoauth.ErrUserDisabled):
		response.ForbiddenWithMessage(c, err.Error())
	case errors.Is(err, serviceoauth.ErrSocialIdentityBound),
		errors.Is(err, serviceoauth.ErrSourceAlreadyBound),
		errors.Is(err, serviceoauth.ErrLastLoginMethod),
		errors.Is(err, domain.ErrInvalidAction),
		errors.Is(err, domain.ErrInvalidState),
		errors.Is(err, domain.ErrProviderNotEnabled),
		errors.Is(err, domain.ErrStateSourceMismatch):
		response.Fail(c, response.CodeBadRequest, err.Error())
	default:
		response.ServerError(c)
	}
}

var _ = dto.OAuthAuthorizeResp{}
