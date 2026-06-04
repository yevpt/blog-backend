package guestbook

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// List 分页查询留言板留言。
// @Summary 分页查询留言板留言
// @Description 查询指定用户留言板的留言；owner_user_id 可省略，默认查询博主 1 的留言板。
// @Tags 留言板
// @Accept json
// @Produce json
// @Param owner_user_id query int false "留言板主人用户 ID，默认 1"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Success 200 {object} response.Response{data=dto.GuestbookPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 404 {object} response.Response "留言板主人不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /guestbook [get]
func (h *GuestbookHandler) List(c *gin.Context) {
	var req dto.GuestbookListReq
	if !reqbind.Query(c, &req) {
		return
	}

	resp, err := h.svc.List(req, optionalUser(c))
	writeGuestbookResponse(c, resp, err)
}
