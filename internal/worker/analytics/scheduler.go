package analytics

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// SchedulerConfig 调度器运行参数。
type SchedulerConfig struct {
	WorkerID      string         // 实例标识，用于租约 value，便于多实例区分
	TZ            *time.Location // 切天时区（Asia/Shanghai）
	RetentionDays int            // 原始数据保留天数
	OnlineWindow  time.Duration  // 在线判定窗口，用于裁剪在线 ZSET
	Tick          time.Duration  // 调度轮询间隔
	AfterMinute   int            // 每日触发的最早分钟阈值（00:30 → 30）
	LeaseTTL      time.Duration  // Redis 租约 TTL
}

// Scheduler 周期性触发日聚合与清理：每日 00:30 后对「昨天」执行一次 RollupDay，
// 由 Redis 租约保证多实例下仅一个实例执行；进程内记录已处理日期避免重复。
type Scheduler struct {
	rollup *Rollup
	rdb    *redis.Client
	cfg    SchedulerConfig
	logger *zap.Logger

	lastRolled string // 进程内已完成聚合的日期（YYYY-MM-DD）
}

// NewScheduler 构造调度器。logger 为 nil 时退化为 Nop。
func NewScheduler(rollup *Rollup, rdb *redis.Client, cfg SchedulerConfig, logger *zap.Logger) *Scheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.TZ == nil {
		cfg.TZ = time.UTC
	}
	if cfg.Tick <= 0 {
		cfg.Tick = time.Minute
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 2 * time.Hour
	}
	return &Scheduler{rollup: rollup, rdb: rdb, cfg: cfg, logger: logger}
}

// Run 阻塞运行调度循环，直到 context 取消。
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.Tick)
	defer ticker.Stop()

	s.logger.Info("统计聚合调度器启动", zap.String("worker_id", s.cfg.WorkerID))
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("统计聚合调度器退出")
			return
		case <-ticker.C:
			s.tick(ctx, time.Now())
		}
	}
}

// tick 是一次调度判定：到达触发时刻且当日未处理时，对昨天执行聚合 + 清理。
func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	local := now.In(s.cfg.TZ)
	// 未到 00:30 不触发。
	if local.Hour() == 0 && local.Minute() < s.cfg.AfterMinute {
		return
	}
	yesterday := local.AddDate(0, 0, -1).Format("2006-01-02")
	if s.lastRolled == yesterday {
		return
	}

	// Redis 租约：SET analytics:rollup:lock:<date> <id> NX EX <ttl>，抢不到则跳过。
	lockKey := "analytics:rollup:lock:" + yesterday
	ok, err := s.rdb.SetNX(ctx, lockKey, s.cfg.WorkerID, s.cfg.LeaseTTL).Result()
	if err != nil {
		s.logger.Error("聚合租约获取失败", zap.Error(err), zap.String("date", yesterday))
		return
	}
	if !ok {
		// 其它实例已在处理或已处理；进程内标记避免反复争抢。
		s.lastRolled = yesterday
		return
	}

	if err := s.rollup.RollupDay(ctx, yesterday); err != nil {
		s.logger.Error("日聚合失败", zap.Error(err), zap.String("date", yesterday))
		return
	}
	if err := s.rollup.Cleanup(ctx, s.rdb, s.cfg.RetentionDays, s.cfg.OnlineWindow, now); err != nil {
		s.logger.Error("数据清理失败", zap.Error(err))
	}
	s.lastRolled = yesterday
}
