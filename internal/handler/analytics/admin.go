package analytics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"github.com/vpt/blog-backend/pkg/response"
)

// 查询参数口径：日期格式、默认区间、限幅与白名单。
const (
	dateLayout      = "2006-01-02"
	defaultTrendDay = 7   // from/to 缺省时回看天数
	maxRangeDays    = 365 // from~to 跨度上限
	defaultPageSize = 20  // 热门页面默认条数
	maxPageSize     = 100 // 热门页面条数上限
)

// adminTZ 统一以 Asia/Shanghai 计算缺省日期，避免跨时区错位；加载失败回落 UTC。
var adminTZ = func() *time.Location {
	tz, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.UTC
	}
	return tz
}()

var (
	validMetrics    = map[string]struct{}{"pv": {}, "uv": {}, "sessions": {}}
	validSegments   = map[string]struct{}{"all": {}, "registered": {}, "anonymous": {}}
	validDimensions = map[string]struct{}{
		"referer_type": {}, "device": {}, "browser": {}, "os": {}, "country": {}, "user_type": {},
	}
)

// AdminHandler 后台统计查询入口，只做参数解析/校验与统一响应。
type AdminHandler struct {
	svc      svc.QueryService
	backfill svc.BackfillService
}

// NewAdminHandler 注入只读查询服务与回填服务。
func NewAdminHandler(s svc.QueryService, b svc.BackfillService) *AdminHandler {
	return &AdminHandler{svc: s, backfill: b}
}

// Overview 站点总览：今日实时 + 在线 + 历史累计。
// @Summary  站点统计总览
// @Tags     analytics
// @Produce  json
// @Success  200 {object} response.Response{data=dto.Overview} "统一响应；code=0 表示成功"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/analytics/overview [get]
func (h *AdminHandler) Overview(c *gin.Context) {
	var data dto.Overview
	data, err := h.svc.Overview(c.Request.Context())
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}

// Trend 趋势图：按 metric/segment 返回区间内逐日取值。
// @Summary  站点访问趋势
// @Tags     analytics
// @Produce  json
// @Param    from    query string false "起始日期 YYYY-MM-DD，默认近 7 天"
// @Param    to      query string false "结束日期 YYYY-MM-DD，默认今天"
// @Param    metric  query string false "指标：pv、uv、sessions，默认 pv"
// @Param    segment query string false "分档：all、registered、anonymous，默认 all"
// @Success  200 {object} response.Response{data=[]dto.TrendPoint} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/analytics/trend [get]
func (h *AdminHandler) Trend(c *gin.Context) {
	from, to, ok := parseRange(c)
	if !ok {
		return
	}

	metric := c.DefaultQuery("metric", "pv")
	if _, valid := validMetrics[metric]; !valid {
		response.Fail(c, response.CodeBadRequest, "metric 仅支持 pv、uv、sessions")
		return
	}
	segment := c.DefaultQuery("segment", "all")
	if _, valid := validSegments[segment]; !valid {
		response.Fail(c, response.CodeBadRequest, "segment 仅支持 all、registered、anonymous")
		return
	}

	var data []dto.TrendPoint
	data, err := h.svc.Trend(c.Request.Context(), from, to, metric, segment)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}

// Pages 热门页面排行：区间内按访问量取前 limit 条。
// @Summary  热门页面排行
// @Tags     analytics
// @Produce  json
// @Param    from  query string false "起始日期 YYYY-MM-DD，默认近 7 天"
// @Param    to    query string false "结束日期 YYYY-MM-DD，默认今天"
// @Param    limit query int    false "返回条数，默认 20，上限 100"
// @Success  200 {object} response.Response{data=[]dto.PageStat} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/analytics/pages [get]
func (h *AdminHandler) Pages(c *gin.Context) {
	from, to, ok := parseRange(c)
	if !ok {
		return
	}

	limit := defaultPageSize
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.Fail(c, response.CodeBadRequest, "limit 必须是大于 0 的整数")
			return
		}
		limit = n
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	var data []dto.PageStat
	data, err := h.svc.TopPages(c.Request.Context(), from, to, limit)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}

// Dimensions 维度分布：某维度在区间内逐日的 PV/UV。
// @Summary  站点维度分布
// @Tags     analytics
// @Produce  json
// @Param    dimension query string true  "维度：referer_type、device、browser、os、country、user_type"
// @Param    from      query string false "起始日期 YYYY-MM-DD，默认近 7 天"
// @Param    to        query string false "结束日期 YYYY-MM-DD，默认今天"
// @Success  200 {object} response.Response{data=[]dto.DimensionPoint} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/analytics/dimensions [get]
func (h *AdminHandler) Dimensions(c *gin.Context) {
	dimension := c.Query("dimension")
	if _, ok := validDimensions[dimension]; !ok {
		response.Fail(c, response.CodeBadRequest, "dimension 仅支持 referer_type、device、browser、os、country、user_type")
		return
	}
	from, to, ok := parseRange(c)
	if !ok {
		return
	}
	data, err := h.svc.Dimensions(c.Request.Context(), dimension, from, to)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}

// Paths 访问路径序列（管理员，聚合会话数）。
// @Summary  访问路径序列
// @Tags     analytics
// @Produce  json
// @Param    from  query string false "起始日期 YYYY-MM-DD，默认近 7 天"
// @Param    to    query string false "结束日期 YYYY-MM-DD，默认今天"
// @Param    limit query int false "返回条数，默认 20，上限 100"
// @Success  200 {object} response.Response{data=[]dto.PathSequence} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/analytics/paths [get]
func (h *AdminHandler) Paths(c *gin.Context) {
	from, to, ok := parseRange(c)
	if !ok {
		return
	}

	limit := defaultPageSize
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.Fail(c, response.CodeBadRequest, "limit 必须是大于 0 的整数")
			return
		}
		limit = n
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	data, err := h.svc.Paths(c.Request.Context(), from, to, limit)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}

// maxFunnelSteps 漏斗步骤数上限，避免超长 step 列表。
const maxFunnelSteps = 10

// Funnel 漏斗留存（管理员，按 step 顺序匹配路径）。
// @Summary  访问漏斗
// @Tags     analytics
// @Produce  json
// @Param    from query string false "起始日期 YYYY-MM-DD，默认近 7 天"
// @Param    to   query string false "结束日期 YYYY-MM-DD，默认今天"
// @Param    step query []string true "漏斗步骤，可重复传 step=/a&step=/b，最多 10 个"
// @Success  200 {object} response.Response{data=[]dto.FunnelStep} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/analytics/funnel [get]
func (h *AdminHandler) Funnel(c *gin.Context) {
	steps := c.QueryArray("step")
	if len(steps) == 0 {
		response.Fail(c, response.CodeBadRequest, "step 至少传 1 个")
		return
	}
	if len(steps) > maxFunnelSteps {
		response.Fail(c, response.CodeBadRequest, "step 最多 10 个")
		return
	}
	from, to, ok := parseRange(c)
	if !ok {
		return
	}

	data, err := h.svc.Funnel(c.Request.Context(), from, to, steps)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}

// Backfill 重算指定区间的日聚合（管理员，幂等）。
// @Summary  回填日聚合
// @Description 对指定闭区间逐日重算统计聚合，适用于统计规则调整后的历史补算。
// @Tags     analytics
// @Accept   json
// @Produce  json
// @Param    from query string true "起始日期 YYYY-MM-DD"
// @Param    to   query string true "结束日期 YYYY-MM-DD"
// @Success  200 {object} response.Response{data=dto.BackfillResult} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  429 {object} response.Response "请求过于频繁"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/analytics/backfill [post]
func (h *AdminHandler) Backfill(c *gin.Context) {
	from, to, ok := parseRequiredRange(c)
	if !ok {
		return
	}

	days, err := h.backfill.Backfill(c.Request.Context(), from, to)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, dto.BackfillResult{From: from, To: to, Days: days})
}

// parseRange 解析 from/to：缺省回看近 7 天，校验格式与跨度上限（含越界即写错误响应）。
func parseRange(c *gin.Context) (from, to string, ok bool) {
	now := time.Now().In(adminTZ)
	toRaw := c.DefaultQuery("to", now.Format(dateLayout))
	fromRaw := c.DefaultQuery("from", now.AddDate(0, 0, -(defaultTrendDay-1)).Format(dateLayout))

	fromDate, err := time.ParseInLocation(dateLayout, fromRaw, adminTZ)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "from 必须是 YYYY-MM-DD 格式")
		return "", "", false
	}
	toDate, err := time.ParseInLocation(dateLayout, toRaw, adminTZ)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "to 必须是 YYYY-MM-DD 格式")
		return "", "", false
	}
	if toDate.Before(fromDate) {
		response.Fail(c, response.CodeBadRequest, "to 不能早于 from")
		return "", "", false
	}
	if toDate.Sub(fromDate) > time.Duration(maxRangeDays)*24*time.Hour {
		response.Fail(c, response.CodeBadRequest, "查询跨度不能超过 365 天")
		return "", "", false
	}
	return fromRaw, toRaw, true
}

// parseRequiredRange 解析必填 from/to，校验格式、顺序与回填跨度上限。
func parseRequiredRange(c *gin.Context) (from, to string, ok bool) {
	fromRaw, toRaw := c.Query("from"), c.Query("to")
	if fromRaw == "" || toRaw == "" {
		response.Fail(c, response.CodeBadRequest, "from 和 to 均为必填 YYYY-MM-DD")
		return "", "", false
	}

	fromDate, err := time.ParseInLocation(dateLayout, fromRaw, adminTZ)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "from 必须是 YYYY-MM-DD 格式")
		return "", "", false
	}
	toDate, err := time.ParseInLocation(dateLayout, toRaw, adminTZ)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "to 必须是 YYYY-MM-DD 格式")
		return "", "", false
	}
	if toDate.Before(fromDate) {
		response.Fail(c, response.CodeBadRequest, "to 不能早于 from")
		return "", "", false
	}

	days := int(toDate.Sub(fromDate).Hours()/24) + 1
	if days > svc.BackfillMaxDays {
		response.Fail(c, response.CodeBadRequest, "回填跨度不能超过 92 天")
		return "", "", false
	}
	return fromRaw, toRaw, true
}
