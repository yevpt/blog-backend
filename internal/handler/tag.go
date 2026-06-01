package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/service"
	"github.com/vpt/blog-backend/pkg/response"
)

// TagHandler 标签模块 HTTP 入口。
type TagHandler struct {
	svc service.TagService
}

func NewTagHandler(svc service.TagService) *TagHandler {
	return &TagHandler{svc: svc}
}

// List 查询标签列表。
// @Summary 查询标签列表
// @Description 返回所有标签及其公开文章数量，按 seq ASC、文章数量 DESC 排序。
// @Tags 标签
// @Produce json
// @Success 200 {object} response.Response{data=dto.TagListResp} "统一响应；code=0 表示查询成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /tags [get]
func (h *TagHandler) List(c *gin.Context) {
	resp, err := h.svc.List()
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, resp)
}

// Get 查询标签详情。
// @Summary 查询标签详情
// @Description 返回单个标签元数据及其公开文章数量。
// @Tags 标签
// @Produce json
// @Param id path int true "标签 ID"
// @Success 200 {object} response.Response{data=dto.TagItemResp} "统一响应；code=0 表示查询成功"
// @Failure 404 {object} response.Response "标签不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /tags/{id} [get]
func (h *TagHandler) Get(c *gin.Context) {
	id, ok := bindTagID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Get(id)
	writeTagResponse(c, resp, err)
}

// ListArticles 查询标签下文章。
// @Summary 查询标签下文章
// @Description 分页返回指定标签下的公开文章，响应结构与文章分页接口一致。
// @Tags 标签
// @Produce json
// @Param id path int true "标签 ID"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，最大 50"
// @Success 200 {object} response.Response{data=dto.ArticlePageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 404 {object} response.Response "标签不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /tags/{id}/articles [get]
func (h *TagHandler) ListArticles(c *gin.Context) {
	id, ok := bindTagID(c)
	if !ok {
		return
	}
	var req dto.ArticleListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	resp, err := h.svc.ListArticles(id, req)
	writeTagResponse(c, resp, err)
}

// Create 新增标签。
// @Summary 新增标签
// @Description 管理员新增标签。
// @Tags 标签
// @Accept json
// @Produce json
// @Param request body dto.TagCreateReq true "标签新增请求"
// @Success 200 {object} response.Response{data=dto.TagItemResp} "统一响应；code=0 表示新增成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/tags [post]
func (h *TagHandler) Create(c *gin.Context) {
	var req dto.TagCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	resp, err := h.svc.Create(req)
	writeTagResponse(c, resp, err)
}

// Update 修改标签。
// @Summary 修改标签
// @Description 管理员修改标签名称、排序、图标、描述、封面等属性。
// @Tags 标签
// @Accept json
// @Produce json
// @Param id path int true "标签 ID"
// @Param request body dto.TagUpdateReq true "标签修改请求"
// @Success 200 {object} response.Response{data=dto.TagItemResp} "统一响应；code=0 表示修改成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "标签不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/tags/{id} [put]
func (h *TagHandler) Update(c *gin.Context) {
	id, ok := bindTagID(c)
	if !ok {
		return
	}
	var req dto.TagUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	resp, err := h.svc.Update(id, req)
	writeTagResponse(c, resp, err)
}

// Delete 删除标签。
// @Summary 删除标签
// @Description 管理员删除标签，并清空该标签下文章关联；文章本身不会被删除。
// @Tags 标签
// @Produce json
// @Param id path int true "标签 ID"
// @Success 200 {object} response.Response{data=dto.TagItemResp} "统一响应；code=0 表示删除成功"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "标签不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/tags/{id} [delete]
func (h *TagHandler) Delete(c *gin.Context) {
	id, ok := bindTagID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Delete(id)
	writeTagResponse(c, resp, err)
}

// AddArticles 给文章添加标签。
// @Summary 给文章添加标签
// @Description 管理员给单篇或多篇文章添加标签；已存在的关联会跳过。
// @Tags 标签
// @Accept json
// @Produce json
// @Param id path int true "标签 ID"
// @Param request body dto.TagArticlesReq true "文章 ID 列表"
// @Success 200 {object} response.Response{data=dto.TagArticlesResp} "统一响应；code=0 表示添加成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "标签或文章不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/tags/{id}/articles [post]
func (h *TagHandler) AddArticles(c *gin.Context) {
	id, ok := bindTagID(c)
	if !ok {
		return
	}
	var req dto.TagArticlesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	resp, err := h.svc.AddArticles(id, req)
	writeTagResponse(c, resp, err)
}

// RemoveArticles 删除文章标签。
// @Summary 删除文章标签
// @Description 管理员批量移除文章的某个标签；文章本身不会被删除。
// @Tags 标签
// @Accept json
// @Produce json
// @Param id path int true "标签 ID"
// @Param request body dto.TagArticlesReq true "文章 ID 列表"
// @Success 200 {object} response.Response{data=dto.TagArticlesResp} "统一响应；code=0 表示移除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "标签不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/tags/{id}/articles [delete]
func (h *TagHandler) RemoveArticles(c *gin.Context) {
	id, ok := bindTagID(c)
	if !ok {
		return
	}
	var req dto.TagArticlesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return
	}
	resp, err := h.svc.RemoveArticles(id, req)
	writeTagResponse(c, resp, err)
}

func bindTagID(c *gin.Context) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return 0, false
	}
	return uint(id64), true
}

func writeTagResponse(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, service.ErrTagNotFound) || errors.Is(err, service.ErrTagArticleMissing) {
		response.NotFound(c)
		return
	}
	if isTagBadRequest(err) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}

func isTagBadRequest(err error) bool {
	return errors.Is(err, service.ErrTagNameRequired) ||
		errors.Is(err, service.ErrTagSeqRequired) ||
		errors.Is(err, service.ErrTagArticleRequired)
}
