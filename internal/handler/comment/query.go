package comment

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// List 分页查询评论。
// @Summary 分页查询评论
// @Description 按目标类型和目标 ID 查询一级评论，并附带当前页评论下的回复列表。
// @Tags 评论
// @Accept json
// @Produce json
// @Param target_type query string true "目标类型：article、moment、guestbook"
// @Param target_id query int true "目标 ID"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Success 200 {object} response.Response{data=dto.CommentPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 404 {object} response.Response "评论目标不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /comments [get]
func (h *CommentHandler) List(c *gin.Context) {
	var req dto.CommentListReq
	if !reqbind.Query(c, &req) {
		return
	}

	resp, err := h.svc.List(req)
	writeCommentResponse(c, resp, err)
}
