package analytics

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/internal/model"
	analyticsrepo "github.com/vpt/blog-backend/internal/repository/analytics"
	"go.uber.org/zap"
)

// onlineKey 与 service 层 realtime 的在线 ZSET key 保持一致。
const onlineKey = "analytics:online"

// RollupReader 读取某一日（Asia/Shanghai）的原始事件聚合结果。
// repo.Repository 结构上满足该接口。
type RollupReader interface {
	AggregateDay(ctx context.Context, date string) (analyticsrepo.DayAggregate, error)
}

// RollupWriter 把聚合结果落入永久聚合表。repo.Repository 结构上满足该接口。
type RollupWriter interface {
	UpsertDaily(ctx context.Context, d model.AnalyticsDaily) error
	UpsertDailyDim(ctx context.Context, rows []model.AnalyticsDailyDim) error
	UpsertPageDaily(ctx context.Context, rows []model.AnalyticsPageDaily) error
}

// RollupCleaner 删除过期的原始事件与会话。repo.Repository 结构上满足该接口。
type RollupCleaner interface {
	DeleteEventsBefore(ctx context.Context, t time.Time) (int64, error)
	DeleteSessionsBefore(ctx context.Context, t time.Time) (int64, error)
}

// Rollup 负责日聚合落库与过期数据清理，是 worker 的纯业务核心（不含调度）。
type Rollup struct {
	reader  RollupReader
	writer  RollupWriter
	cleaner RollupCleaner
	logger  *zap.Logger
}

// NewRollup 构造日聚合器。writer 同时须满足 RollupCleaner（repo.Repository 即可）。
// logger 为 nil 时退化为 Nop。
func NewRollup(reader RollupReader, writer RollupWriter, logger *zap.Logger) *Rollup {
	if logger == nil {
		logger = zap.NewNop()
	}
	cleaner, _ := writer.(RollupCleaner)
	return &Rollup{reader: reader, writer: writer, cleaner: cleaner, logger: logger}
}

// RollupDay 聚合指定日（YYYY-MM-DD，Asia/Shanghai）并 upsert 到三张永久聚合表。
func (r *Rollup) RollupDay(ctx context.Context, date string) error {
	agg, err := r.reader.AggregateDay(ctx, date)
	if err != nil {
		return err
	}
	if err := r.writer.UpsertDaily(ctx, agg.Daily); err != nil {
		return err
	}
	if err := r.writer.UpsertDailyDim(ctx, agg.Dims); err != nil {
		return err
	}
	if err := r.writer.UpsertPageDaily(ctx, agg.Pages); err != nil {
		return err
	}
	r.logger.Info("日聚合完成",
		zap.String("date", date),
		zap.Int("pv", agg.Daily.PV),
		zap.Int("uv", agg.Daily.UV),
		zap.Int("dims", len(agg.Dims)),
		zap.Int("pages", len(agg.Pages)),
	)
	return nil
}

// Cleanup 删除 retentionDays 之前的原始事件与会话，并裁剪在线 ZSET 中超出窗口的成员。
// now 由调用方注入，便于测试。
func (r *Rollup) Cleanup(ctx context.Context, rdb *redis.Client, retentionDays int, onlineWindow time.Duration, now time.Time) error {
	cutoff := now.AddDate(0, 0, -retentionDays)

	if r.cleaner != nil {
		if n, err := r.cleaner.DeleteEventsBefore(ctx, cutoff); err != nil {
			r.logger.Error("清理过期事件失败", zap.Error(err))
		} else {
			r.logger.Info("清理过期事件", zap.Int64("deleted", n), zap.Time("before", cutoff))
		}
		if n, err := r.cleaner.DeleteSessionsBefore(ctx, cutoff); err != nil {
			r.logger.Error("清理过期会话失败", zap.Error(err))
		} else {
			r.logger.Info("清理过期会话", zap.Int64("deleted", n), zap.Time("before", cutoff))
		}
	}

	// 裁剪在线 ZSET：移除 score < now-onlineWindow 的成员。
	if rdb != nil {
		max := "(" + strconv.FormatInt(now.Add(-onlineWindow).Unix(), 10)
		if n, err := rdb.ZRemRangeByScore(ctx, onlineKey, "-inf", max).Result(); err != nil {
			r.logger.Error("裁剪在线表失败", zap.Error(err))
		} else if n > 0 {
			r.logger.Info("裁剪在线表", zap.Int64("removed", n))
		}
	}
	return nil
}
