package moderation

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	moderationservice "github.com/vpt/blog-backend/internal/service/moderation"
)

// List 查询审核版本。
// @Summary 查询内容审核队列
// @Description 管理员按审核状态、内容类型和风险等级分页查询审核版本，默认只返回待审版本。
// @Tags 内容审核管理
// @Accept json
// @Produce json
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 20，最大 100"
// @Param content_type query string false "内容类型"
// @Param risk_level query string false "风险：low、medium、high"
// @Param review_status query string false "状态：pending、approved、rejected、superseded、all（全部）"
// @Param public_state query string false "公开状态：visible、placeholder、hidden、emergency_hidden"
// @Success 200 {object} response.Response{data=dto.AdminModerationPageResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/items [get]
func (h *AdminHandler) List(c *gin.Context) {
	var req dto.AdminModerationListReq
	if !reqbind.Query(c, &req) {
		return
	}
	cmd := moderationservice.ListReviewCommand{Page: req.Page, PageSize: req.PageSize}
	if req.ContentType != "" {
		value := moderationservice.SubjectType(req.ContentType)
		cmd.ContentType = &value
	}
	if req.RiskLevel != "" {
		value := moderationservice.RiskLevel(req.RiskLevel)
		cmd.RiskLevel = &value
	}
	switch req.ReviewStatus {
	case "", "pending":
		pending := moderationservice.ReviewPending
		cmd.ReviewStatus = &pending
	case "all":
		cmd.IncludeAllReviewStatuses = true
	default:
		value := moderationservice.ReviewStatus(req.ReviewStatus)
		cmd.ReviewStatus = &value
	}
	if req.PublicState != "" {
		value := moderationservice.PublicState(req.PublicState)
		cmd.PublicState = &value
	}
	page, err := h.svc.List(c.Request.Context(), cmd)
	writeAdminResponse(c, moderationPageToDTO(page), err)
}

// Get 查询审核项当前版本。
// @Summary 查询内容审核详情
// @Description 优先返回当前待审版本；没有待审版本时返回最新历史版本。
// @Tags 内容审核管理
// @Produce json
// @Param id path int true "审核项 ID"
// @Success 200 {object} response.Response{data=dto.AdminModerationItemResp}
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/moderation/items/{id} [get]
func (h *AdminHandler) Get(c *gin.Context) {
	itemID, ok := reqbind.PathUint(c, "id", "审核项 ID")
	if !ok {
		return
	}
	item, err := h.svc.Get(c.Request.Context(), uint64(itemID))
	writeAdminResponse(c, moderationItemToDTO(item), err)
}
