package comment

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// ListArticle 分页查询文章评论。
// @Summary 分页查询文章评论
// @Description 查询指定文章的一级评论列表；登录态可返回当前用户点赞状态，回复通过独立接口懒加载。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "文章 ID"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Success 200 {object} response.Response{data=dto.CommentPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 404 {object} response.Response "评论目标不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/{id}/comments [get]
func (h *CommentHandler) ListArticle(c *gin.Context) {
	h.listTargetComments(c, targetTypeArticle, "id", "文章 ID")
}

// ListMoment 分页查询碎语评论。
// @Summary 分页查询碎语评论
// @Description 查询指定碎语的一级评论列表；登录态可返回当前用户点赞状态，回复通过独立接口懒加载。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Success 200 {object} response.Response{data=dto.CommentPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 404 {object} response.Response "评论目标不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id}/comments [get]
func (h *CommentHandler) ListMoment(c *gin.Context) {
	h.listTargetComments(c, targetTypeMoment, "id", "碎语 ID")
}

// ListArticleReplies 分页查询文章评论回复。
// @Summary 分页查询文章评论回复
// @Description 查询指定文章评论下的回复列表，支持分页懒加载；登录态可返回当前用户点赞状态。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "文章评论 ID"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Success 200 {object} response.Response{data=dto.CommentReplyPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 404 {object} response.Response "评论不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/comments/{id}/replies [get]
func (h *CommentHandler) ListArticleReplies(c *gin.Context) {
	h.listTargetReplies(c, targetTypeArticle, "id", "文章评论 ID")
}

// ListMomentReplies 分页查询碎语评论回复。
// @Summary 分页查询碎语评论回复
// @Description 查询指定碎语评论下的回复列表，支持分页懒加载；登录态可返回当前用户点赞状态。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "碎语评论 ID"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Success 200 {object} response.Response{data=dto.CommentReplyPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 404 {object} response.Response "评论不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/comments/{id}/replies [get]
func (h *CommentHandler) ListMomentReplies(c *gin.Context) {
	h.listTargetReplies(c, targetTypeMoment, "id", "碎语评论 ID")
}

// ListGuestbookReplies 分页查询留言回复。
// @Summary 分页查询留言回复
// @Description 查询指定留言下的回复列表，支持分页懒加载；登录态可返回当前用户点赞状态。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "留言 ID"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Success 200 {object} response.Response{data=dto.CommentReplyPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 404 {object} response.Response "评论不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /guestbook/comments/{id}/replies [get]
func (h *CommentHandler) ListGuestbookReplies(c *gin.Context) {
	h.listTargetReplies(c, targetTypeGuestbook, "id", "留言 ID")
}

func (h *CommentHandler) listTargetComments(c *gin.Context, targetType string, paramName string, fieldName string) {
	targetID, ok := reqbind.PathUint(c, paramName, fieldName)
	if !ok {
		return
	}

	var req dto.CommentListReq
	if !reqbind.Query(c, &req) {
		return
	}

	resp, err := h.svc.List(targetType, targetID, req, optionalCommentUser(c))
	writeCommentResponse(c, resp, err)
}

func (h *CommentHandler) listTargetReplies(c *gin.Context, targetType string, paramName string, fieldName string) {
	commentID, ok := reqbind.PathUint(c, paramName, fieldName)
	if !ok {
		return
	}

	var req dto.CommentReplyListReq
	if !reqbind.Query(c, &req) {
		return
	}

	resp, err := h.svc.ListReplies(targetType, commentID, req, optionalCommentUser(c))
	writeCommentResponse(c, resp, err)
}
