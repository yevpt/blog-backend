package user

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/middleware"
	userservice "github.com/vpt/blog-backend/internal/service/user"
	"github.com/vpt/blog-backend/pkg/response"
)

// maxPresenceIDs 是 /users/presence 单次查询接受的最大 id 数量（前端订阅集硬上限同为 100，这里是双保险）。
const maxPresenceIDs = 100

// UserHandler 用户资料 HTTP 入口，只负责读取登录态和写统一响应。
type UserHandler struct {
	svc      userservice.UserService
	moments  UserMomentsCounter
	presence userservice.PresenceProvider
}

// UserMomentsCounter 供个人页 Tab 展示碎语总数，避免 handler 依赖完整 MomentService。
type UserMomentsCounter interface {
	CountByUser(userID uint) (*dto.UserMomentsCountResp, error)
}

// NewUserHandler 创建用户资料处理器。
func NewUserHandler(svc userservice.UserService, moments UserMomentsCounter, presence userservice.PresenceProvider) *UserHandler {
	return &UserHandler{svc: svc, moments: moments, presence: presence}
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
	// 在线态来自 Redis，需经 service 实时 enrichment（profile 缓存不含 is_online）。
	enriched, err := h.svc.GetDetail(detail.ID)
	if err != nil || enriched == nil {
		response.Unauthorized(c)
		return
	}
	response.Success(c, enriched)
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

// BatchPresence 批量查询用户在线感知
// @Summary 批量查询用户在线感知
// @Description 公开接口；按 ids 批量返回在线状态与最近活跃/登录时间，最多 100 个 id，超出截断，重复去重，非数字静默丢弃；未知 id 在 data 中整条缺席
// @Tags 用户
// @Accept json
// @Produce json
// @Param ids query string false "用户 ID 列表，逗号分隔，最多 100 个"
// @Success 200 {object} response.Response{data=dto.BatchPresenceResp} "成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /users/presence [get]
func (h *UserHandler) BatchPresence(c *gin.Context) {
	ids := parsePresenceIDs(c.Query("ids"))
	if len(ids) == 0 {
		response.Success(c, dto.BatchPresenceResp{Data: map[uint]dto.UserPresenceResp{}})
		return
	}

	data, err := h.presence.BatchPresence(c.Request.Context(), ids)
	if err != nil {
		response.ServerError(c)
		return
	}

	resp := dto.BatchPresenceResp{Data: make(map[uint]dto.UserPresenceResp, len(data))}
	for id, item := range data {
		resp.Data[id] = *item
	}
	response.Success(c, resp)
}

// parsePresenceIDs 解析 ids 查询参数：逗号分隔、去重、丢弃非数字、截断到 maxPresenceIDs。
func parsePresenceIDs(raw string) []uint {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[uint]struct{}, len(parts))
	ids := make([]uint, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64)
		if err != nil {
			continue
		}
		id := uint(v)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) >= maxPresenceIDs {
			break
		}
	}
	return ids
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
// @Description 更新当前登录用户的昵称、标签等信息；头像请使用 POST /users/me/avatar。
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.UserUpdateReq true "更新信息"
// @Success 200 {object} response.Response{data=dto.UserDetailResp} "统一响应；code=0 表示更新成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 500 {object} response.Response "服务器内部错误"
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
	resp, err := h.svc.Update(detail.ID, &req)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
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

// ListLikedContent 按用户 ID 返回其公开点赞内容。
// @Summary 获取用户点赞内容
// @Description 公开分页查询指定用户赞过的文章、评论、留言、碎语和回复；type=comment 包含评论与回复。
// @Tags 用户
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量，最大 50" default(20)
// @Param type query string false "筛选类型：article/comment/guestbook/moment；comment 包含评论与回复"
// @Success 200 {object} response.Response{data=dto.UserLikedContentPageResp} "统一响应；code=0 表示查询成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /users/{id}/likes [get]
func (h *UserHandler) ListLikedContent(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, response.CodeBadRequest, "无效的用户 ID")
		return
	}

	var req dto.UserLikedContentListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	resp, err := h.svc.ListLikedContent(uint(id), req)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// CountLikedContent 按用户 ID 返回其公开点赞内容总数。
// @Summary 获取用户点赞总数
// @Description 公开查询指定用户赞过且当前仍公开可见的内容总数，统计口径与 GET /users/{id}/likes 一致。
// @Tags 用户
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} response.Response{data=dto.UserLikedContentCountResp} "统一响应；code=0 表示查询成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /users/{id}/likes/count [get]
func (h *UserHandler) CountLikedContent(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, response.CodeBadRequest, "无效的用户 ID")
		return
	}

	resp, err := h.svc.CountLikedContent(uint(id))
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// CountMoments 按用户 ID 返回其公开发布的碎语总数。
// @Summary 获取用户碎语总数
// @Description 公开查询指定用户发布的公开碎语总数，统计口径与 GET /moments?user_id= 一致。
// @Tags 用户
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} response.Response{data=dto.UserMomentsCountResp} "统一响应；code=0 表示查询成功"
// @Failure 400 {object} response.Response "无效的用户 ID"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /users/{id}/moments/count [get]
func (h *UserHandler) CountMoments(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, response.CodeBadRequest, "无效的用户 ID")
		return
	}

	resp, err := h.moments.CountByUser(uint(id))
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
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
		if errors.Is(err, userservice.ErrUsernameExists) {
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
		if errors.Is(err, userservice.ErrWrongPassword) {
			response.Fail(c, response.CodeBadRequest, err.Error())
			return
		}
		response.ServerError(c)
		return
	}
	response.Success(c, nil)
}

// SetInitialPassword 使用当前主邮箱验证码设置登录密码。
// @Summary 设置初始密码
// @Description 适用于 OAuth 注册等没有用户自设密码的账号；需先向当前主邮箱发送验证码。
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.SetInitialPasswordReq true "密码和验证码"
// @Success 200 {object} response.Response "成功；code != 0 表示业务失败"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Router /users/me/password/initial [patch]
func (h *UserHandler) SetInitialPassword(c *gin.Context) {
	var req dto.SetInitialPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.SetInitialPassword(detail.ID, req.NewPassword, req.Code); err != nil {
		writeUserSecurityError(c, err)
		return
	}
	response.Success(c, nil)
}

// SendEmailCode 向账号安全场景的目标邮箱发送验证码。
// @Summary 发送账号邮箱验证码
// @Description 用于主邮箱/副邮箱绑定换绑，以及当前主邮箱校验后设置初始密码。
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.SendAccountEmailCodeReq true "目标邮箱和图形验证码票据"
// @Success 200 {object} response.Response "成功；code != 0 表示业务失败"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 429 {object} response.Response "发送过于频繁"
// @Router /users/me/email/code [post]
func (h *UserHandler) SendEmailCode(c *gin.Context) {
	var req dto.SendAccountEmailCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.SendEmailCode(detail.ID, req.Email, req.CaptchaToken, c.ClientIP()); err != nil {
		writeUserSecurityError(c, err)
		return
	}
	response.Success(c, nil)
}

// UpdateEmail 绑定或换绑当前用户的主邮箱/副邮箱。
// @Summary 更新账号邮箱
// @Description target=main 更新主邮箱，target=sub 更新副邮箱；邮箱验证码必须匹配目标邮箱。
// @Tags 用户
// @Accept json
// @Produce json
// @Param req body dto.UpdateEmailReq true "邮箱更新信息"
// @Success 200 {object} response.Response "成功；code != 0 表示业务失败"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Router /users/me/email [patch]
func (h *UserHandler) UpdateEmail(c *gin.Context) {
	var req dto.UpdateEmailReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	detail := middleware.GetUserDetail(c)
	if detail == nil {
		response.Unauthorized(c)
		return
	}
	if err := h.svc.UpdateEmail(detail.ID, req.Target, req.Email, req.Code); err != nil {
		writeUserSecurityError(c, err)
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

func writeUserSecurityError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, userservice.ErrWrongPassword),
		errors.Is(err, userservice.ErrUsernameExists),
		errors.Is(err, userservice.ErrEmailTaken),
		errors.Is(err, userservice.ErrInvalidEmailCode),
		errors.Is(err, userservice.ErrEmailRequired),
		errors.Is(err, userservice.ErrSecurityDisabled),
		errors.Is(err, userservice.ErrPasswordAlreadySet):
		response.Fail(c, response.CodeBadRequest, err.Error())
	case errors.Is(err, userservice.ErrTooManyEmailCodeRequests):
		retryAfter := 60
		msg := fmt.Sprintf("发送过于频繁，请在 %s 后重试", response.FormatRetryAfter(retryAfter))
		response.TooManyRequests(c, msg, retryAfter)
	default:
		response.ServerError(c)
	}
}
