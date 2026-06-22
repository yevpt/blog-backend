// Package notification 提供站内通知的 HTTP 入口。
// handler 只做身份提取、参数绑定、调用 service 与选择统一响应，不含业务规则。
package notification

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/middleware"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
	"github.com/vpt/blog-backend/pkg/response"
)

// NotificationHandler 站内通知 HTTP 处理器。
type NotificationHandler struct {
	svc notificationservice.InboxService
}

// NewNotificationHandler 创建站内通知处理器。
func NewNotificationHandler(svc notificationservice.InboxService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// requiredUserID 取当前登录用户 ID；未登录时写 401 并返回 false。
func requiredUserID(c *gin.Context) (uint, bool) {
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return 0, false
	}
	return uint(detail.ID), true
}

// writeResponse 统一处理 service 返回：成功回数据，已知错误映射状态码，其余 500。
func writeResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, notificationservice.ErrNotificationNotFound) {
		response.NotFound(c)
		return
	}
	response.ServerError(c)
}
