package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
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

// Create 新增分类。
// @Summary 新增分类
// @Description 管理员新增分类；父分类字段仅预留，当前不处理父子层级。
// @Tags 分类
// @Accept json
// @Produce json
// @Param request body dto.CategoryCreateReq true "分类新增请求"
// @Success 200 {object} response.Response{data=dto.CategoryItemResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/categories [post]
func (h *CategoryHandler) Create(c *gin.Context) {
	var req dto.CategoryCreateReq
	if !reqbind.JSON(c, &req) {
		return
	}
	resp, err := h.svc.Create(req)
	writeCategoryResponse(c, resp, err)
}

// Update 修改分类。
// @Summary 修改分类
// @Description 管理员修改分类名称、排序、图标、描述、封面等属性。
// @Tags 分类
// @Accept json
// @Produce json
// @Param id path int true "分类 ID"
// @Param request body dto.CategoryUpdateReq true "分类修改请求"
// @Success 200 {object} response.Response{data=dto.CategoryItemResp} "统一响应；code=0 表示修改成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "分类不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/categories/{id} [put]
func (h *CategoryHandler) Update(c *gin.Context) {
	id, ok := bindCategoryID(c)
	if !ok {
		return
	}
	var req dto.CategoryUpdateReq
	if !reqbind.JSON(c, &req) {
		return
	}
	resp, err := h.svc.Update(id, req)
	writeCategoryResponse(c, resp, err)
}

// Delete 删除分类。
// @Summary 删除分类
// @Description 管理员删除分类，并清空该分类下文章关联；文章本身不会被删除。
// @Tags 分类
// @Produce json
// @Param id path int true "分类 ID"
// @Success 200 {object} response.Response{data=dto.CategoryItemResp} "统一响应；code=0 表示删除成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "分类不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/categories/{id} [delete]
func (h *CategoryHandler) Delete(c *gin.Context) {
	id, ok := bindCategoryID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Delete(id)
	writeCategoryResponse(c, resp, err)
}

// AddArticles 添加文章到分类。
// @Summary 添加文章到分类
// @Description 管理员把单篇或多篇文章加入分类；文章原有分类关系会迁移到当前分类。
// @Tags 分类
// @Accept json
// @Produce json
// @Param id path int true "分类 ID"
// @Param request body dto.CategoryArticlesReq true "文章 ID 列表"
// @Success 200 {object} response.Response{data=dto.CategoryArticlesResp} "统一响应；code=0 表示添加成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "分类或文章不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/categories/{id}/articles [post]
func (h *CategoryHandler) AddArticles(c *gin.Context) {
	id, ok := bindCategoryID(c)
	if !ok {
		return
	}
	var req dto.CategoryArticlesReq
	if !reqbind.JSON(c, &req) {
		return
	}
	resp, err := h.svc.AddArticles(id, req)
	writeCategoryResponse(c, resp, err)
}

// RemoveArticles 删除分类下文章。
// @Summary 删除分类下文章
// @Description 管理员批量移除分类下文章关联；文章本身不会被删除。
// @Tags 分类
// @Accept json
// @Produce json
// @Param id path int true "分类 ID"
// @Param request body dto.CategoryArticlesReq true "文章 ID 列表"
// @Success 200 {object} response.Response{data=dto.CategoryArticlesResp} "统一响应；code=0 表示移除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "分类不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/categories/{id}/articles [delete]
func (h *CategoryHandler) RemoveArticles(c *gin.Context) {
	id, ok := bindCategoryID(c)
	if !ok {
		return
	}
	var req dto.CategoryArticlesReq
	if !reqbind.JSON(c, &req) {
		return
	}
	resp, err := h.svc.RemoveArticles(id, req)
	writeCategoryResponse(c, resp, err)
}

func bindCategoryID(c *gin.Context) (uint, bool) {
	return reqbind.PathUint(c, "id", "分类 ID")
}

func writeCategoryResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, service.ErrCategoryNotFound) || errors.Is(err, service.ErrCategoryArticleMissing) {
		response.NotFound(c)
		return
	}
	if isCategoryBadRequest(err) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}

func isCategoryBadRequest(err error) bool {
	return errors.Is(err, service.ErrCategoryNameRequired) ||
		errors.Is(err, service.ErrCategorySeqRequired) ||
		errors.Is(err, service.ErrCategoryIconRequired) ||
		errors.Is(err, service.ErrCategoryDescRequired) ||
		errors.Is(err, service.ErrCategoryCoverRequired) ||
		errors.Is(err, service.ErrCategoryArticleRequired)
}
