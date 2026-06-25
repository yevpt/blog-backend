package analytics

import (
	"strconv"

	"github.com/gin-gonic/gin"
	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"github.com/vpt/blog-backend/pkg/response"
)

const (
	publicDefaultPopular = 10
	publicMaxPopular     = 20
)

// PublicHandler 前台公开统计入口（无需登录，仅聚合数字）。
type PublicHandler struct {
	svc svc.PublicService
}

// NewPublicHandler 注入前台公开统计服务。
func NewPublicHandler(s svc.PublicService) *PublicHandler { return &PublicHandler{svc: s} }

// Summary 前台公开总览。
// @Summary  前台站点统计总览（公开）
// @Tags     analytics
// @Produce  json
// @Success  200 {object} response.Response{data=dto.PublicSummary} "统一响应；code=0 成功"
// @Failure  429 {object} response.Response "请求过于频繁"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /analytics/public/summary [get]
func (h *PublicHandler) Summary(c *gin.Context) {
	var data dto.PublicSummary
	data, err := h.svc.Summary(c.Request.Context())
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}

// Popular 前台热门页面榜（公开，排除 /admin/*）。
// @Summary  前台热门页面（公开）
// @Tags     analytics
// @Produce  json
// @Param    limit query int false "返回条数，默认 10，上限 20"
// @Success  200 {object} response.Response{data=[]dto.PublicPageStat} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  429 {object} response.Response "请求过于频繁"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /analytics/public/popular [get]
func (h *PublicHandler) Popular(c *gin.Context) {
	limit := publicDefaultPopular
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.Fail(c, response.CodeBadRequest, "limit 必须是大于 0 的整数")
			return
		}
		limit = n
	}
	if limit > publicMaxPopular {
		limit = publicMaxPopular
	}
	var data []dto.PublicPageStat
	data, err := h.svc.Popular(c.Request.Context(), limit)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}
