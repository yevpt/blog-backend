package bootstrap

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/zap"
)

// ErrTaskShutdownTimeout 表示后台任务未能在关闭期限内全部退出。
var ErrTaskShutdownTimeout = errors.New("后台任务关闭超时")

// TaskGroup 统一管理随应用生命周期启动和停止的后台任务。
type TaskGroup struct {
	ctx    context.Context
	logger *zap.Logger
	wg     sync.WaitGroup
}

// NewTaskGroup 创建共享同一生命周期 context 的后台任务组。
func NewTaskGroup(ctx context.Context, logger *zap.Logger) *TaskGroup {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &TaskGroup{ctx: ctx, logger: logger}
}

// Go 启动一个后台任务，并在任务退出时计入统一等待流程。
func (g *TaskGroup) Go(name string, run func(context.Context)) {
	if g == nil || run == nil {
		return
	}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		run(g.ctx)
		g.logger.Info("后台任务已停止", zap.String("task", name))
	}()
}

// Wait 等待全部后台任务退出，超过调用方给定期限时返回错误。
func (g *TaskGroup) Wait(ctx context.Context) error {
	if g == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return errors.Join(ErrTaskShutdownTimeout, ctx.Err())
	}
}
