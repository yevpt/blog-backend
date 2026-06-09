package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
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

// ListRecent 获取最近访问用户列表
// @Summary 获取最近访问用户列表
// @Description 默认按最后登录时间降序，支持分页
// @Tags 用户
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} response.Response{data=dto.UserPageResp} "成功"
// @Router /users/recent [get]
func (h *UserHandler) ListRecent(c *gin.Context) {
	var req dto.UserListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	resp, err := h.svc.ListRecent(&req)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// ListAll 获取全部用户列表
// @Summary 获取全部用户列表
// @Description 支持分页，按角色权限排序优先，然后按最后登录时间降序
// @Tags 用户
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} response.Response{data=dto.UserPageResp} "成功"
// @Router /users [get]
func (h *UserHandler) ListAll(c *gin.Context) {
	var req dto.UserListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	resp, err := h.svc.ListAll(&req)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// Update 更新当前用户信息
// @Summary 更新当前用户信息
// @Description 更新当前登录用户的昵称、头像、标签等信息
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.UserUpdateReq true "更新信息"
// @Success 200 {object} response.Response "成功"
// @Router /users/me [put]
func (h *UserHandler) Update(c *gin.Context) {
	var req dto.UserUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.Update(detail.ID, &req); err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, nil)
}

// RecordLogin 记录当前用户登录时间
// @Summary 记录当前用户登录时间
// @Description 从 jwt 中获取当前用户信息，更新最后登录时间
// @Tags 用户
// @Accept json
// @Produce json
// @Success 200 {object} response.Response "成功"
// @Router /users/me/login-time [post]
func (h *UserHandler) RecordLogin(c *gin.Context) {
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.RecordLogin(detail.ID); err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, nil)
}
