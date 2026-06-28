package moderation

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
)

// Approve 通过待审版本。
// @Summary 通过待审内容
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param id path int true "审核项 ID"
// @Param request body dto.AdminModerationReviewReq true "通过请求"
// @Success 200 {object} response.Response{data=dto.AdminModerationItemResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 409 {object} response.Response "版本冲突或内容已删除"
// @Failure 500 {object} response.Response
// @Router /admin/moderation/items/{id}/approve [post]
func (h *AdminHandler) Approve(c *gin.Context) {
	itemID, reviewerID, req, ok := bindReviewRequest(c)
	if !ok {
		return
	}
	item, err := h.svc.Approve(c.Request.Context(), moderationservice.ReviewCommand{
		ItemID: itemID, RevisionID: req.RevisionID, ExpectedLockVersion: req.LockVersion,
		ReviewerID: reviewerID, Reason: req.Reason,
	})
	writeAdminResponse(c, moderationItemToDTO(item), err)
}

// Correct 修正正文后通过待审版本。
// @Summary 修正并通过待审内容
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param id path int true "审核项 ID"
// @Param request body dto.AdminModerationCorrectReq true "修正请求"
// @Success 200 {object} response.Response{data=dto.AdminModerationItemResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 409 {object} response.Response "版本冲突或内容已删除"
// @Failure 500 {object} response.Response
// @Router /admin/moderation/items/{id}/correct [post]
func (h *AdminHandler) Correct(c *gin.Context) {
	itemID, ok := bindItemID(c)
	if !ok {
		return
	}
	reviewerID, ok := requiredReviewerID(c)
	if !ok {
		return
	}
	var req dto.AdminModerationCorrectReq
	if !reqbind.JSON(c, &req) {
		return
	}
	item, err := h.svc.Correct(c.Request.Context(), moderationservice.CorrectCommand{
		ReviewCommand: moderationservice.ReviewCommand{
			ItemID: itemID, RevisionID: req.RevisionID, ExpectedLockVersion: req.LockVersion,
			ReviewerID: reviewerID, Reason: req.Reason,
		},
		Content: req.Content,
	})
	writeAdminResponse(c, moderationItemToDTO(item), err)
}

// Reject 驳回待审版本。
// @Summary 驳回待审内容
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param id path int true "审核项 ID"
// @Param request body dto.AdminModerationReviewReq true "驳回请求，reason 必填"
// @Success 200 {object} response.Response{data=dto.AdminModerationItemResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 409 {object} response.Response "版本冲突或内容已删除"
// @Failure 500 {object} response.Response
// @Router /admin/moderation/items/{id}/reject [post]
func (h *AdminHandler) Reject(c *gin.Context) {
	itemID, reviewerID, req, ok := bindReviewRequest(c)
	if !ok {
		return
	}
	item, err := h.svc.Reject(c.Request.Context(), moderationservice.ReviewCommand{
		ItemID: itemID, RevisionID: req.RevisionID, ExpectedLockVersion: req.LockVersion,
		ReviewerID: reviewerID, Reason: req.Reason,
	})
	writeAdminResponse(c, moderationItemToDTO(item), err)
}

func bindReviewRequest(c *gin.Context) (uint64, uint64, dto.AdminModerationReviewReq, bool) {
	itemID, ok := bindItemID(c)
	if !ok {
		return 0, 0, dto.AdminModerationReviewReq{}, false
	}
	reviewerID, ok := requiredReviewerID(c)
	if !ok {
		return 0, 0, dto.AdminModerationReviewReq{}, false
	}
	var req dto.AdminModerationReviewReq
	if !reqbind.JSON(c, &req) {
		return 0, 0, dto.AdminModerationReviewReq{}, false
	}
	return itemID, reviewerID, req, true
}
