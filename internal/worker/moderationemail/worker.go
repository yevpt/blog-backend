// Package moderationemail 编排待审核摘要邮件的规划与发送循环。
package moderationemail

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const defaultBatchSize = 20

// LoopFunc 是审核邮件 worker 的单次处理函数。
type LoopFunc func(ctx context.Context, workerID string, limit int) (int, error)

// Config 定义审核邮件 worker 的运行参数。
type Config struct {
	Enabled      bool             // 是否启动 worker
	WorkerID     string           // worker 标识，用于租约 locked_by
	PollInterval time.Duration    // 轮询间隔
	Tick         <-chan time.Time // 测试注入的 tick 通道；生产为空时使用 PollInterval 创建 ticker
}

// Worker 按固定有界批次先规划再发送待审核摘要邮件。
type Worker struct {
	cfg    Config
	plan   LoopFunc
	send   LoopFunc
	logger *zap.Logger
}

// NewWorker 通过构造注入创建审核邮件 worker。
func NewWorker(cfg Config, plan, send LoopFunc, logger *zap.Logger) *Worker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Worker{cfg: cfg, plan: plan, send: send, logger: logger}
}

// Run 立即执行一轮，随后按配置间隔循环；单次错误只记录并继续。
func (w *Worker) Run(ctx context.Context) {
	if w == nil || !w.cfg.Enabled || w.cfg.PollInterval <= 0 {
		return
	}
	ticks, stop := w.ticks()
	defer stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		w.invoke(ctx, "planner", w.plan)
		w.invoke(ctx, "sender", w.send)

		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
		}
	}
}

func (w *Worker) ticks() (<-chan time.Time, func()) {
	if w.cfg.Tick != nil {
		return w.cfg.Tick, func() {}
	}
	ticker := time.NewTicker(w.cfg.PollInterval)
	return ticker.C, ticker.Stop
}

func (w *Worker) invoke(ctx context.Context, name string, fn LoopFunc) {
	if fn == nil {
		return
	}
	if _, err := fn(ctx, w.cfg.WorkerID, defaultBatchSize); err != nil {
		w.logger.Warn("审核邮件 worker 迭代失败", zap.String("loop", name), zap.Error(err))
	}
}
