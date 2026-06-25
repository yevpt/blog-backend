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
	validMetrics  = map[string]struct{}{"pv": {}, "uv": {}, "sessions": {}}
	validSegments = map[string]struct{}{"all": {}, "registered": {}, "anonymous": {}}
)

// AdminHandler 后台统计查询入口，只做参数解析/校验与统一响应。
type AdminHandler struct {
	svc svc.QueryService
}

// NewAdminHandler 注入只读查询服务。
func NewAdminHandler(s svc.QueryService) *AdminHandler {
	return &AdminHandler{svc: s}
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
