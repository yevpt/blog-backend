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
	analyticsrepo "github.com/vpt/blog-backend/internal/repository/analytics"
	worker "github.com/vpt/blog-backend/internal/worker/analytics"
	"go.uber.org/zap"
)

// fakeReader 记录 AggregateDay 入参并返回预置聚合结果。
type fakeReader struct {
	gotDate string
	agg     analyticsrepo.DayAggregate
	err     error
}

func (f *fakeReader) AggregateDay(_ context.Context, date string) (analyticsrepo.DayAggregate, error) {
	f.gotDate = date
	return f.agg, f.err
}

// recRepo 记录 upsert 与清理调用次数及入参，供断言。
type recRepo struct {
	dailyCalls    int
	dimCalls      int
	pageCalls     int
	replaceDim    []model.AnalyticsDailyDim
	replacePage   []model.AnalyticsPageDaily
	replaceDimAt  string
	replacePageAt string

	delEventsCutoff   time.Time
	delSessionsCutoff time.Time
	delEventsCalls    int
	delSessionsCalls  int
}

func (r *recRepo) UpsertDaily(_ context.Context, _ model.AnalyticsDaily) error {
	r.dailyCalls++
	return nil
}

func (r *recRepo) ReplaceDailyDims(_ context.Context, date string, rows []model.AnalyticsDailyDim) error {
	r.dimCalls++
	r.replaceDimAt = date
	r.replaceDim = rows
	return nil
}

func (r *recRepo) ReplacePageDaily(_ context.Context, date string, rows []model.AnalyticsPageDaily) error {
	r.pageCalls++
	r.replacePageAt = date
	r.replacePage = rows
	return nil
}

func (r *recRepo) DeleteEventsBefore(_ context.Context, t time.Time) (int64, error) {
	r.delEventsCalls++
	r.delEventsCutoff = t
	return 3, nil
}

func (r *recRepo) DeleteSessionsBefore(_ context.Context, t time.Time) (int64, error) {
	r.delSessionsCalls++
	r.delSessionsCutoff = t
	return 2, nil
}

func TestRollupDay(t *testing.T) {
	reader := &fakeReader{agg: analyticsrepo.DayAggregate{
		Daily: model.AnalyticsDaily{Date: "2026-06-24", PV: 10, UV: 5},
		Dims:  []model.AnalyticsDailyDim{{Date: "2026-06-24", Dimension: "device", DimValue: "mobile", PV: 6, UV: 3}},
		Pages: []model.AnalyticsPageDaily{{Date: "2026-06-24", Path: "/", PV: 4, UV: 2}},
	}}
	rec := &recRepo{}
	r := worker.NewRollup(reader, rec, zap.NewNop())

	require.NoError(t, r.RollupDay(context.Background(), "2026-06-24"))

	assert.Equal(t, "2026-06-24", reader.gotDate)
	assert.Equal(t, 1, rec.dailyCalls)
	assert.Equal(t, 1, rec.dimCalls)
	assert.Equal(t, "2026-06-24", rec.replaceDimAt)
	assert.Len(t, rec.replaceDim, 1)
	assert.Equal(t, 1, rec.pageCalls)
	assert.Equal(t, "2026-06-24", rec.replacePageAt)
	assert.Len(t, rec.replacePage, 1)
}

func TestCleanup(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// 预置在线 ZSET：一个过期成员、一个在窗口内的成员。
	now := time.Now()
	_, err = rdb.ZAdd(context.Background(), "analytics:online",
		redis.Z{Score: float64(now.Add(-10 * time.Minute).Unix()), Member: "stale"},
		redis.Z{Score: float64(now.Unix()), Member: "fresh"},
	).Result()
	require.NoError(t, err)

	rec := &recRepo{}
	r := worker.NewRollup(&fakeReader{}, rec, zap.NewNop())

	require.NoError(t, r.Cleanup(context.Background(), rdb, 90, 90*time.Second, now))

	// 事件 / 会话清理各调用一次，截止时间约为 now-90 天。
	assert.Equal(t, 1, rec.delEventsCalls)
	assert.Equal(t, 1, rec.delSessionsCalls)
	expectedCutoff := now.AddDate(0, 0, -90)
	assert.WithinDuration(t, expectedCutoff, rec.delEventsCutoff, time.Second)
	assert.WithinDuration(t, expectedCutoff, rec.delSessionsCutoff, time.Second)

	// 在线 ZSET 仅保留窗口内成员。
	members, err := rdb.ZRange(context.Background(), "analytics:online", 0, -1).Result()
	require.NoError(t, err)
	assert.Equal(t, []string{"fresh"}, members)
}
