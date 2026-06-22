package notification

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// List 分页查询当前用户的站内通知。
// @Summary 分页查询站内通知
// @Description 登录用户分页查询自己的站内通知，支持 unread_only 只看未读。
// @Tags 通知
// @Accept json
// @Produce json
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Param unread_only query bool false "为 true 时只返回未读"
// @Success 200 {object} response.Response{data=dto.NotificationPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /notifications [get]
func (h *NotificationHandler) List(c *gin.Context) {
	userID, ok := requiredUserID(c)
	if !ok {
		return
	}

	var req dto.NotificationListReq
	if !reqbind.Query(c, &req) {
		return
	}

	resp, err := h.svc.List(userID, req)
	writeResponse(c, resp, err)
}

// UnreadCount 查询当前用户的未读通知数量。
// @Summary 查询未读通知数量
// @Description 登录用户查询自己的未读站内通知数量，用于小红点展示。
// @Tags 通知
// @Accept json
// @Produce json
// @Success 200 {object} response.Response{data=dto.NotificationUnreadCountResp} "统一响应；code=0 表示查询成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /notifications/unread-count [get]
func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	userID, ok := requiredUserID(c)
	if !ok {
		return
	}

	resp, err := h.svc.UnreadCount(userID)
	writeResponse(c, resp, err)
}
