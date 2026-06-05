package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/internal/service"
	"github.com/vpt/blog-backend/pkg/response"
)

// UserHandler 用户资料 HTTP 入口，只负责读取登录态和写统一响应。
type UserHandler struct {
	svc service.UserService
}

// NewUserHandler 创建用户资料处理器。
func NewUserHandler(svc service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// GetDetail 返回当前登录用户完整资料。
// Auth 中间件已从 Redis 加载 UserDetail 并写入 Context，此处直接读取。
// @Summary 查询当前登录用户详情
// @Description 返回当前 access token 对应用户的完整资料、角色、扩展信息、偏好设置和社交链接。
// @Tags 用户
// @Accept json
// @Produce json
// @Success 200 {object} response.Response{data=dto.UserDetailResp} "统一响应；code=0 表示查询成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Router /users/me [get]
func (h *UserHandler) GetDetail(c *gin.Context) {
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	response.Success(c, detail)
}
