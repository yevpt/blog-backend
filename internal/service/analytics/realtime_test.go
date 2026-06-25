package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func newRT(t *testing.T) (svc.Realtime, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return svc.NewRealtime(rdb, time.UTC, 90*time.Second), mr
}

func TestOnlineCount(t *testing.T) {
	rt, _ := newRT(t)
	ctx := context.Background()
	require.NoError(t, rt.TouchOnline(ctx, "v1"))
	require.NoError(t, rt.TouchOnline(ctx, "v2"))
	require.NoError(t, rt.TouchOnline(ctx, "v1"))
	n, err := rt.OnlineCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestIncrTodaySegments(t *testing.T) {
	rt, _ := newRT(t)
	ctx := context.Background()
	uid := uint(1)
	require.NoError(t, rt.IncrToday(ctx, model.AnalyticsEvent{VisitorID: "v1", UserID: &uid, IsAuthenticated: true}))
	require.NoError(t, rt.IncrToday(ctx, model.AnalyticsEvent{VisitorID: "v2", IsAuthenticated: false}))
	require.NoError(t, rt.IncrToday(ctx, model.AnalyticsEvent{VisitorID: "v2", IsAuthenticated: false}))

	st, err := rt.TodayCounters(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), st.PV)
	assert.Equal(t, int64(2), st.UV) // identity: user1 + visitor v2
	assert.Equal(t, int64(1), st.RegisteredPV)
	assert.Equal(t, int64(1), st.RegisteredUV)
	assert.Equal(t, int64(2), st.AnonymousPV)
	assert.Equal(t, int64(1), st.AnonymousUV)
}
