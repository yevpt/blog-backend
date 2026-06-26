// Package dashboard 暴露后台首页汇总接口。
package dashboard

import (
	"github.com/gin-gonic/gin"
	dto "github.com/vpt/blog-backend/internal/dto/dashboard"
	svc "github.com/vpt/blog-backend/internal/service/dashboard"
	"github.com/vpt/blog-backend/pkg/response"
)

// Handler 后台首页汇总处理器。
type Handler struct {
	svc svc.Service
}

// NewHandler 构造首页汇总处理器。
func NewHandler(s svc.Service) *Handler { return &Handler{svc: s} }

// Overview 后台首页汇总：内容总量、近 7 天新增互动、用户统计。
// @Summary  后台首页汇总
// @Description 概览页非流量块：内容总量、近 7 天新增评论/留言/动态、用户总数/今日新增/今日活跃。
// @Tags     dashboard
// @Produce  json
// @Success  200 {object} response.Response{data=dto.OverviewSummary} "统一响应；code=0 成功"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/overview/summary [get]
func (h *Handler) Overview(c *gin.Context) {
	var data dto.OverviewSummary
	data, err := h.svc.Overview(c.Request.Context())
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}
