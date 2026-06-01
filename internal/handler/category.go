package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/service"
	"github.com/vpt/blog-backend/pkg/response"
)

// CategoryHandler 分类模块 HTTP 入口。
type CategoryHandler struct {
	svc service.CategoryService
}

func NewCategoryHandler(svc service.CategoryService) *CategoryHandler {
	return &CategoryHandler{svc: svc}
}

// ListTabs 查询分类 Tab 列表。
// @Summary 查询分类 Tab 列表
// @Description 返回所有分类及其公开文章数量，按 seq ASC、文章数量 DESC 排序。
// @Tags 分类
// @Produce json
// @Success 200 {object} response.Response{data=dto.CategoryTabsResp} "统一响应；code=0 表示查询成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /categories [get]
func (h *CategoryHandler) ListTabs(c *gin.Context) {
	resp, err := h.svc.ListTabs()
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}
