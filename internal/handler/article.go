package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/service"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

// ArticleHandler 文章模块 HTTP 入口，只负责参数绑定、调用 service 和选择响应。
type ArticleHandler struct {
	svc service.ArticleService
}

func NewArticleHandler(svc service.ArticleService) *ArticleHandler {
	return &ArticleHandler{svc: svc}
}

// ListIDs 查询公开文章 ID 列表。
// @Summary 查询公开文章 ID 列表
// @Description 返回所有公开文章 ID，按发布时间倒序排列。
// @Tags 文章
// @Accept json
// @Produce json
// @Success 200 {object} response.Response{data=dto.ArticleIDsResp} "统一响应；code=0 表示查询成功"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/ids [get]
func (h *ArticleHandler) ListIDs(c *gin.Context) {
	resp, err := h.svc.ListIDs()
	writeArticleResponse(c, resp, err)
}

// ListPublic 分页查询公开文章。
// @Summary 分页查询公开文章
// @Description 按页码分页查询公开文章，支持推荐、分类和标签过滤；加密文章只在详情接口隐藏正文。
// @Tags 文章
// @Accept json
// @Produce json
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Param recommend query bool false "是否只查询推荐文章"
// @Param category_id query int false "分类 ID"
// @Param tag_id query int false "标签 ID"
// @Success 200 {object} response.Response{data=dto.ArticlePageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles [get]
func (h *ArticleHandler) ListPublic(c *gin.Context) {
	var req dto.ArticleListReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	resp, err := h.svc.ListPublic(req)
	writeArticleResponse(c, resp, err)
}

// GetPublicDetail 查询文章详情。
// @Summary 查询文章详情
// @Description 查询公开或加密文章详情；加密文章在公开接口中不返回正文。
// @Tags 文章
// @Accept json
// @Produce json
// @Param id path int true "文章 ID"
// @Success 200 {object} response.Response{data=dto.ArticleDetailResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 404 {object} response.Response "文章不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/{id} [get]
func (h *ArticleHandler) GetPublicDetail(c *gin.Context) {
	id, ok := bindUintPath(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.GetPublicDetail(id, optionalUserID(c))
	writeArticleResponse(c, resp, err)
}

// Read 增加文章阅读数。
// @Summary 增加文章阅读数
// @Description 使用数据库原子更新将文章阅读数增加 1。
// @Tags 文章
// @Accept json
// @Produce json
// @Param id path int true "文章 ID"
// @Success 200 {object} response.Response{data=dto.ArticleReadResp} "统一响应；code=0 表示更新成功，code=400 表示参数错误"
// @Failure 404 {object} response.Response "文章不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/{id}/read [post]
func (h *ArticleHandler) Read(c *gin.Context) {
	id, ok := bindUintPath(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.Read(id)
	writeArticleResponse(c, resp, err)
}

// IsLiked 查询当前用户是否已点赞。
// @Summary 查询文章点赞状态
// @Description 查询当前登录用户是否已点赞指定文章。
// @Tags 文章
// @Accept json
// @Produce json
// @Param id path int true "文章 ID"
// @Success 200 {object} response.Response{data=dto.ArticleLikeResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/{id}/like [get]
func (h *ArticleHandler) IsLiked(c *gin.Context) {
	id, ok := bindUintPath(c, "id")
	if !ok {
		return
	}
	userID, ok := requiredUserID(c)
	if !ok {
		return
	}

	resp, err := h.svc.IsLiked(id, userID)
	writeArticleResponse(c, resp, err)
}

// ToggleLike 切换当前用户点赞状态。
// @Summary 切换文章点赞
// @Description 当前用户未点赞时点赞，已点赞时取消点赞。
// @Tags 文章
// @Accept json
// @Produce json
// @Param id path int true "文章 ID"
// @Success 200 {object} response.Response{data=dto.ArticleDetailResp} "统一响应；code=0 表示切换成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 404 {object} response.Response "文章不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/{id}/like [post]
func (h *ArticleHandler) ToggleLike(c *gin.Context) {
	id, ok := bindUintPath(c, "id")
	if !ok {
		return
	}
	userID, ok := requiredUserID(c)
	if !ok {
		return
	}

	resp, err := h.svc.ToggleLike(id, userID)
	writeArticleResponse(c, resp, err)
}

// Save 新增或更新文章。
// @Summary 新增或更新文章
// @Description 管理员新增或更新文章，并同步分类、标签、音乐和推荐关系。
// @Tags 文章管理
// @Accept json
// @Produce json
// @Param request body dto.ArticleSaveReq true "文章保存请求"
// @Success 200 {object} response.Response{data=dto.ArticleDetailResp} "统一响应；code=0 表示保存成功，code=400 表示参数错误或业务错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "文章不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/articles [post]
func (h *ArticleHandler) Save(c *gin.Context) {
	userID, ok := requiredUserID(c)
	if !ok {
		return
	}

	var req dto.ArticleSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeBadRequest, "参数错误: "+err.Error())
		return
	}

	resp, err := h.svc.Save(req, userID)
	writeArticleResponse(c, resp, err)
}

// Delete 软删除文章。
// @Summary 删除文章
// @Description 管理员软删除文章。
// @Tags 文章管理
// @Accept json
// @Produce json
// @Param id path int true "文章 ID"
// @Success 200 {object} response.Response{data=dto.ArticleDetailResp} "统一响应；code=0 表示删除成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 403 {object} response.Response "权限不足"
// @Failure 404 {object} response.Response "文章不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /admin/articles/{id} [delete]
func (h *ArticleHandler) Delete(c *gin.Context) {
	id, ok := bindUintPath(c, "id")
	if !ok {
		return
	}

	resp, err := h.svc.Delete(id)
	writeArticleResponse(c, resp, err)
}

func bindUintPath(c *gin.Context, name string) (uint, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, response.CodeBadRequest, "参数错误")
		return 0, false
	}
	return uint(id), true
}

func requiredUserID(c *gin.Context) (uint, bool) {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		response.Unauthorized(c)
		return 0, false
	}
	return uint(claims.UserId), true
}

func optionalUserID(c *gin.Context) *uint {
	claims := jwtpkg.GetClaims(c)
	if claims == nil || claims.UserId <= 0 {
		return nil
	}
	id := uint(claims.UserId)
	return &id
}

func writeArticleResponse(c *gin.Context, data interface{}, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	if errors.Is(err, service.ErrArticleNotFound) {
		response.NotFound(c)
		return
	}
	if errors.Is(err, service.ErrArticlePasswordRequired) || errors.Is(err, service.ErrArticleCategoryRequired) {
		response.Fail(c, response.CodeBadRequest, err.Error())
		return
	}
	response.ServerError(c)
}
