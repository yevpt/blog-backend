package analytics_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	worker "github.com/vpt/blog-backend/internal/worker/analytics"
	"go.uber.org/zap"
)

// fakeRepo 直接实现 RepoForIngest，记录落库的事件以供断言。
type fakeRepo struct {
	mu     sync.Mutex
	events []model.AnalyticsEvent
}

func (f *fakeRepo) InsertEvents(_ context.Context, evs []model.AnalyticsEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, evs...)
	return nil
}

func (f *fakeRepo) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.events) }

func TestIngestorFlush(t *testing.T) {
	fr := &fakeRepo{}
	ing := worker.NewIngestor(fr, 16, 8, 20*time.Millisecond, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	go ing.Run(ctx)

	for i := 0; i < 5; i++ {
		assert.True(t, ing.Submit(model.AnalyticsEvent{EventType: "page_view"}))
	}
	require.Eventually(t, func() bool { return fr.count() == 5 }, time.Second, 10*time.Millisecond)
	cancel()
}

func TestIngestorDropWhenFull(t *testing.T) {
	fr := &fakeRepo{}
	ing := worker.NewIngestor(fr, 2, 8, time.Hour, zap.NewNop()) // 不 flush，撑满
	// 不启动 Run，channel 很快满
	ok := 0
	for i := 0; i < 10; i++ {
		if ing.Submit(model.AnalyticsEvent{}) {
			ok++
		}
	}
	assert.LessOrEqual(t, ok, 2)
	assert.GreaterOrEqual(t, ing.Dropped(), int64(8))
}
