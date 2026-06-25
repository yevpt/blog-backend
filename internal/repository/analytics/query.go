package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// shanghai 是聚合切天口径。AggregateDay 用它把 YYYY-MM-DD 解析成 UTC 区间。
var shanghai = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}()

func (r *repository) QueryDailyRange(ctx context.Context, from, to string) ([]model.AnalyticsDaily, error) {
	var out []model.AnalyticsDaily
	err := r.db.WithContext(ctx).
		Where("date >= ? AND date <= ?", from, to).
		Order("date asc").Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("查询日趋势失败: %w", err)
	}
	return out, nil
}

func (r *repository) QueryDimRange(ctx context.Context, dimension, from, to string) ([]model.AnalyticsDailyDim, error) {
	var out []model.AnalyticsDailyDim
	err := r.db.WithContext(ctx).
		Where("dimension = ? AND date >= ? AND date <= ?", dimension, from, to).
		Order("date asc").Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("查询维度区间失败: %w", err)
	}
	return out, nil
}

func (r *repository) QueryTopPages(ctx context.Context, from, to string, limit int) ([]model.AnalyticsPageDaily, error) {
	var out []model.AnalyticsPageDaily
	err := r.db.WithContext(ctx).
		Model(&model.AnalyticsPageDaily{}).
		Select("path, max(title) as title, sum(pv) as pv, sum(uv) as uv").
		Where("date >= ? AND date <= ?", from, to).
		Group("path").Order("pv desc").Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("查询热门页面失败: %w", err)
	}
	return out, nil
}

func (r *repository) QueryTotals(ctx context.Context) (pv, uv int64, err error) {
	var row struct {
		PV int64
		UV int64
	}
	e := r.db.WithContext(ctx).
		Model(&model.AnalyticsDaily{}).
		Select("COALESCE(SUM(pv),0) as pv, COALESCE(SUM(uv),0) as uv").
		Scan(&row).Error
	if e != nil {
		return 0, 0, fmt.Errorf("查询累计 PV/UV 失败: %w", e)
	}
	return row.PV, row.UV, nil
}

// aggregateDims 列出需要逐列 GROUP BY 的维度及其源列表达式。
// user_type 由 is_authenticated 推导出 registered/anonymous。
var aggregateDims = []struct {
	dimension string
	column    string
}{
	{"device", "device_type"},
	{"browser", "browser"},
	{"os", "os"},
	{"referer_type", "referer_type"},
	{"country", "country"},
	{"user_type", "CASE WHEN is_authenticated THEN 'registered' ELSE 'anonymous' END"},
}

// AggregateDay 读取指定日（Asia/Shanghai）的原始事件，聚合成 DayAggregate。
// 过滤 is_bot=0 AND is_suspect=0，identity 为 COALESCE(user_id, visitor_id)。
// TODO(Phase2): avg_duration / bounce_rate 需从 analytics_sessions 计算，当前置 0。
func (r *repository) AggregateDay(ctx context.Context, date string) (DayAggregate, error) {
	start, end, err := dayRangeUTC(date)
	if err != nil {
		return DayAggregate{}, err
	}

	agg := DayAggregate{Daily: model.AnalyticsDaily{Date: date}}

	// 1) Daily 总览聚合
	var daily struct {
		PV           int
		UV           int
		RegisteredPV int
		RegisteredUV int
		AnonymousPV  int
		AnonymousUV  int
		Sessions     int
		NewVisitors  int
	}
	dailySelect := "COUNT(*) as pv, " +
		"COUNT(DISTINCT COALESCE(user_id, visitor_id)) as uv, " +
		"SUM(is_authenticated) as registered_pv, " +
		"COUNT(DISTINCT CASE WHEN is_authenticated THEN user_id END) as registered_uv, " +
		"SUM(NOT is_authenticated) as anonymous_pv, " +
		"COUNT(DISTINCT CASE WHEN NOT is_authenticated THEN visitor_id END) as anonymous_uv, " +
		"COUNT(DISTINCT session_id) as sessions, " +
		"COUNT(DISTINCT CASE WHEN is_new_visitor THEN visitor_id END) as new_visitors"
	if e := r.eventScope(ctx, start, end).
		Select(dailySelect).Scan(&daily).Error; e != nil {
		return DayAggregate{}, fmt.Errorf("聚合日总览失败: %w", e)
	}
	agg.Daily.PV = daily.PV
	agg.Daily.UV = daily.UV
	agg.Daily.RegisteredPV = daily.RegisteredPV
	agg.Daily.RegisteredUV = daily.RegisteredUV
	agg.Daily.AnonymousPV = daily.AnonymousPV
	agg.Daily.AnonymousUV = daily.AnonymousUV
	agg.Daily.Sessions = daily.Sessions
	agg.Daily.NewVisitors = daily.NewVisitors

	// 2) 各维度 GROUP BY
	for _, d := range aggregateDims {
		var rows []struct {
			DimValue string
			PV       int
			UV       int
		}
		sel := fmt.Sprintf(
			"%s as dim_value, COUNT(*) as pv, COUNT(DISTINCT COALESCE(user_id, visitor_id)) as uv",
			d.column,
		)
		if e := r.eventScope(ctx, start, end).
			Select(sel).Group(d.column).Scan(&rows).Error; e != nil {
			return DayAggregate{}, fmt.Errorf("聚合维度 %s 失败: %w", d.dimension, e)
		}
		for _, row := range rows {
			agg.Dims = append(agg.Dims, model.AnalyticsDailyDim{
				Date: date, Dimension: d.dimension, DimValue: row.DimValue,
				PV: row.PV, UV: row.UV,
			})
		}
	}

	// 3) 页面 GROUP BY
	var pages []struct {
		Path  string
		Title string
		PV    int
		UV    int
	}
	if e := r.eventScope(ctx, start, end).
		Select("path, max(title) as title, COUNT(*) as pv, COUNT(DISTINCT COALESCE(user_id, visitor_id)) as uv").
		Group("path").Scan(&pages).Error; e != nil {
		return DayAggregate{}, fmt.Errorf("聚合页面失败: %w", e)
	}
	for _, p := range pages {
		agg.Pages = append(agg.Pages, model.AnalyticsPageDaily{
			Date: date, Path: p.Path, Title: p.Title, PV: p.PV, UV: p.UV,
		})
	}

	return agg, nil
}

// eventScope 返回带「当日有效事件」过滤条件的查询 builder。
func (r *repository) eventScope(ctx context.Context, start, end time.Time) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.AnalyticsEvent{}).
		Where("created_at >= ? AND created_at < ?", start, end).
		Where("is_bot = ? AND is_suspect = ?", false, false)
}

// dayRangeUTC 把 Asia/Shanghai 的 YYYY-MM-DD 解析成 [start,end) 的 UTC 时间区间。
func dayRangeUTC(date string) (start, end time.Time, err error) {
	d, e := time.ParseInLocation("2006-01-02", date, shanghai)
	if e != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("解析聚合日期失败: %w", e)
	}
	return d.UTC(), d.Add(24 * time.Hour).UTC(), nil
}
