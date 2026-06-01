package comment

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/pkg/response"
)

// Create 新增一级评论。
// @Summary 新增一级评论
// @Description 登录用户为文章、说说或留言板新增一级评论；目标关闭评论时会拒绝提交。
// @Tags 评论
// @Accept json
// @Produce json
// @Param request body dto.CommentCreateReq true "评论新增请求"
// @Success 200 {object} response.Response{data=dto.CommentItemResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误或评论关闭"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "评论目标不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /comments [post]
func (h *CommentHandler) Create(c *gin.Context) {
	userID, _, ok := requiredCommentClaims(c)
	if !ok {
		return
	}

	var req dto.CommentCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}

	resp, err := h.svc.Create(req, userID)
	writeCommentResponse(c, resp, err)
}

// Reply 新增评论回复。
// @Summary 新增评论回复
// @Description 登录用户回复一级评论或回复；parent_reply_id 为 0 时直接回复一级评论。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "一级评论 ID"
// @Param request body dto.CommentReplyCreateReq true "回复新增请求"
// @Success 200 {object} response.Response{data=dto.CommentReplyResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "评论或回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /comments/{id}/replies [post]
func (h *CommentHandler) Reply(c *gin.Context) {
	userID, _, ok := requiredCommentClaims(c)
	if !ok {
		return
	}
	commentID, ok := bindCommentID(c, "id")
	if !ok {
		return
	}

	var req dto.CommentReplyCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}

	resp, err := h.svc.Reply(commentID, req, userID)
	writeCommentResponse(c, resp, err)
}

// Delete 删除一级评论。
// @Summary 删除一级评论
// @Description 评论作者可删除自己的一级评论；管理员可删除任意评论，同时删除其下回复。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "一级评论 ID"
// @Param target_type query string true "目标类型：article、moment、guestbook"
// @Success 200 {object} response.Response{data=dto.CommentDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权删除评论"
// @Failure 404 {object} response.Response "评论不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /comments/{id} [delete]
func (h *CommentHandler) Delete(c *gin.Context) {
	userID, roleNames, ok := requiredCommentClaims(c)
	if !ok {
		return
	}
	commentID, ok := bindCommentID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.DeleteComment(c.Query("target_type"), commentID, userID, roleNames)
	writeCommentResponse(c, resp, err)
}

// DeleteReply 删除评论回复。
// @Summary 删除评论回复
// @Description 回复作者可删除自己的回复；管理员可删除任意回复。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "回复 ID"
// @Success 200 {object} response.Response{data=dto.CommentDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权删除回复"
// @Failure 404 {object} response.Response "回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /comment-replies/{id} [delete]
func (h *CommentHandler) DeleteReply(c *gin.Context) {
	userID, roleNames, ok := requiredCommentClaims(c)
	if !ok {
		return
	}
	replyID, ok := bindCommentID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.DeleteReply(replyID, userID, roleNames)
	writeCommentResponse(c, resp, err)
}
