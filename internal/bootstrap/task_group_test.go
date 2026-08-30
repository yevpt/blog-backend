package bootstrap_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/bootstrap"
	"go.uber.org/zap"
)

func TestTaskGroupWaitsForCanceledTask(t *testing.T) {
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	tasks := bootstrap.NewTaskGroup(workerCtx, zap.NewNop())
	started := make(chan struct{})
	tasks.Go("test", func(ctx context.Context) {
		close(started)
		<-ctx.Done()
	})
	<-started
	cancelWorker()

	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	require.NoError(t, tasks.Wait(waitCtx))
}

func TestTaskGroupReturnsTimeout(t *testing.T) {
	tasks := bootstrap.NewTaskGroup(context.Background(), zap.NewNop())
	release := make(chan struct{})
	tasks.Go("blocked", func(context.Context) {
		<-release
	})

	waitCtx, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	err := tasks.Wait(waitCtx)
	require.Error(t, err)
	require.True(t, errors.Is(err, bootstrap.ErrTaskShutdownTimeout))

	close(release)
	cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), time.Second)
	defer cancelCleanup()
	require.NoError(t, tasks.Wait(cleanupCtx))
}
