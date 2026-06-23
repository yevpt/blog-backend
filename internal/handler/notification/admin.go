package notification

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
	"github.com/vpt/blog-backend/pkg/response"
)

// NotificationAdminHandler 通知管理端 HTTP 处理器。
type NotificationAdminHandler struct {
	svc notificationservice.AdminService
}

// NewNotificationAdminHandler 创建通知管理端处理器。
func NewNotificationAdminHandler(svc notificationservice.AdminService) *NotificationAdminHandler {
	return &NotificationAdminHandler{svc: svc}
}

// writeAdminResponse 统一处理管理端 service 返回。
func writeAdminResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, notificationservice.ErrQuotaPolicyNotFound) ||
		errors.Is(err, notificationservice.ErrBatchNotRetryable) {
		response.NotFound(c)
		return
	}
	response.ServerError(c)
}

// ListEmailTasks 分页查询邮件任务。
// @Summary 查询邮件任务（管理端）
// @Description 管理员按状态分页查询邮件任务队列。
// @Tags 通知管理
// @Accept json
// @Produce json
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Param status query string false "状态过滤，留空表示全部"
// @Success 200 {object} response.Response{data=dto.AdminEmailTaskPageResp} "统一响应；code=0 表示成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/notifications/email-tasks [get]
func (h *NotificationAdminHandler) ListEmailTasks(c *gin.Context) {
	var req dto.AdminNotificationListReq
	if !reqbind.Query(c, &req) {
		return
	}
	resp, err := h.svc.ListEmailTasks(req)
	writeAdminResponse(c, resp, err)
}

// ListEmailBatches 分页查询邮件批次。
// @Summary 查询邮件批次（管理端）
// @Description 管理员按状态分页查询邮件批次。
// @Tags 通知管理
// @Accept json
// @Produce json
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Param status query string false "状态过滤，留空表示全部"
// @Success 200 {object} response.Response{data=dto.AdminEmailBatchPageResp} "统一响应；code=0 表示成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/notifications/email-batches [get]
func (h *NotificationAdminHandler) ListEmailBatches(c *gin.Context) {
	var req dto.AdminNotificationListReq
	if !reqbind.Query(c, &req) {
		return
	}
	resp, err := h.svc.ListEmailBatches(req)
	writeAdminResponse(c, resp, err)
}

// ListQuotas 查询额度策略与角色额度。
// @Summary 查询邮件额度策略（管理端）
// @Description 管理员查询 purpose 额度策略与角色额度策略。
// @Tags 通知管理
// @Accept json
// @Produce json
// @Success 200 {object} response.Response{data=dto.AdminQuotaListResp} "统一响应；code=0 表示成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/notifications/email-quotas [get]
func (h *NotificationAdminHandler) ListQuotas(c *gin.Context) {
	resp, err := h.svc.ListQuotas()
	writeAdminResponse(c, resp, err)
}

// UpdateQuota 调整 purpose 额度策略。
// @Summary 调整邮件额度策略（管理端）
// @Description 管理员调整指定 purpose 的额度与频率，数值必须非负且不超过上限。
// @Tags 通知管理
// @Accept json
// @Produce json
// @Param id path int true "额度策略 ID"
// @Param request body dto.AdminUpdateQuotaReq true "额度调整请求"
// @Success 200 {object} response.Response "统一响应；code=0 表示成功，code=400 表示参数越界"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 404 {object} response.Response "额度策略不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/notifications/email-quotas/{id} [put]
func (h *NotificationAdminHandler) UpdateQuota(c *gin.Context) {
	id, ok := reqbind.PathUint(c, "id", "额度策略 ID")
	if !ok {
		return
	}
	var req dto.AdminUpdateQuotaReq
	if !reqbind.JSON(c, &req) {
		return
	}
	writeAdminResponse(c, nil, h.svc.UpdateQuota(id, req))
}

// UpdateRoleQuota 调整角色额度策略。
// @Summary 调整角色额度策略（管理端）
// @Description 管理员调整指定角色的 actor/recipient 额度，数值必须非负且不超过上限。
// @Tags 通知管理
// @Accept json
// @Produce json
// @Param id path int true "角色额度策略 ID"
// @Param request body dto.AdminUpdateRoleQuotaReq true "角色额度调整请求"
// @Success 200 {object} response.Response "统一响应；code=0 表示成功，code=400 表示参数越界"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 404 {object} response.Response "额度策略不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/notifications/role-quotas/{id} [put]
func (h *NotificationAdminHandler) UpdateRoleQuota(c *gin.Context) {
	id, ok := reqbind.PathUint(c, "id", "角色额度策略 ID")
	if !ok {
		return
	}
	var req dto.AdminUpdateRoleQuotaReq
	if !reqbind.JSON(c, &req) {
		return
	}
	writeAdminResponse(c, nil, h.svc.UpdateRoleQuota(id, req))
}

// RetryBatch 重试失败批次。
// @Summary 重试失败邮件批次（管理端）
// @Description 管理员把失败/延后的批次重置为待发送，不立即发送，由 sender 在下次循环处理。
// @Tags 通知管理
// @Accept json
// @Produce json
// @Param id path int true "邮件批次 ID"
// @Success 200 {object} response.Response{data=dto.AdminBatchRetryResp} "统一响应；code=0 表示成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 404 {object} response.Response "批次不存在或不可重试"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/notifications/email-batches/{id}/retry [post]
func (h *NotificationAdminHandler) RetryBatch(c *gin.Context) {
	id, ok := reqbind.PathUint(c, "id", "邮件批次 ID")
	if !ok {
		return
	}
	resp, err := h.svc.RetryBatch(id)
	writeAdminResponse(c, resp, err)
}
