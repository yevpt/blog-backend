package moment

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/handler/reqbind"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/pkg/response"
)

const maxMomentExcludeIDs = 50

// Feed 分页查询碎语独立页广场流。
// @Summary 分页查询碎语广场流
// @Description 碎语独立页专用列表，支持全部/博主/朋友们范围与最新/最热排序；最新按置顶优先、发布时间倒序，最热按评论×10+点赞×3+阅读综合分倒序。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param scope query string true "范围：all、owner、friends"
// @Param sort query string true "排序：latest、hot"
// @Param page query int false "页码，从 1 开始"
// @Param page_size query int false "每页数量，默认 10，最大 50"
// @Success 200 {object} response.Response{data=dto.MomentPageResp} "统一响应；code=0 表示查询成功，code=400 表示参数错误"
// @Failure 401 {object} response.Response "Authorization header 存在但 token 非法或已过期"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/feed [get]
func (h *MomentHandler) Feed(c *gin.Context) {
	var req dto.MomentFeedReq
	if !reqbind.Query(c, &req) {
		return
	}

	resp, err := h.svc.FeedList(req, optionalUser(c))
	writeMomentResponse(c, resp, err)
}

// List 分页查询公开碎语。
// @Summary 分页查询公开碎语
// @Description 查询公开碎语列表，支持按作者或角色过滤；登录态可返回当前用户点赞状态。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param user_id query int false "作者用户 ID"
// @Param role_id query int false "作者角色 ID"
// @Param random query bool false "是否随机抽样；true 时忽略 page，从公开碎语池随机返回 page_size 条"
// @Param exclude_ids query string false "随机抽样时排除的碎语 ID，逗号分隔，如 1,2,3"
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
	excludeIDs, ok := parseMomentExcludeIDs(c.Query("exclude_ids"))
	if !ok {
		response.Fail(c, response.CodeBadRequest, "exclude_ids 必须是最多 50 个大于 0 的整数")
		return
	}
	req.ExcludeIDs = excludeIDs

	resp, err := h.svc.List(req, optionalUser(c))
	writeMomentResponse(c, resp, err)
}

func parseMomentExcludeIDs(raw string) ([]uint, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}

	parts := strings.Split(raw, ",")
	if len(parts) > maxMomentExcludeIDs {
		return nil, false
	}

	ids := make([]uint, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil || id == 0 {
			return nil, false
		}
		ids = append(ids, uint(id))
	}
	return ids, true
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

// View 增加碎语阅读数（UV 去重）。
// @Summary 增加碎语阅读数
// @Description 同一访客同一碎语 24 小时内只增加一次阅读数。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Success 200 {object} response.Response{data=dto.MomentViewResp} "统一响应；code=0 表示更新成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 404 {object} response.Response "碎语不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id}/view [post]
func (h *MomentHandler) View(c *gin.Context) {
	id, ok := bindMomentID(c, "id")
	if !ok {
		return
	}
	visitorID := middleware.GetVisitorID(c)
	resp, err := h.svc.View(id, visitorID)
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
