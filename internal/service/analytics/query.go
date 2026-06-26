package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	"github.com/vpt/blog-backend/internal/model"
	repo "github.com/vpt/blog-backend/internal/repository/analytics"
	"go.uber.org/zap"
)

// QueryReader 抽象后台统计读所需的 repo 方法（结构化满足 repository）。
type QueryReader interface {
	QueryDailyRange(ctx context.Context, from, to string) ([]model.AnalyticsDaily, error)
	QueryTopPages(ctx context.Context, from, to string, limit int) ([]model.AnalyticsPageDaily, error)
	QueryTotals(ctx context.Context) (pv, uv int64, err error)
	QueryDimRange(ctx context.Context, dimension, from, to string) ([]model.AnalyticsDailyDim, error)
	QuerySessionPaths(ctx context.Context, from, to string, limit int) ([]repo.SessionPath, error)
	QueryRecentActivePaths(ctx context.Context, since time.Time, limit int) ([]repo.RecentActivePath, error)
}

// funnelMaxSessions 漏斗计算时从原始事件读取的会话上限，避免大区间扫描全表。
const funnelMaxSessions = 5000

// recentActiveWindow 实时概览「最近活跃」回看窗口。
const recentActiveWindow = 5 * time.Minute

// recentActiveLimit 实时概览返回的活跃路径条数上限。
const recentActiveLimit = 10

// pathSep 在拼接去重路径序列时使用的分隔符（单元分隔符，路径中不会出现）。
const pathSep = "\x1f"

// QueryService 提供后台只读统计：总览、趋势、热门页面。返回 dto.*，不外泄 model。
type QueryService interface {
	Overview(ctx context.Context) (dto.Overview, error)
	Realtime(ctx context.Context) (dto.RealtimeStat, error)
	Trend(ctx context.Context, from, to, metric, segment string) ([]dto.TrendPoint, error)
	TopPages(ctx context.Context, from, to string, limit int) ([]dto.PageStat, error)
	Dimensions(ctx context.Context, dimension, from, to string) ([]dto.DimensionPoint, error)
	Paths(ctx context.Context, from, to string, limit int) ([]dto.PathSequence, error)
	Funnel(ctx context.Context, from, to string, steps []string) ([]dto.FunnelStep, error)
}

type queryService struct {
	repo     QueryReader
	realtime Realtime
	logger   *zap.Logger
}

// NewQueryService 注入 repo（历史累计）与 realtime（今日/在线），构造后台读服务。
func NewQueryService(repo QueryReader, realtime Realtime, logger *zap.Logger) QueryService {
	return &queryService{repo: repo, realtime: realtime, logger: logger}
}

// Overview 合并今日实时计数、在线人数与历史累计 PV/UV。
func (s *queryService) Overview(ctx context.Context) (dto.Overview, error) {
	today, err := s.realtime.TodayCounters(ctx)
	if err != nil {
		return dto.Overview{}, fmt.Errorf("读取今日计数失败: %w", err)
	}
	online, err := s.realtime.OnlineCount(ctx)
	if err != nil {
		return dto.Overview{}, fmt.Errorf("读取在线人数失败: %w", err)
	}
	totalPV, totalUV, err := s.repo.QueryTotals(ctx)
	if err != nil {
		return dto.Overview{}, fmt.Errorf("读取累计统计失败: %w", err)
	}
	return dto.Overview{
		TodayPV:    today.PV,
		TodayUV:    today.UV,
		Online:     online,
		TotalPV:    totalPV,
		TotalUV:    totalUV,
		Registered: dto.SegmentStat{TodayPV: today.RegisteredPV, TodayUV: today.RegisteredUV},
		Anonymous:  dto.SegmentStat{TodayPV: today.AnonymousPV, TodayUV: today.AnonymousUV},
	}, nil
}

// Realtime 返回实时概览：当前在线数 + 最近活跃路径（聚合）。
// online 为主信号，读取失败直接返回错误；最近活跃路径出错则降级为空并告警。
func (s *queryService) Realtime(ctx context.Context) (dto.RealtimeStat, error) {
	online, err := s.realtime.OnlineCount(ctx)
	if err != nil {
		return dto.RealtimeStat{}, fmt.Errorf("读取在线人数失败: %w", err)
	}
	out := dto.RealtimeStat{Online: online, RecentPaths: []dto.RealtimePath{}}

	rows, err := s.repo.QueryRecentActivePaths(ctx, time.Now().Add(-recentActiveWindow), recentActiveLimit)
	if err != nil {
		s.logger.Warn("读取最近活跃路径失败，降级为空", zap.Error(err))
		return out, nil
	}
	for _, row := range rows {
		out.RecentPaths = append(out.RecentPaths, dto.RealtimePath{Path: row.Path, Active: row.Active})
	}
	return out, nil
}

// Trend 读取日聚合区间，按 metric/segment 选取对应字段映射成趋势点。
// metric ∈ {pv, uv, sessions}；segment ∈ {all, registered, anonymous}。
// sessions 不分档：任何 segment 都回落到总会话数。
func (s *queryService) Trend(ctx context.Context, from, to, metric, segment string) ([]dto.TrendPoint, error) {
	rows, err := s.repo.QueryDailyRange(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("读取趋势数据失败: %w", err)
	}
	out := make([]dto.TrendPoint, 0, len(rows))
	for _, d := range rows {
		out = append(out, dto.TrendPoint{Date: d.Date, Value: pickTrendValue(d, metric, segment)})
	}
	return out, nil
}

// pickTrendValue 按 metric/segment 从日聚合行选字段。
func pickTrendValue(d model.AnalyticsDaily, metric, segment string) int {
	if metric == "sessions" {
		return d.Sessions
	}
	switch segment {
	case "registered":
		if metric == "uv" {
			return d.RegisteredUV
		}
		return d.RegisteredPV
	case "anonymous":
		if metric == "uv" {
			return d.AnonymousUV
		}
		return d.AnonymousPV
	default: // all
		if metric == "uv" {
			return d.UV
		}
		return d.PV
	}
}

// TopPages 映射 repo 的热门页面聚合为 dto.PageStat。
func (s *queryService) TopPages(ctx context.Context, from, to string, limit int) ([]dto.PageStat, error) {
	rows, err := s.repo.QueryTopPages(ctx, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("读取热门页面失败: %w", err)
	}
	out := make([]dto.PageStat, 0, len(rows))
	for _, p := range rows {
		out = append(out, dto.PageStat{Path: p.Path, Title: p.Title, PV: p.PV, UV: p.UV})
	}
	return out, nil
}

// Dimensions 读取某维度在区间内的逐日分布，映射为 dto.DimensionPoint。
func (s *queryService) Dimensions(ctx context.Context, dimension, from, to string) ([]dto.DimensionPoint, error) {
	rows, err := s.repo.QueryDimRange(ctx, dimension, from, to)
	if err != nil {
		return nil, fmt.Errorf("读取维度分布失败: %w", err)
	}
	out := make([]dto.DimensionPoint, 0, len(rows))
	for _, d := range rows {
		out = append(out, dto.DimensionPoint{Date: d.Date, DimValue: d.DimValue, PV: d.PV, UV: d.UV})
	}
	return out, nil
}

// Paths 聚合区间内多步会话的访问路径序列：拆分逗号序列、按完整序列去重计数，
// 按会话数降序返回前 limit 条。仅含聚合后的序列与会话数，不外泄个体信息。
func (s *queryService) Paths(ctx context.Context, from, to string, limit int) ([]dto.PathSequence, error) {
	rows, err := s.repo.QuerySessionPaths(ctx, from, to, funnelMaxSessions)
	if err != nil {
		return nil, fmt.Errorf("读取访问路径失败: %w", err)
	}

	type entry struct {
		parts []string
		count int
	}
	agg := make(map[string]*entry, len(rows))
	for _, row := range rows {
		parts := strings.Split(row.Sequence, ",")
		key := strings.Join(parts, pathSep)
		if e, ok := agg[key]; ok {
			e.count++
			continue
		}
		agg[key] = &entry{parts: parts, count: 1}
	}

	out := make([]dto.PathSequence, 0, len(agg))
	for _, e := range agg {
		out = append(out, dto.PathSequence{Sequence: e.parts, Sessions: e.count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sessions > out[j].Sessions })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Funnel 按 steps 给定的顺序统计逐步留存：对每个会话做有序包含匹配，
// 命中第 k 步即累加前 k 步的会话计数。转化率 = 当前步会话 / 首步会话，
// 首步在有会话时为 1.0，否则 0。
func (s *queryService) Funnel(ctx context.Context, from, to string, steps []string) ([]dto.FunnelStep, error) {
	rows, err := s.repo.QuerySessionPaths(ctx, from, to, funnelMaxSessions)
	if err != nil {
		return nil, fmt.Errorf("读取漏斗路径失败: %w", err)
	}

	counts := make([]int, len(steps))
	for _, row := range rows {
		parts := strings.Split(row.Sequence, ",")
		reached := orderedMatchCount(parts, steps)
		for i := 0; i < reached; i++ {
			counts[i]++
		}
	}

	out := make([]dto.FunnelStep, 0, len(steps))
	first := 0
	if len(counts) > 0 {
		first = counts[0]
	}
	for i, step := range steps {
		rate := 0.0
		if first > 0 {
			rate = float64(counts[i]) / float64(first)
		}
		out = append(out, dto.FunnelStep{Step: step, Sessions: counts[i], ConversionRate: rate})
	}
	return out, nil
}

// orderedMatchCount 返回 steps 在 parts 中按顺序（可不相邻）能匹配到的前缀长度。
func orderedMatchCount(parts, steps []string) int {
	idx := 0
	for _, p := range parts {
		if idx < len(steps) && p == steps[idx] {
			idx++
		}
	}
	return idx
}
