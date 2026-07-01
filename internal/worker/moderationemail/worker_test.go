package moderationemail_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationemailworker "github.com/vpt/blog-backend/internal/worker/moderationemail"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestModerationReviewEmailWorkerRunsPlannerBeforeSenderImmediatelyAndOnTicks(t *testing.T) {
	ticks := make(chan time.Time)
	calls := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := moderationemailworker.NewWorker(moderationemailworker.Config{
		Enabled:      true,
		WorkerID:     "worker-1",
		PollInterval: time.Hour,
		Tick:         ticks,
	}, recordLoop(calls, "plan"), recordLoop(calls, "send"), zap.NewNop())

	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	assert.Equal(t, "plan:worker-1:20", waitWorkerCall(t, calls))
	assert.Equal(t, "send:worker-1:20", waitWorkerCall(t, calls))

	ticks <- time.Now()
	assert.Equal(t, "plan:worker-1:20", waitWorkerCall(t, calls))
	assert.Equal(t, "send:worker-1:20", waitWorkerCall(t, calls))

	cancel()
	waitWorkerDone(t, done)
}

func TestModerationReviewEmailWorkerLogsPlannerAndSenderErrorsAndContinues(t *testing.T) {
	ticks := make(chan time.Time)
	calls := make(chan string, 4)
	core, logs := observer.New(zap.WarnLevel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := moderationemailworker.NewWorker(moderationemailworker.Config{
		Enabled:      true,
		WorkerID:     "worker-1",
		PollInterval: time.Hour,
		Tick:         ticks,
	}, errorLoop(calls, "plan", errors.New("plan down")), errorLoop(calls, "send", errors.New("send down")), zap.New(core))

	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	require.Equal(t, "plan", waitWorkerCall(t, calls))
	require.Equal(t, "send", waitWorkerCall(t, calls))
	waitWorkerLogs(t, logs, 2)

	ticks <- time.Now()
	assert.Equal(t, "plan", waitWorkerCall(t, calls))
	assert.Equal(t, "send", waitWorkerCall(t, calls))

	cancel()
	waitWorkerDone(t, done)
}

func TestModerationReviewEmailWorkerStopsOnCancellation(t *testing.T) {
	ticks := make(chan time.Time)
	calls := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())

	worker := moderationemailworker.NewWorker(moderationemailworker.Config{
		Enabled:      true,
		WorkerID:     "worker-1",
		PollInterval: time.Hour,
		Tick:         ticks,
	}, recordLoop(calls, "plan"), recordLoop(calls, "send"), zap.NewNop())

	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	require.Equal(t, "plan:worker-1:20", waitWorkerCall(t, calls))
	require.Equal(t, "send:worker-1:20", waitWorkerCall(t, calls))

	cancel()
	waitWorkerDone(t, done)

	select {
	case ticks <- time.Now():
	case <-time.After(50 * time.Millisecond):
	}
	assert.Empty(t, calls)
}

func recordLoop(calls chan<- string, name string) moderationemailworker.LoopFunc {
	return func(_ context.Context, workerID string, limit int) (int, error) {
		calls <- fmt.Sprintf("%s:%s:%d", name, workerID, limit)
		if limit != 20 {
			return 0, errors.New("unexpected limit")
		}
		return 1, nil
	}
}

func errorLoop(calls chan<- string, name string, err error) moderationemailworker.LoopFunc {
	return func(context.Context, string, int) (int, error) {
		calls <- name
		return 0, err
	}
}

func waitWorkerCall(t *testing.T, calls <-chan string) string {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for worker call")
	}
	return ""
}

func waitWorkerDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for worker stop")
	}
}

func waitWorkerLogs(t *testing.T, logs *observer.ObservedLogs, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return logs.FilterMessage("审核邮件 worker 迭代失败").Len() == want
	}, time.Second, 10*time.Millisecond)
}
