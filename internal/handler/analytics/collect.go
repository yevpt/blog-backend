package analytics

import (
	"net/http"

	"github.com/gin-gonic/gin"
	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	"github.com/vpt/blog-backend/internal/middleware"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"github.com/vpt/blog-backend/pkg/jwt"
)

// CollectHandler 处理 /collect 上报。
type CollectHandler struct {
	svc            svc.CollectService
	allowedOrigins map[string]struct{}
}

// NewCollectHandler 构造。allowedOrigins 为空表示不校验（开发环境）。
func NewCollectHandler(s svc.CollectService, allowedOrigins []string) *CollectHandler {
	set := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o != "" {
			set[o] = struct{}{}
		}
	}
	return &CollectHandler{svc: s, allowedOrigins: set}
}

// Collect 接收上报，校验/富化交由 service，统一返回 204。
// @Summary  站点访问上报
// @Tags     analytics
// @Accept   json
// @Param    body body dto.CollectRequest true "上报载荷"
// @Success  204
// @Router   /collect [post]
func (h *CollectHandler) Collect(c *gin.Context) {
	var req dto.CollectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Status(http.StatusNoContent) // 上报失败不回错，避免暴露细节/影响前台
		return
	}
	origin := c.GetHeader("Origin")
	raw := svc.RawEvent{
		EventType:     req.EventType,
		VisitorID:     middleware.GetVisitorID(c),
		SessionID:     req.SessionID,
		Path:          req.Path,
		Title:         req.Title,
		Referer:       req.Referer,
		UA:            c.Request.UserAgent(),
		IP:            c.ClientIP(),
		Origin:        origin,
		OriginAllowed: h.originAllowed(origin),
		UserID:        userIDFromContext(c),
		CollectToken:  req.CollectToken,
		Signals:       svc.CollectSignals{WebDriver: req.Signals.WebDriver, NoInteraction: req.Signals.NoInteraction},
	}
	_ = h.svc.Handle(c.Request.Context(), raw)
	c.Status(http.StatusNoContent)
}

// originAllowed 判断 Origin 是否在白名单内；未配置白名单（开发）恒放行。
func (h *CollectHandler) originAllowed(origin string) bool {
	if len(h.allowedOrigins) == 0 {
		return true
	}
	_, ok := h.allowedOrigins[origin]
	return ok
}

// userIDFromContext 从 OptionalAuth 写入的 Claims 取 user_id，未登录返回 nil。
func userIDFromContext(c *gin.Context) *uint {
	claims := jwt.GetClaims(c)
	if claims == nil {
		return nil
	}
	id := uint(claims.UserId)
	return &id
}
