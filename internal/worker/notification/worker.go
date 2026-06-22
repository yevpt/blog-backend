// Package notification 提供通知后台 worker 的启动与循环编排。
//
// worker 把 dispatcher、planner、sender 三条循环按各自间隔周期性触发，
// 单次迭代出错只记日志并继续；context 取消时所有循环优雅退出。
// 任务领取依赖 MySQL 租约，宕机重启后可继续处理，无需额外恢复逻辑。
package notification

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// LoopFunc 是一次循环迭代，返回处理数量与错误。
// dispatcher.DispatchOnce、planner.PlanOnce、sender.SendOnce 均匹配该签名。
type LoopFunc func(ctx context.Context, workerID string, limit int) (int, error)

// Config 通知 worker 的运行配置。
type Config struct {
	Enabled          bool          // 是否启动 dispatcher/sender 循环
	PlannerEnabled   bool          // 是否启动 planner 循环
	WorkerID         string        // worker 标识，用于任务租约 locked_by
	BatchSize        int           // 单次迭代领取数量
	DispatchInterval time.Duration // 事件分发间隔
	PlanInterval     time.Duration // 邮件聚合间隔
	SendInterval     time.Duration // 邮件发送间隔
}

// Worker 周期性触发三条通知循环。
type Worker struct {
	cfg      Config
	dispatch LoopFunc
	plan     LoopFunc
	send     LoopFunc
	logger   *zap.Logger
}

// NewWorker 创建通知 worker。logger 为 nil 时使用 Nop logger。
func NewWorker(cfg Config, dispatch, plan, send LoopFunc, logger *zap.Logger) *Worker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Worker{cfg: cfg, dispatch: dispatch, plan: plan, send: send, logger: logger}
}

// Run 启动各循环并阻塞，直到 context 取消后所有循环退出才返回。
// 未启用时立即返回，不启动任何循环。
func (w *Worker) Run(ctx context.Context) {
	if !w.cfg.Enabled {
		return
	}

	type namedLoop struct {
		name     string
		interval time.Duration
		fn       LoopFunc
	}
	loops := []namedLoop{
		{"dispatcher", w.cfg.DispatchInterval, w.dispatch},
		{"sender", w.cfg.SendInterval, w.send},
	}
	// planner 可独立开关，便于多实例下只在单实例聚合。
	if w.cfg.PlannerEnabled {
		loops = append(loops, namedLoop{"planner", w.cfg.PlanInterval, w.plan})
	}

	var wg sync.WaitGroup
	for _, l := range loops {
		if l.fn == nil || l.interval <= 0 {
			continue
		}
		wg.Add(1)
		go func(name string, interval time.Duration, fn LoopFunc) {
			defer wg.Done()
			w.runLoop(ctx, name, interval, fn)
		}(l.name, l.interval, l.fn)
	}
	wg.Wait()
}

// runLoop 立即执行一次随后按间隔循环；单次出错记录日志后继续，不中断循环。
func (w *Worker) runLoop(ctx context.Context, name string, interval time.Duration, fn LoopFunc) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		w.invoke(ctx, name, fn)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// invoke 执行一次迭代并吞掉错误（仅记录），保证循环韧性。
func (w *Worker) invoke(ctx context.Context, name string, fn LoopFunc) {
	if _, err := fn(ctx, w.cfg.WorkerID, w.cfg.BatchSize); err != nil {
		w.logger.Warn("通知 worker 迭代失败", zap.String("loop", name), zap.Error(err))
	}
}
