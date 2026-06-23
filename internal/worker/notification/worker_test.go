package notification_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notificationworker "github.com/vpt/blog-backend/internal/worker/notification"
)

func baseConfig() notificationworker.Config {
	return notificationworker.Config{
		Enabled:          true,
		EmailEnabled:     true,
		PlannerEnabled:   true,
		WorkerID:         "worker-test",
		BatchSize:        10,
		DispatchInterval: 5 * time.Millisecond,
		PlanInterval:     5 * time.Millisecond,
		SendInterval:     5 * time.Millisecond,
	}
}

// 未启用时不启动任何循环。
func TestWorker_DisabledDoesNotStartLoops(t *testing.T) {
	var calls int32
	loop := func(context.Context, string, int) (int, error) {
		atomic.AddInt32(&calls, 1)
		return 0, nil
	}
	cfg := baseConfig()
	cfg.Enabled = false
	w := notificationworker.NewWorker(cfg, loop, loop, loop, nil)

	w.Run(context.Background()) // 未启用应立即返回

	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
}

// context 取消后所有循环退出，Run 返回。
func TestWorker_StopsOnContextCancel(t *testing.T) {
	loop := func(context.Context, string, int) (int, error) { return 0, nil }
	w := notificationworker.NewWorker(baseConfig(), loop, loop, loop, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run 未在 context 取消后返回")
	}
}

// 邮件循环关闭时，站内通知 dispatcher 仍应运行。
func TestWorker_EmailDisabledStillRunsDispatcher(t *testing.T) {
	var dispatchCalls int32
	var planCalls int32
	var sendCalls int32
	dispatch := func(context.Context, string, int) (int, error) {
		atomic.AddInt32(&dispatchCalls, 1)
		return 0, nil
	}
	plan := func(context.Context, string, int) (int, error) {
		atomic.AddInt32(&planCalls, 1)
		return 0, nil
	}
	send := func(context.Context, string, int) (int, error) {
		atomic.AddInt32(&sendCalls, 1)
		return 0, nil
	}
	cfg := baseConfig()
	cfg.EmailEnabled = false
	w := notificationworker.NewWorker(cfg, dispatch, plan, send, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	time.Sleep(20 * time.Millisecond)
	cancel()

	assert.Greater(t, atomic.LoadInt32(&dispatchCalls), int32(0))
	assert.Equal(t, int32(0), atomic.LoadInt32(&planCalls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&sendCalls))
}

// 单次迭代出错后循环继续，不会停止。
func TestWorker_ContinuesAfterIterationError(t *testing.T) {
	var calls int32
	loop := func(context.Context, string, int) (int, error) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return 0, errors.New("第一次失败")
		}
		return 0, nil
	}
	noop := func(context.Context, string, int) (int, error) { return 0, nil }
	w := notificationworker.NewWorker(baseConfig(), loop, noop, noop, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	time.Sleep(40 * time.Millisecond)
	cancel()

	// 首次出错后仍应被多次调用，说明循环没有因错误中断。
	require.Greater(t, atomic.LoadInt32(&calls), int32(1))
}
