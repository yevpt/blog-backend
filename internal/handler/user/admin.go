package user

import (
	"errors"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	"github.com/vpt/blog-backend/internal/middleware"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/response"
)

// UserAdminHandler 用户管理端 HTTP 处理器。
type UserAdminHandler struct {
	svc userservice.AdminService
	log *zap.Logger
}

// NewUserAdminHandler 创建用户管理端处理器。
func NewUserAdminHandler(svc userservice.AdminService, log *zap.Logger) *UserAdminHandler {
	return &UserAdminHandler{svc: svc, log: log}
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
	}
	writeUserAdminResponse(c, resp, err)
}

// NormalizeAvatars 检查并重压缩不符合规范的老用户头像。
// @Summary 归一化老用户头像
// @Description 管理员检查本站托管头像是否超出 120px / 20KB JPEG 规范；超出则压缩替换、更新 avatar_url，并清理无引用的旧对象。可指定 user_id 处理单个用户，不传则处理全部并扫描对象存储；clear_invalid=true 时无法处理的头像会被清空。
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
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, response.CodeBadRequest, "参数错误")
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
	response.Success(c, resp)
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
