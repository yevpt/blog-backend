package comment

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// CreateArticle 新增文章一级评论。
// @Summary 新增文章一级评论
// @Description 登录用户为指定文章新增一级评论；目标关闭评论时会拒绝提交。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "文章 ID"
// @Param request body dto.CommentCreateReq true "评论新增请求"
// @Success 200 {object} response.Response{data=dto.CommentItemResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误或评论关闭"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "评论目标不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/{id}/comments [post]
func (h *CommentHandler) CreateArticle(c *gin.Context) {
	h.createTargetComment(c, targetTypeArticle, "id", "文章 ID")
}

// CreateMoment 新增碎语一级评论。
// @Summary 新增碎语一级评论
// @Description 登录用户为指定碎语新增一级评论；目标关闭评论时会拒绝提交。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Param request body dto.CommentCreateReq true "评论新增请求"
// @Success 200 {object} response.Response{data=dto.CommentItemResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误或评论关闭"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "评论目标不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id}/comments [post]
func (h *CommentHandler) CreateMoment(c *gin.Context) {
	h.createTargetComment(c, targetTypeMoment, "id", "碎语 ID")
}

// ReplyArticle 新增文章评论回复。
// @Summary 新增文章评论回复
// @Description 登录用户回复文章评论或其回复；parent_reply_id 为 0 时直接回复一级评论。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "文章评论 ID"
// @Param request body dto.CommentReplyCreateReq true "回复新增请求"
// @Success 200 {object} response.Response{data=dto.CommentReplyResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "评论或回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/comments/{id}/replies [post]
func (h *CommentHandler) ReplyArticle(c *gin.Context) {
	h.replyTargetComment(c, targetTypeArticle, "id", "文章评论 ID")
}

// ReplyMoment 新增碎语评论回复。
// @Summary 新增碎语评论回复
// @Description 登录用户回复碎语评论或其回复；parent_reply_id 为 0 时直接回复一级评论。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "碎语评论 ID"
// @Param request body dto.CommentReplyCreateReq true "回复新增请求"
// @Success 200 {object} response.Response{data=dto.CommentReplyResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "评论或回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/comments/{id}/replies [post]
func (h *CommentHandler) ReplyMoment(c *gin.Context) {
	h.replyTargetComment(c, targetTypeMoment, "id", "碎语评论 ID")
}

// ReplyGuestbook 新增留言回复。
// @Summary 新增留言回复
// @Description 登录用户回复留言或其回复；parent_reply_id 为 0 时直接回复一级留言。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "留言 ID"
// @Param request body dto.CommentReplyCreateReq true "回复新增请求"
// @Success 200 {object} response.Response{data=dto.CommentReplyResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "评论或回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /guestbook/comments/{id}/replies [post]
func (h *CommentHandler) ReplyGuestbook(c *gin.Context) {
	h.replyTargetComment(c, targetTypeGuestbook, "id", "留言 ID")
}

// ToggleArticleLike 切换文章评论点赞状态。
// @Summary 切换文章评论点赞
// @Description 当前用户未点赞时点赞，已点赞时取消点赞。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "文章评论 ID"
// @Success 200 {object} response.Response{data=dto.CommentLikeResp} "统一响应；code=0 表示切换成功，返回最新点赞状态和点赞数，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "评论不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/comments/{id}/like [post]
func (h *CommentHandler) ToggleArticleLike(c *gin.Context) {
	h.toggleTargetCommentLike(c, targetTypeArticle, "id", "文章评论 ID")
}

// ToggleMomentLike 切换碎语评论点赞状态。
// @Summary 切换碎语评论点赞
// @Description 当前用户未点赞时点赞，已点赞时取消点赞。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "碎语评论 ID"
// @Success 200 {object} response.Response{data=dto.CommentLikeResp} "统一响应；code=0 表示切换成功，返回最新点赞状态和点赞数，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "评论不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/comments/{id}/like [post]
func (h *CommentHandler) ToggleMomentLike(c *gin.Context) {
	h.toggleTargetCommentLike(c, targetTypeMoment, "id", "碎语评论 ID")
}

// ToggleArticleReplyLike 切换文章评论回复点赞状态。
// @Summary 切换文章评论回复点赞
// @Description 当前用户未点赞时点赞，已点赞时取消点赞。
// @Tags 评论
// @Accept json
// @Produce json
// @Param replyId path int true "文章评论回复 ID"
// @Success 200 {object} response.Response{data=dto.CommentLikeResp} "统一响应；code=0 表示切换成功，返回最新点赞状态和点赞数，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/comments/{id}/replies/{replyId}/like [post]
func (h *CommentHandler) ToggleArticleReplyLike(c *gin.Context) {
	h.toggleTargetReplyLike(c, targetTypeArticle, "replyId", "文章评论回复 ID")
}

// ToggleMomentReplyLike 切换碎语评论回复点赞状态。
// @Summary 切换碎语评论回复点赞
// @Description 当前用户未点赞时点赞，已点赞时取消点赞。
// @Tags 评论
// @Accept json
// @Produce json
// @Param replyId path int true "碎语评论回复 ID"
// @Success 200 {object} response.Response{data=dto.CommentLikeResp} "统一响应；code=0 表示切换成功，返回最新点赞状态和点赞数，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/comments/{id}/replies/{replyId}/like [post]
func (h *CommentHandler) ToggleMomentReplyLike(c *gin.Context) {
	h.toggleTargetReplyLike(c, targetTypeMoment, "replyId", "碎语评论回复 ID")
}

// ToggleGuestbookReplyLike 切换留言回复点赞状态。
// @Summary 切换留言回复点赞
// @Description 当前用户未点赞时点赞，已点赞时取消点赞。
// @Tags 评论
// @Accept json
// @Produce json
// @Param replyId path int true "留言回复 ID"
// @Success 200 {object} response.Response{data=dto.CommentLikeResp} "统一响应；code=0 表示切换成功，返回最新点赞状态和点赞数，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /guestbook/comments/{id}/replies/{replyId}/like [post]
func (h *CommentHandler) ToggleGuestbookReplyLike(c *gin.Context) {
	h.toggleTargetReplyLike(c, targetTypeGuestbook, "replyId", "留言回复 ID")
}

// DeleteArticle 删除文章评论。
// @Summary 删除文章评论
// @Description 评论作者可删除自己的文章评论；管理员可删除任意文章评论，同时删除其下回复。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "文章评论 ID"
// @Success 200 {object} response.Response{data=dto.CommentDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权删除评论"
// @Failure 404 {object} response.Response "评论不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/comments/{id} [delete]
func (h *CommentHandler) DeleteArticle(c *gin.Context) {
	h.deleteTargetComment(c, targetTypeArticle, "id", "文章评论 ID")
}

// DeleteMoment 删除碎语评论。
// @Summary 删除碎语评论
// @Description 评论作者可删除自己的碎语评论；管理员可删除任意碎语评论，同时删除其下回复。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "碎语评论 ID"
// @Success 200 {object} response.Response{data=dto.CommentDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权删除评论"
// @Failure 404 {object} response.Response "评论不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/comments/{id} [delete]
func (h *CommentHandler) DeleteMoment(c *gin.Context) {
	h.deleteTargetComment(c, targetTypeMoment, "id", "碎语评论 ID")
}

// DeleteGuestbook 删除留言。
// @Summary 删除留言
// @Description 留言作者可删除自己的留言；管理员可删除任意留言，同时删除其下回复。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "留言 ID"
// @Success 200 {object} response.Response{data=dto.CommentDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权删除评论"
// @Failure 404 {object} response.Response "评论不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /guestbook/comments/{id} [delete]
func (h *CommentHandler) DeleteGuestbook(c *gin.Context) {
	h.deleteTargetComment(c, targetTypeGuestbook, "id", "留言 ID")
}

// DeleteArticleReply 删除文章评论回复。
// @Summary 删除文章评论回复
// @Description 回复作者可删除自己的文章评论回复；管理员可删除任意回复。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "文章评论回复 ID"
// @Success 200 {object} response.Response{data=dto.CommentDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权删除回复"
// @Failure 404 {object} response.Response "回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/comment-replies/{id} [delete]
func (h *CommentHandler) DeleteArticleReply(c *gin.Context) {
	h.deleteTargetReply(c, targetTypeArticle, "id", "文章评论回复 ID")
}

// DeleteMomentReply 删除碎语评论回复。
// @Summary 删除碎语评论回复
// @Description 回复作者可删除自己的碎语评论回复；管理员可删除任意回复。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "碎语评论回复 ID"
// @Success 200 {object} response.Response{data=dto.CommentDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权删除回复"
// @Failure 404 {object} response.Response "回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/comment-replies/{id} [delete]
func (h *CommentHandler) DeleteMomentReply(c *gin.Context) {
	h.deleteTargetReply(c, targetTypeMoment, "id", "碎语评论回复 ID")
}

// DeleteGuestbookReply 删除留言回复。
// @Summary 删除留言回复
// @Description 回复作者可删除自己的留言回复；管理员可删除任意回复。
// @Tags 评论
// @Accept json
// @Produce json
// @Param id path int true "留言回复 ID"
// @Success 200 {object} response.Response{data=dto.CommentDeleteResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "无权删除回复"
// @Failure 404 {object} response.Response "回复不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /guestbook/comment-replies/{id} [delete]
func (h *CommentHandler) DeleteGuestbookReply(c *gin.Context) {
	h.deleteTargetReply(c, targetTypeGuestbook, "id", "留言回复 ID")
}

func (h *CommentHandler) createTargetComment(c *gin.Context, targetType string, paramName string, fieldName string) {
	userID, _, ok := requiredCommentClaims(c)
	if !ok {
		return
	}
	targetID, ok := reqbind.PathUint(c, paramName, fieldName)
	if !ok {
		return
	}

	var req dto.CommentCreateReq
	if !reqbind.JSON(c, &req) {
		return
	}

	resp, err := h.svc.Create(targetType, targetID, req, userID)
	writeCommentResponse(c, resp, err)
}

func (h *CommentHandler) replyTargetComment(c *gin.Context, targetType string, paramName string, fieldName string) {
	userID, _, ok := requiredCommentClaims(c)
	if !ok {
		return
	}
	commentID, ok := reqbind.PathUint(c, paramName, fieldName)
	if !ok {
		return
	}

	var req dto.CommentReplyCreateReq
	if !reqbind.JSON(c, &req) {
		return
	}

	resp, err := h.svc.Reply(targetType, commentID, req, userID)
	writeCommentResponse(c, resp, err)
}

func (h *CommentHandler) toggleTargetCommentLike(c *gin.Context, targetType string, paramName string, fieldName string) {
	userID, _, ok := requiredCommentClaims(c)
	if !ok {
		return
	}
	commentID, ok := reqbind.PathUint(c, paramName, fieldName)
	if !ok {
		return
	}

	resp, err := h.svc.ToggleLike(targetType, commentID, userID)
	writeCommentResponse(c, resp, err)
}

func (h *CommentHandler) toggleTargetReplyLike(c *gin.Context, targetType string, paramName string, fieldName string) {
	userID, _, ok := requiredCommentClaims(c)
	if !ok {
		return
	}
	replyID, ok := reqbind.PathUint(c, paramName, fieldName)
	if !ok {
		return
	}

	resp, err := h.svc.ToggleReplyLike(targetType, replyID, userID)
	writeCommentResponse(c, resp, err)
}

func (h *CommentHandler) deleteTargetComment(c *gin.Context, targetType string, paramName string, fieldName string) {
	userID, roleNames, ok := requiredCommentClaims(c)
	if !ok {
		return
	}
	commentID, ok := reqbind.PathUint(c, paramName, fieldName)
	if !ok {
		return
	}

	resp, err := h.svc.DeleteComment(targetType, commentID, userID, roleNames)
	writeCommentResponse(c, resp, err)
}

func (h *CommentHandler) deleteTargetReply(c *gin.Context, targetType string, paramName string, fieldName string) {
	userID, roleNames, ok := requiredCommentClaims(c)
	if !ok {
		return
	}
	replyID, ok := reqbind.PathUint(c, paramName, fieldName)
	if !ok {
		return
	}

	resp, err := h.svc.DeleteReply(targetType, replyID, userID, roleNames)
	writeCommentResponse(c, resp, err)
}
