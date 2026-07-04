package moderation

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	"github.com/vpt/blog-backend/internal/service/adminlog"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/response"
)

// GetControl 查询全站审核控制。
// @Summary 查询全站审核控制
// @Tags 内容审核管理
// @Produce json
// @Success 200 {object} response.Response{data=dto.AdminModerationControlResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/control [get]
func (h *AdminHandler) GetControl(c *gin.Context) {
	control, err := h.ops.GetControl(c.Request.Context())
	writeOperationsResponse(c, controlToDTO(control), err)
}

// UpdateControl 更新全站注册和发布控制。
// @Summary 更新全站审核控制
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param request body dto.AdminModerationControlReq true "控制状态"
// @Success 200 {object} response.Response{data=dto.AdminModerationControlResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 409 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/control [patch]
func (h *AdminHandler) UpdateControl(c *gin.Context) {
	actorID, ok := requiredReviewerID(c)
	if !ok {
		return
	}
	var req dto.AdminModerationControlReq
	if !reqbind.JSON(c, &req) {
		return
	}
	control, err := h.ops.UpdateControl(c.Request.Context(), moderationservice.UpdateControlCommand{
		RegistrationMode: moderationservice.RegistrationMode(req.RegistrationMode),
		PublishingMode:   moderationservice.PublishingMode(req.PublishingMode), Reason: req.Reason,
		OperatorID: actorID, ExpectedLockVersion: req.LockVersion,
	})
	writeOperationsResponse(c, controlToDTO(control), err)
}

// GetUserProfile 查询用户审核画像。
// @Summary 查询用户审核画像
// @Tags 内容审核管理
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} response.Response{data=dto.AdminModerationProfileResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/users/{id} [get]
func (h *AdminHandler) GetUserProfile(c *gin.Context) {
	userID, ok := bindUserID(c)
	if !ok {
		return
	}
	profile, err := h.ops.GetUserProfile(c.Request.Context(), userID)
	writeOperationsResponse(c, profileToDTO(profile), err)
}

// UpdateUserProfile 手工校正用户信任等级。
// @Summary 校正用户审核等级
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Param request body dto.AdminModerationProfileReq true "信任等级"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/users/{id}/profile [patch]
func (h *AdminHandler) UpdateUserProfile(c *gin.Context) {
	userID, ok := bindUserID(c)
	if !ok {
		return
	}
	var req dto.AdminModerationProfileReq
	if !reqbind.JSON(c, &req) {
		return
	}
	actorID, ok := requiredReviewerID(c)
	if !ok {
		return
	}
	err := h.ops.SetUserTrust(c.Request.Context(), moderationservice.SetTrustCommand{
		UserID: userID, ActorID: actorID, TrustLevel: moderationservice.TrustLevel(req.TrustLevel),
		ManualLocked: *req.ManualLocked, RestrictedUntil: req.RestrictedUntil,
	})
	if err == nil && h.logRecorder != nil {
		_ = h.logRecorder.Record(c.Request.Context(), uint(actorID), uint(userID), adminlog.ActionUpdateTrustLevel, map[string]any{
			"trust_level": string(req.TrustLevel),
		})
	}
	writeOperationsResponse(c, nil, err)
}

// MuteUser 禁言用户。
// @Summary 禁言用户
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Param request body dto.AdminModerationSanctionReq true "处罚期限和理由"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/users/{id}/mute [post]
func (h *AdminHandler) MuteUser(c *gin.Context) { h.setSanction(c, moderationservice.SanctionMuted) }

// BanUser 封禁用户发布能力。
// @Summary 封禁用户发布能力
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Param request body dto.AdminModerationSanctionReq true "处罚期限和理由"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/users/{id}/ban [post]
func (h *AdminHandler) BanUser(c *gin.Context) { h.setSanction(c, moderationservice.SanctionBanned) }

// ReleaseUser 解除用户处罚。
// @Summary 解除用户审核处罚
// @Tags 内容审核管理
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/users/{id}/release [post]
func (h *AdminHandler) ReleaseUser(c *gin.Context) {
	userID, ok := bindUserID(c)
	if !ok {
		return
	}
	actorID, ok := requiredReviewerID(c)
	if !ok {
		return
	}
	err := h.ops.ReleaseUserSanction(c.Request.Context(), userID, actorID)
	if err == nil && h.logRecorder != nil {
		_ = h.logRecorder.Record(c.Request.Context(), uint(actorID), uint(userID), adminlog.ActionRelease, nil)
	}
	writeOperationsResponse(c, nil, err)
}

// HideItem 紧急隐藏单条已通过内容。
// @Summary 紧急隐藏单条内容
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param id path int true "审核项 ID"
// @Param request body dto.AdminModerationEmergencyReq true "下架理由"
// @Success 200 {object} response.Response{data=dto.AdminModerationEmergencyItemResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 409 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/items/{id}/hide [post]
func (h *AdminHandler) HideItem(c *gin.Context) {
	h.itemEmergency(c, true)
}

// RestoreItem 恢复单条紧急隐藏内容。
// @Summary 恢复单条紧急隐藏内容
// @Tags 内容审核管理
// @Produce json
// @Param id path int true "审核项 ID"
// @Success 200 {object} response.Response{data=dto.AdminModerationEmergencyItemResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 409 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/items/{id}/restore [post]
func (h *AdminHandler) RestoreItem(c *gin.Context) {
	h.itemEmergency(c, false)
}

// HideUserContent 分批隐藏用户公开内容。
// @Summary 分批隐藏用户公开内容
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Param request body dto.AdminModerationEmergencyBatchReq true "游标、数量和理由"
// @Success 200 {object} response.Response{data=dto.AdminModerationEmergencyBatchResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/users/{id}/hide-content [post]
func (h *AdminHandler) HideUserContent(c *gin.Context) { h.userEmergency(c, true) }

// RestoreUserContent 分批恢复用户紧急隐藏内容。
// @Summary 分批恢复用户紧急隐藏内容
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Param request body dto.AdminModerationEmergencyBatchReq true "游标和数量"
// @Success 200 {object} response.Response{data=dto.AdminModerationEmergencyBatchResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/users/{id}/restore-content [post]
func (h *AdminHandler) RestoreUserContent(c *gin.Context) { h.userEmergency(c, false) }

func (h *AdminHandler) setSanction(c *gin.Context, state moderationservice.SanctionState) {
	userID, ok := bindUserID(c)
	if !ok {
		return
	}
	var req dto.AdminModerationSanctionReq
	if !reqbind.JSON(c, &req) {
		return
	}
	actorID, ok := requiredReviewerID(c)
	if !ok {
		return
	}
	err := h.ops.SetUserSanction(c.Request.Context(), moderationservice.SetSanctionCommand{
		UserID: userID, ActorID: actorID, State: state, Until: req.Until, Reason: req.Reason,
	})
	if err == nil && h.logRecorder != nil {
		action := adminlog.ActionMute
		if state == moderationservice.SanctionBanned {
			action = adminlog.ActionBan
		}
		detail := map[string]any{"reason": req.Reason}
		if req.Until != nil {
			detail["until"] = *req.Until
		}
		_ = h.logRecorder.Record(c.Request.Context(), uint(actorID), uint(userID), action, detail)
	}
	writeOperationsResponse(c, nil, err)
}

func (h *AdminHandler) itemEmergency(c *gin.Context, hide bool) {
	itemID, ok := bindItemID(c)
	if !ok {
		return
	}
	actorID, ok := requiredReviewerID(c)
	if !ok {
		return
	}
	cmd := moderationservice.EmergencyItemCommand{ItemID: itemID, ActorID: actorID}
	if hide {
		var req dto.AdminModerationEmergencyReq
		if !reqbind.JSON(c, &req) {
			return
		}
		cmd.Reason = req.Reason
		item, err := h.ops.HideItem(c.Request.Context(), cmd)
		writeOperationsResponse(c, emergencyItemToDTO(item), err)
		return
	}
	item, err := h.ops.RestoreItem(c.Request.Context(), cmd)
	writeOperationsResponse(c, emergencyItemToDTO(item), err)
}

func (h *AdminHandler) userEmergency(c *gin.Context, hide bool) {
	userID, ok := bindUserID(c)
	if !ok {
		return
	}
	actorID, ok := requiredReviewerID(c)
	if !ok {
		return
	}
	var req dto.AdminModerationEmergencyBatchReq
	if !reqbind.JSON(c, &req) {
		return
	}
	cmd := moderationservice.UserEmergencyBatchCommand{
		UserID: userID, ActorID: actorID, Cursor: req.Cursor, Limit: req.Limit, Reason: req.Reason,
	}
	var result moderationservice.EmergencyBatchResult
	var err error
	if hide {
		result, err = h.ops.HideUserContent(c.Request.Context(), cmd)
	} else {
		result, err = h.ops.RestoreUserContent(c.Request.Context(), cmd)
	}
	writeOperationsResponse(c, dto.AdminModerationEmergencyBatchResp(result), err)
}

func bindUserID(c *gin.Context) (uint64, bool) {
	id, ok := reqbind.PathUint(c, "id", "用户 ID")
	return uint64(id), ok
}

func writeOperationsResponse(c *gin.Context, data any, err error) {
	switch {
	case err == nil:
		response.Success(c, data)
	case errors.Is(err, moderationservice.ErrReviewConflict), errors.Is(err, moderationrepo.ErrOptimisticLock),
		errors.Is(err, moderationservice.ErrInvalidTransition), errors.Is(err, moderationservice.ErrAlreadyDeleted):
		response.Conflict(c, response.CodeModerationReviewConflict, moderationservice.PublicErrorMessage(err))
	case errors.Is(err, moderationservice.ErrItemNotFound), errors.Is(err, moderationservice.ErrSubjectNotFound):
		response.NotFound(c)
	case errors.Is(err, moderationservice.ErrInvalidRequest):
		response.Fail(c, response.CodeBadRequest, "审核治理参数不正确")
	default:
		response.ServerError(c)
	}
}

func controlToDTO(control moderationservice.Control) dto.AdminModerationControlResp {
	return dto.AdminModerationControlResp{
		RegistrationMode: string(control.RegistrationMode), PublishingMode: string(control.PublishingMode),
		Reason: control.Reason, OperatorID: control.OperatorID, ChangedAt: control.ChangedAt, LockVersion: control.LockVersion,
	}
}

func profileToDTO(profile moderationservice.UserModerationProfile) dto.AdminModerationProfileResp {
	return dto.AdminModerationProfileResp{
		UserID: profile.UserID, TrustLevel: string(profile.TrustLevel), TrustSource: profile.TrustSource,
		ManualTrustLocked: profile.ManualTrustLocked, SanctionState: string(profile.SanctionState),
		SanctionUntil: profile.SanctionUntil, SanctionReason: profile.SanctionReason,
		CleanApprovalStreak: profile.CleanApprovalStreak, CorrectedCount: profile.CorrectedCount,
		RejectedCount: profile.RejectedCount, HighRiskCount: profile.HighRiskCount,
		ViolationScore: profile.ViolationScore, LastViolationAt: profile.LastViolationAt,
		RestrictedUntil: profile.RestrictedUntil, CreatedAt: profile.CreatedAt, UpdatedAt: profile.UpdatedAt,
	}
}

func emergencyItemToDTO(item moderationservice.EmergencyItemResult) dto.AdminModerationEmergencyItemResp {
	return dto.AdminModerationEmergencyItemResp{
		ItemID: item.ItemID, PublicState: string(item.PublicState), LockVersion: item.LockVersion,
	}
}
