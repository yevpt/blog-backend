package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	"github.com/vpt/blog-backend/internal/model"
	"go.uber.org/zap"
)

// PublicReader 抽象前台公开统计所需的 repo 方法。
type PublicReader interface {
	QueryTotals(ctx context.Context) (pv, uv int64, err error)
	QueryTotalsSegmented(ctx context.Context) (total, registered, anonymous int64, err error)
	QueryTopPagesPublic(ctx context.Context, from, to string, limit int) ([]model.AnalyticsPageDaily, error)
}

// PublicService 提供前台公开聚合数据（仅聚合数字），结果走 Redis 短 TTL 缓存。
type PublicService interface {
	Summary(ctx context.Context) (dto.PublicSummary, error)
	Popular(ctx context.Context, limit int) ([]dto.PublicPageStat, error)
}

type publicService struct {
	repo     PublicReader
	realtime Realtime
	rdb      *redis.Client
	cacheTTL time.Duration
	logger   *zap.Logger
}

// NewPublicService 注入 repo（累计/榜单）、realtime（今日/在线）与 Redis（响应缓存）。
// rdb 为 nil 时缓存读写均为无操作，逻辑仍可正常运行。
func NewPublicService(repo PublicReader, realtime Realtime, rdb *redis.Client, cacheTTL time.Duration, logger *zap.Logger) PublicService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &publicService{repo: repo, realtime: realtime, rdb: rdb, cacheTTL: cacheTTL, logger: logger}
}

const (
	publicSummaryKey = "analytics:public:summary"
	publicPopularKey = "analytics:public:popular:" // + limit
)

func (s *publicService) getCached(ctx context.Context, key string, out any) bool {
	if s.rdb == nil {
		return false
	}
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func (s *publicService) setCached(ctx context.Context, key string, v any) {
	if s.rdb == nil {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := s.rdb.Set(ctx, key, raw, s.cacheTTL).Err(); err != nil {
		s.logger.Warn("公开统计缓存写入失败", zap.String("key", key), zap.Error(err))
	}
}

// Summary 前台公开总览：今日 PV/UV + 在线 + 累计 PV/UV + 注册/匿名 UV，仅聚合数字。
func (s *publicService) Summary(ctx context.Context) (dto.PublicSummary, error) {
	var out dto.PublicSummary
	if s.getCached(ctx, publicSummaryKey, &out) {
		return out, nil
	}
	today, err := s.realtime.TodayCounters(ctx)
	if err != nil {
		return dto.PublicSummary{}, fmt.Errorf("读取今日计数失败: %w", err)
	}
	online, err := s.realtime.OnlineCount(ctx)
	if err != nil {
		return dto.PublicSummary{}, fmt.Errorf("读取在线人数失败: %w", err)
	}
	totalPV, _, err := s.repo.QueryTotals(ctx)
	if err != nil {
		return dto.PublicSummary{}, fmt.Errorf("读取累计统计失败: %w", err)
	}
	totalUV, regUV, anonUV, err := s.repo.QueryTotalsSegmented(ctx)
	if err != nil {
		return dto.PublicSummary{}, fmt.Errorf("读取分档累计失败: %w", err)
	}
	out = dto.PublicSummary{
		TodayPV: today.PV, TodayUV: today.UV, Online: online,
		TotalPV: totalPV, TotalUV: totalUV, RegisteredUV: regUV, AnonymousUV: anonUV,
	}
	s.setCached(ctx, publicSummaryKey, out)
	return out, nil
}

// Popular 前台热门页面榜（近 30 天，排除 /admin/*），仅暴露 path/title/pv。
func (s *publicService) Popular(ctx context.Context, limit int) ([]dto.PublicPageStat, error) {
	key := fmt.Sprintf("%s%d", publicPopularKey, limit)
	var out []dto.PublicPageStat
	if s.getCached(ctx, key, &out) {
		return out, nil
	}
	tz := shanghaiTZ()
	now := time.Now().In(tz)
	to := now.Format("2006-01-02")
	from := now.AddDate(0, 0, -29).Format("2006-01-02") // 近 30 天
	rows, err := s.repo.QueryTopPagesPublic(ctx, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("读取前台热门页面失败: %w", err)
	}
	out = make([]dto.PublicPageStat, 0, len(rows))
	for _, p := range rows {
		out = append(out, dto.PublicPageStat{Path: p.Path, Title: p.Title, PV: p.PV})
	}
	s.setCached(ctx, key, out)
	return out, nil
}

// shanghaiTZ 与聚合口径一致的东八区（加载失败回退固定时区）。
func shanghaiTZ() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}
