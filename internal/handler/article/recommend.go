package article

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
)

// ListRecommendedAdmin 查询推荐文章排序列表。
// @Summary 查询推荐文章排序列表
// @Description 返回未软删除的推荐文章，按 recommend_seq 升序排列，供管理端拖拽排序弹窗使用。
// @Tags 文章管理
// @Accept json
// @Produce json
// @Success 200 {object} response.Response{data=dto.AdminRecommendListResp} "统一响应；code=0 表示查询成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/articles/recommendations [get]
func (h *ArticleHandler) ListRecommendedAdmin(c *gin.Context) {
	resp, err := h.svc.ListRecommendedAdmin()
	writeArticleResponse(c, resp, err)
}

// ReorderRecommendedAdmin 批量更新推荐文章排序。
// @Summary 批量更新推荐文章排序
// @Description 按 article_ids 数组顺序重写 recommend_seq；须包含当前全部推荐文章且无重复。
// @Tags 文章管理
// @Accept json
// @Produce json
// @Param request body dto.AdminRecommendOrderReq true "推荐排序请求"
// @Success 200 {object} response.Response "统一响应；code=0 表示保存成功"
// @Failure 400 {object} response.Response "参数错误或推荐集合不一致"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/articles/recommendations/order [put]
func (h *ArticleHandler) ReorderRecommendedAdmin(c *gin.Context) {
	var req dto.AdminRecommendOrderReq
	if !reqbind.JSON(c, &req) {
		return
	}
	writeArticleResponse(c, nil, h.svc.ReorderRecommendedAdmin(req))
}
