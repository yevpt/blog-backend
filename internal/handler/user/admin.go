package user

import (
	"errors"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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
