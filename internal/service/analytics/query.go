package analytics

import (
	"context"
	"fmt"

	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	"github.com/vpt/blog-backend/internal/model"
	"go.uber.org/zap"
)

// QueryReader 抽象后台统计读所需的 repo 方法（结构化满足 repository）。
type QueryReader interface {
	QueryDailyRange(ctx context.Context, from, to string) ([]model.AnalyticsDaily, error)
	QueryTopPages(ctx context.Context, from, to string, limit int) ([]model.AnalyticsPageDaily, error)
	QueryTotals(ctx context.Context) (pv, uv int64, err error)
}

// QueryService 提供后台只读统计：总览、趋势、热门页面。返回 dto.*，不外泄 model。
type QueryService interface {
	Overview(ctx context.Context) (dto.Overview, error)
	Trend(ctx context.Context, from, to, metric, segment string) ([]dto.TrendPoint, error)
	TopPages(ctx context.Context, from, to string, limit int) ([]dto.PageStat, error)
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
