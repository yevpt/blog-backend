package moment

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// List 分页查询公开碎语。
// @Summary 分页查询公开碎语
// @Description 查询公开碎语列表，支持按作者或角色过滤；登录态可返回当前用户点赞状态。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param user_id query int false "作者用户 ID"
// @Param role_id query int false "作者角色 ID"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Success 200 {object} response.Response{data=dto.MomentPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments [get]
func (h *MomentHandler) List(c *gin.Context) {
	var req dto.MomentListReq
	if !reqbind.Query(c, &req) {
		return
	}

	resp, err := h.svc.List(req, optionalUser(c))
	writeMomentResponse(c, resp, err)
}

// GetDetail 查询碎语详情。
// @Summary 查询碎语详情
// @Description 查询公开碎语详情，包含作者、图片、点赞数和评论数。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Success 200 {object} response.Response{data=dto.MomentItemResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 404 {object} response.Response "碎语不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id} [get]
func (h *MomentHandler) GetDetail(c *gin.Context) {
	id, ok := bindMomentID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.GetDetail(id, optionalUser(c))
	writeMomentResponse(c, resp, err)
}

// Read 增加碎语阅读数。
// @Summary 增加碎语阅读数
// @Description 使用数据库原子更新将碎语阅读数增加 1。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Success 200 {object} response.Response{data=dto.MomentReadResp} "统一响应；code=0 表示更新成功，code=400 表示参数错误"
// @Failure 404 {object} response.Response "碎语不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id}/read [post]
func (h *MomentHandler) Read(c *gin.Context) {
	id, ok := bindMomentID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.Read(id)
	writeMomentResponse(c, resp, err)
}

// IsLiked 查询当前用户是否已点赞碎语。
// @Summary 查询碎语点赞状态
// @Description 查询当前登录用户是否已点赞指定碎语。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Success 200 {object} response.Response{data=dto.MomentLikeResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "碎语不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id}/like [get]
func (h *MomentHandler) IsLiked(c *gin.Context) {
	userID, _, ok := requiredUser(c)
	if !ok {
		return
	}
	id, ok := bindMomentID(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.IsLiked(id, userID)
	writeMomentResponse(c, resp, err)
}
