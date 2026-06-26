package moment

import (
	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// ListAdmin 后台分页查询碎语。
// @Summary 后台分页查询碎语
// @Description 管理员查询全站碎语列表，支持状态筛选与正文搜索。
// @Tags 后台碎语
// @Accept json
// @Produce json
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Param status query string false "状态筛选：all、public、hidden"
// @Param search query string false "按碎语正文搜索"
// @Success 200 {object} response.Response{data=dto.AdminMomentPageResp} "统一响应；code=0 表示查询成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 401 {object} response.Response "未登录或 token 无效"
// @Failure 403 {object} response.Response "非管理员"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/moments [get]
func (h *MomentHandler) ListAdmin(c *gin.Context) {
	var req dto.AdminMomentListReq
	if !reqbind.Query(c, &req) {
		return
	}

	resp, err := h.svc.ListAdmin(req)
	writeMomentResponse(c, resp, err)
}
