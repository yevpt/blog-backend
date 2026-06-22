package notification

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	"github.com/vpt/blog-backend/pkg/response"
)

// MarkRead 将当前用户名下的单条通知置为已读。
// @Summary 单条通知已读
// @Description 登录用户将自己名下的指定通知置为已读；非本人或不存在返回 404。
// @Tags 通知
// @Accept json
// @Produce json
// @Param id path int true "收件箱通知 ID"
// @Success 200 {object} response.Response "统一响应；code=0 表示已读成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "通知不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /notifications/{id}/read [patch]
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID, ok := requiredUserID(c)
	if !ok {
		return
	}
	id, ok := reqbind.PathUint(c, "id", "通知 ID")
	if !ok {
		return
	}

	err := h.svc.MarkRead(userID, id)
	writeResponse(c, nil, err)
}

// MarkAllRead 批量将当前用户的通知置为已读。
// @Summary 批量通知已读
// @Description 登录用户批量已读；传 ids 列表则只已读指定通知，ids 为空且 all=true 时已读全部未读。
// @Tags 通知
// @Accept json
// @Produce json
// @Param request body dto.NotificationReadAllReq true "批量已读请求"
// @Success 200 {object} response.Response{data=dto.NotificationReadResp} "统一响应；code=0 表示成功，code=400 表示未指定已读范围"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /notifications/read [patch]
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID, ok := requiredUserID(c)
	if !ok {
		return
	}

	var req dto.NotificationReadAllReq
	if !reqbind.JSON(c, &req) {
		return
	}
	// 必须显式指定范围：给定 ids，或 all=true 表示全部未读，避免误操作清空。
	if len(req.IDs) == 0 && !req.All {
		response.Fail(c, response.CodeBadRequest, "请指定要已读的通知，或设置 all=true 表示全部已读")
		return
	}

	resp, err := h.svc.MarkAllRead(userID, req.IDs)
	writeResponse(c, resp, err)
}

// Delete 软删除当前用户名下的单条通知。
// @Summary 删除站内通知
// @Description 登录用户删除自己名下的指定通知（软删除）；非本人或不存在返回 404。
// @Tags 通知
// @Accept json
// @Produce json
// @Param id path int true "收件箱通知 ID"
// @Success 200 {object} response.Response "统一响应；code=0 表示删除成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "通知不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /notifications/{id} [delete]
func (h *NotificationHandler) Delete(c *gin.Context) {
	userID, ok := requiredUserID(c)
	if !ok {
		return
	}
	id, ok := reqbind.PathUint(c, "id", "通知 ID")
	if !ok {
		return
	}

	err := h.svc.Delete(userID, id)
	writeResponse(c, nil, err)
}
