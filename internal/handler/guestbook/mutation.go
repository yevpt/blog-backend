package guestbook

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// Create 发表留言。
// @Summary 发表留言
// @Description 登录用户发表留言；owner_user_id 可省略，默认给博主 1 留言。
// @Tags 留言板
// @Accept json
// @Produce json
// @Param request body dto.GuestbookCreateReq true "留言发表请求"
// @Param Idempotency-Key header string true "幂等键，重试时保持不变"
// @Success 200 {object} response.Response{data=dto.GuestbookItemResp} "统一响应；code=0 表示发表成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "留言板主人不存在"
// @Failure 409 {object} response.Response "图片审核未启用或内容状态冲突"
// @Failure 422 {object} response.Response "内容存在风险，拒绝发布"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /guestbook [post]
func (h *GuestbookHandler) Create(c *gin.Context) {
	userID, _, ok := requiredUser(c)
	if !ok {
		return
	}

	var req dto.GuestbookCreateReq
	if !reqbind.JSON(c, &req) {
		return
	}
	key, ok := reqbind.IdempotencyKeyIf(c, h.requireModerationWrite)
	if !ok {
		return
	}
	req.IdempotencyKey = key

	resp, err := h.svc.Create(req, userID)
	writeGuestbookResponse(c, resp, err)
}

// Edit 编辑留言。
// @Summary 编辑留言
// @Tags 留言板
// @Accept json
// @Produce json
// @Param id path int true "留言 ID"
// @Param request body dto.GuestbookCreateReq true "留言编辑请求"
// @Param Idempotency-Key header string true "幂等键，重试时保持不变"
// @Success 200 {object} response.Response{data=dto.GuestbookItemResp}
// @Failure 409 {object} response.Response
// @Failure 422 {object} response.Response
// @Router /guestbook/{id} [patch]
func (h *GuestbookHandler) Edit(c *gin.Context) {
	userID, roleNames, ok := requiredUser(c)
	if !ok {
		return
	}
	id, ok := bindGuestbookID(c, "id")
	if !ok {
		return
	}
	var req dto.GuestbookCreateReq
	if !reqbind.JSON(c, &req) {
		return
	}
	key, ok := reqbind.IdempotencyKeyIf(c, h.requireModerationWrite)
	if !ok {
		return
	}
	req.IdempotencyKey = key
	resp, err := h.svc.Edit(id, req, userID, roleNames)
	writeGuestbookResponse(c, resp, err)
}

// ToggleLike 切换留言点赞状态。
// @Summary 切换留言点赞
// @Description 当前用户未点赞时点赞，已点赞时取消点赞。
// @Tags 留言板
// @Accept json
// @Produce json
// @Param id path int true "留言 ID"
// @Success 200 {object} response.Response{data=dto.GuestbookLikeResp} "统一响应；code=0 表示切换成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "留言不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /guestbook/{id}/like [post]
func (h *GuestbookHandler) ToggleLike(c *gin.Context) {
	userID, _, ok := requiredUser(c)
	if !ok {
		return
	}
	id, ok := bindGuestbookID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.ToggleLike(id, userID)
	writeGuestbookResponse(c, resp, err)
}

// Delete 删除留言。
// @Summary 删除留言
// @Description 留言作者、留言板主人或管理员可删除留言。
// @Tags 留言板
// @Accept json
// @Produce json
// @Param id path int true "留言 ID"
// @Success 200 {object} response.Response{data=dto.GuestbookDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权删除留言"
// @Failure 404 {object} response.Response "留言不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /guestbook/{id} [delete]
func (h *GuestbookHandler) Delete(c *gin.Context) {
	userID, roleNames, ok := requiredUser(c)
	if !ok {
		return
	}
	id, ok := bindGuestbookID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.Delete(id, userID, roleNames)
	writeGuestbookResponse(c, resp, err)
}
