package user

import (
	"errors"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/internal/service/adminlog"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/response"
)

// UserAdminHandler 用户管理端 HTTP 处理器。
type UserAdminHandler struct {
	svc         userservice.AdminService
	log         *zap.Logger
	logRecorder adminlog.Recorder
}

// NewUserAdminHandler 创建用户管理端处理器。
func NewUserAdminHandler(svc userservice.AdminService, log *zap.Logger, logRecorder adminlog.Recorder) *UserAdminHandler {
	return &UserAdminHandler{svc: svc, log: log, logRecorder: logRecorder}
}

func writeUserAdminResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, userservice.ErrUserNotFound) {
		response.NotFound(c)
		return
	}
	response.ServerError(c)
}

// GrantVip 为目标用户赋予 VIP 角色。
// @Summary 赋予用户 VIP 角色
// @Description 管理员为目标用户追加 ROLE_VIP，保留 ROLE_NORMAL；已是 VIP 时幂等成功。
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param id path int true "目标用户 ID"
// @Success 200 {object} response.Response{data=dto.AdminUserRolesResp} "统一响应；code=0 表示成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 404 {object} response.Response "目标用户不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/users/{id}/roles/vip [post]
func (h *UserAdminHandler) GrantVip(c *gin.Context) {
	targetUserID, ok := reqbind.PathUint(c, "id", "用户 ID")
	if !ok {
		return
	}
	resp, err := h.svc.GrantVip(targetUserID)
	if err == nil {
		h.logVipRoleChange(c, "grant", targetUserID)
		h.recordLog(c, adminlog.ActionGrantVIP, targetUserID, nil)
	}
	writeUserAdminResponse(c, resp, err)
}

// RevokeVip 取消目标用户的 VIP 角色。
// @Summary 取消用户 VIP 角色
// @Description 管理员移除目标用户的 ROLE_VIP；本就不是 VIP 时幂等成功。
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param id path int true "目标用户 ID"
// @Success 200 {object} response.Response{data=dto.AdminUserRolesResp} "统一响应；code=0 表示成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 404 {object} response.Response "目标用户不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/users/{id}/roles/vip [delete]
func (h *UserAdminHandler) RevokeVip(c *gin.Context) {
	targetUserID, ok := reqbind.PathUint(c, "id", "用户 ID")
	if !ok {
		return
	}
	resp, err := h.svc.RevokeVip(targetUserID)
	if err == nil {
		h.logVipRoleChange(c, "revoke", targetUserID)
		h.recordLog(c, adminlog.ActionRevokeVIP, targetUserID, nil)
	}
	writeUserAdminResponse(c, resp, err)
}

// NormalizeAvatars 检查并重压缩不符合规范的老用户头像。
// @Summary 归一化老用户头像
// @Description 管理员检查本站托管头像是否超出 240px / 20KB 规范；已合规的 JPG/PNG/WebP 原样保留，超出则压缩为 WebP 替换、更新 avatar_url，并清理无引用的旧对象。可指定 user_id 处理单个用户，不传则处理全部并扫描对象存储；clear_invalid=true 时无法处理的头像会被清空。
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param body body dto.NormalizeAvatarsReq false "可选筛选条件"
// @Success 200 {object} response.Response{data=dto.NormalizeAvatarsResp} "统一响应；code=0 表示任务执行完成（含部分失败明细）"
// @Failure 400 {object} response.Response "参数错误，如 limit 超出上限"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 404 {object} response.Response "指定用户不存在"
// @Failure 429 {object} response.Response "请求过于频繁"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/users/avatars/normalize [post]
func (h *UserAdminHandler) NormalizeAvatars(c *gin.Context) {
	var req dto.NormalizeAvatarsReq
	if c.Request.ContentLength > 0 {
		if !reqbind.JSON(c, &req) {
			return
		}
	}

	resp, err := h.svc.NormalizeAvatars(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, userservice.ErrUserNotFound) {
			response.NotFound(c)
			return
		}
		if errors.Is(err, userservice.ErrAvatarNormalizeUnavailable) {
			response.ServerError(c)
			return
		}
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.Success(c, resp)
}

// ClearUserAvatar 清除指定用户的本站托管头像。
// @Summary 清除用户头像
// @Description 管理员清空目标用户 avatar_url，并在无其他引用时删除对象存储中的头像文件。
// @Tags 用户管理
// @Accept json
// @Produce json
// @Param id path int true "目标用户 ID"
// @Success 200 {object} response.Response{data=dto.ClearUserAvatarResp} "统一响应；code=0 表示清除成功"
// @Failure 400 {object} response.Response "目标头像不是本站托管对象"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 404 {object} response.Response "目标用户不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/users/{id}/avatar/clear [post]
func (h *UserAdminHandler) ClearUserAvatar(c *gin.Context) {
	targetUserID, ok := reqbind.PathUint(c, "id", "用户 ID")
	if !ok {
		return
	}
	resp, err := h.svc.ClearUserAvatar(c.Request.Context(), targetUserID)
	if err != nil {
		if errors.Is(err, userservice.ErrUserNotFound) {
			response.NotFound(c)
			return
		}
		if errors.Is(err, userservice.ErrAvatarNotManaged) {
			response.Fail(c, response.CodeBadRequest, err.Error())
			return
		}
		if errors.Is(err, userservice.ErrAvatarNormalizeUnavailable) {
			response.ServerError(c)
			return
		}
		response.ServerError(c)
		return
	}
	h.recordLog(c, adminlog.ActionClearAvatar, targetUserID, nil)
	response.Success(c, resp)
}

// DisableAccount 禁用目标用户账号登录。
// @Summary 禁用用户账号
// @Description 管理员禁用目标账号登录；不能禁用自己，不能禁用系统里最后一个管理员。
// @Tags 用户管理
// @Produce json
// @Param id path int true "目标用户 ID"
// @Success 200 {object} response.Response "成功；code != 0 表示业务失败（如最后一个管理员）"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 404 {object} response.Response "目标用户不存在"
// @Router /admin/users/{id}/disable [post]
func (h *UserAdminHandler) DisableAccount(c *gin.Context) {
	targetUserID, ok := reqbind.PathUint(c, "id", "用户 ID")
	if !ok {
		return
	}
	operator := middleware.GetUserDetail(c)
	if operator == nil {
		response.Unauthorized(c)
		return
	}
	err := h.svc.DisableAccount(operator.ID, targetUserID)
	if err != nil {
		switch {
		case errors.Is(err, userservice.ErrUserNotFound):
			response.NotFound(c)
		case errors.Is(err, userservice.ErrCannotDisableSelf), errors.Is(err, userservice.ErrLastAdminAccount):
			response.Fail(c, response.CodeBadRequest, err.Error())
		default:
			response.ServerError(c)
		}
		return
	}
	h.recordLog(c, adminlog.ActionDisableAccount, targetUserID, nil)
	response.Success(c, nil)
}

// EnableAccount 启用目标用户账号登录。
// @Summary 启用用户账号
// @Tags 用户管理
// @Produce json
// @Param id path int true "目标用户 ID"
// @Success 200 {object} response.Response "成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "需要管理员权限"
// @Failure 404 {object} response.Response "目标用户不存在"
// @Router /admin/users/{id}/enable [post]
func (h *UserAdminHandler) EnableAccount(c *gin.Context) {
	targetUserID, ok := reqbind.PathUint(c, "id", "用户 ID")
	if !ok {
		return
	}
	err := h.svc.EnableAccount(targetUserID)
	if err == nil {
		h.recordLog(c, adminlog.ActionEnableAccount, targetUserID, nil)
	}
	writeUserAdminResponse(c, nil, err)
}

// ListAdmin 管理端分页查询用户，支持关键词/角色/状态筛选。
// @Summary 管理端查询用户列表
// @Tags 用户管理
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param keyword query string false "关键词，匹配用户名/昵称/邮箱"
// @Param role query string false "角色筛选：ROLE_ADMIN/ROLE_VIP/ROLE_NORMAL"
// @Param status query string false "账号状态：active/disabled"
// @Success 200 {object} response.Response{data=dto.AdminUserPageResp}
// @Router /admin/users [get]
func (h *UserAdminHandler) ListAdmin(c *gin.Context) {
	var req dto.AdminUserListReq
	if !reqbind.Query(c, &req) {
		return
	}
	resp, err := h.svc.ListAdmin(&req)
	writeUserAdminResponse(c, resp, err)
}

// GetDetail 管理端查询用户详情。
// @Summary 管理端查询用户详情
// @Tags 用户管理
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} response.Response{data=dto.AdminUserDetailResp}
// @Failure 404 {object} response.Response "用户不存在"
// @Router /admin/users/{id} [get]
func (h *UserAdminHandler) GetDetail(c *gin.Context) {
	targetUserID, ok := reqbind.PathUint(c, "id", "用户 ID")
	if !ok {
		return
	}
	resp, err := h.svc.GetAdminDetail(targetUserID)
	writeUserAdminResponse(c, resp, err)
}

// GetOperationLogs 查询目标用户的管理员操作日志。
// @Summary 查询用户操作日志
// @Tags 用户管理
// @Produce json
// @Param id path int true "用户 ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} response.Response{data=dto.AdminOperationLogPageResp}
// @Router /admin/users/{id}/operation-logs [get]
func (h *UserAdminHandler) GetOperationLogs(c *gin.Context) {
	targetUserID, ok := reqbind.PathUint(c, "id", "用户 ID")
	if !ok {
		return
	}
	var req struct {
		Page     int `form:"page" binding:"omitempty,min=1"`
		PageSize int `form:"page_size" binding:"omitempty,min=1,max=50"`
	}
	if !reqbind.Query(c, &req) {
		return
	}
	resp, err := h.svc.GetOperationLogs(targetUserID, req.Page, req.PageSize)
	writeUserAdminResponse(c, resp, err)
}

func (h *UserAdminHandler) logVipRoleChange(c *gin.Context, action string, targetUserID uint) {
	fields := []zap.Field{
		zap.String("action", action),
		zap.Uint("target_user_id", targetUserID),
	}
	if operator := middleware.GetUserDetail(c); operator != nil {
		fields = append(fields, zap.Uint("operator_user_id", operator.ID))
	}
	h.log.Info("调整用户 VIP 角色", fields...)
}

func (h *UserAdminHandler) recordLog(c *gin.Context, action adminlog.Action, targetUserID uint, detail map[string]any) {
	if h.logRecorder == nil {
		return
	}
	operator := middleware.GetUserDetail(c)
	if operator == nil {
		return
	}
	if err := h.logRecorder.Record(c.Request.Context(), operator.ID, targetUserID, action, detail); err != nil {
		h.log.Warn("记录管理员操作日志失败", zap.Error(err), zap.String("action", string(action)))
	}
}
