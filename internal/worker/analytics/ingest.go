package analytics

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"go.uber.org/zap"
)

// RepoForIngest 是 ingest worker 依赖的最小写入接口（便于测试）。
type RepoForIngest interface {
	InsertEvents(ctx context.Context, events []model.AnalyticsEvent) error
}

// Ingestor 通过有界 channel 异步批量落库事件。
type Ingestor interface {
	Submit(ev model.AnalyticsEvent) bool
	Run(ctx context.Context)
	Dropped() int64
}

type ingestor struct {
	repo          RepoForIngest
	ch            chan model.AnalyticsEvent
	batchSize     int
	flushInterval time.Duration
	logger        *zap.Logger
	dropped       atomic.Int64
}

// NewIngestor 构造异步落库器。bufferSize 为 channel 容量，满则丢弃。
func NewIngestor(repo RepoForIngest, bufferSize, batchSize int, flushInterval time.Duration, logger *zap.Logger) Ingestor {
	return &ingestor{
		repo:          repo,
		ch:            make(chan model.AnalyticsEvent, bufferSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		logger:        logger,
	}
}

// Submit 非阻塞投递事件；channel 满则累加丢弃计数并返回 false。
func (i *ingestor) Submit(ev model.AnalyticsEvent) bool {
	select {
	case i.ch <- ev:
		return true
	default:
		i.dropped.Add(1)
		return false
	}
}

// Dropped 返回累计丢弃的事件数。
func (i *ingestor) Dropped() int64 { return i.dropped.Load() }

// Run 消费 channel，按批量大小 / 定时器 / ctx 取消三种时机落库。
func (i *ingestor) Run(ctx context.Context) {
	ticker := time.NewTicker(i.flushInterval)
	defer ticker.Stop()
	buf := make([]model.AnalyticsEvent, 0, i.batchSize)

	// flush 用传入的 flushCtx 落库；周期/批量刷盘用存活的 ctx，
	// 关闭路径用脱离取消的短超时 ctx，避免最后一批因 ctx 取消而丢失。
	flush := func(flushCtx context.Context) {
		if len(buf) == 0 {
			return
		}
		if err := i.repo.InsertEvents(flushCtx, buf); err != nil {
			i.logger.Error("统计事件落库失败", zap.Error(err), zap.Int("count", len(buf)))
		}
		buf = buf[:0]
	}

	for {
		select {
		case <-ctx.Done():
			// 收尾刷盘脱离已取消的 ctx，给一个短超时确保最后一批落库。
			finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			flush(finalCtx)
			cancel()
			return
		case ev := <-i.ch:
			buf = append(buf, ev)
			if len(buf) >= i.batchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}
