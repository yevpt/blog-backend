package handler

import (
	"errors"
	"strconv"

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

// GetPublicProfile 按 ID 返回某用户的公开详情。
// @Summary 获取用户公开详情
// @Tags 用户
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} response.Response{data=dto.UserPublicProfileResp} "成功"
// @Failure 404 {object} response.Response "用户不存在"
// @Router /users/{id} [get]
func (h *UserHandler) GetPublicProfile(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, response.CodeBadRequest, "无效的用户 ID")
		return
	}
	profile, err := h.svc.GetPublicProfile(uint(id))
	if err != nil {
		response.ServerError(c)
		return
	}
	if profile == nil {
		response.NotFound(c)
		return
	}
	response.Success(c, profile)
}

// UpdateProfile 更新当前用户昵称、身份标签、个人简介。
// @Summary 更新用户基本资料
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.UpdateProfileReq true "更新资料"
// @Success 200 {object} response.Response{data=dto.UserDetailResp} "成功"
// @Router /users/me/profile [patch]
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	var req dto.UpdateProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	resp, err := h.svc.UpdateProfile(detail.ID, &req)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// UpdateMeta 更新当前用户扩展信息（性别、生日、手机号）。
// @Summary 更新用户扩展信息
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.UpdateMetaReq true "扩展信息"
// @Success 200 {object} response.Response{data=dto.UserDetailResp} "成功"
// @Router /users/me/meta [patch]
func (h *UserHandler) UpdateMeta(c *gin.Context) {
	var req dto.UpdateMetaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	resp, err := h.svc.UpdateMeta(detail.ID, &req)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.Success(c, resp)
}

// UpdateSocialLink 更新或删除当前用户指定平台的社交链接。
// @Summary 更新社交链接
// @Tags 用户
// @Accept json
// @Produce json
// @Param platform path string true "平台标识"
// @Param req body dto.UpdateSocialLinkReq true "链接信息"
// @Success 200 {object} response.Response{data=dto.UserDetailResp} "成功"
// @Router /users/me/social/{platform} [patch]
func (h *UserHandler) UpdateSocialLink(c *gin.Context) {
	platform := c.Param("platform")
	if platform == "" {
		response.Fail(c, response.CodeBadRequest, "平台参数缺失")
		return
	}
	var req dto.UpdateSocialLinkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	resp, err := h.svc.UpdateSocialLink(detail.ID, platform, req.URL)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// UpdateUsername 修改当前用户的登录用户名。
// @Summary 修改用户名
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.UpdateUsernameReq true "新用户名"
// @Success 200 {object} response.Response "成功"
// @Router /users/me/username [patch]
func (h *UserHandler) UpdateUsername(c *gin.Context) {
	var req dto.UpdateUsernameReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.UpdateUsername(detail.ID, req.Username); err != nil {
		if errors.Is(err, service.ErrUsernameExists) {
			response.Fail(c, response.CodeBadRequest, err.Error())
			return
		}
		response.ServerError(c)
		return
	}
	response.Success(c, nil)
}

// UpdatePassword 修改当前用户密码。
// @Summary 修改密码
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.UpdatePasswordReq true "密码信息"
// @Success 200 {object} response.Response "成功"
// @Router /users/me/password [patch]
func (h *UserHandler) UpdatePassword(c *gin.Context) {
	var req dto.UpdatePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.UpdatePassword(detail.ID, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrWrongPassword) {
			response.Fail(c, response.CodeBadRequest, err.Error())
			return
		}
		response.ServerError(c)
		return
	}
	response.Success(c, nil)
}

// UpdateEmailDisplay 设置对外展示邮箱（主邮箱/副邮箱/不展示）。
// @Summary 设置展示邮箱
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.EmailDisplayReq true "展示设置"
// @Success 200 {object} response.Response "成功"
// @Router /users/me/email/display [patch]
func (h *UserHandler) UpdateEmailDisplay(c *gin.Context) {
	var req dto.EmailDisplayReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.UpdateEmailDisplay(detail.ID, req.Display); err != nil {
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
