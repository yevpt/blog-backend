package analytics_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	repo "github.com/vpt/blog-backend/internal/repository/analytics"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"go.uber.org/zap"
)

// fakeQueryRepo 手写假 repo，只实现 query service 依赖的读方法。
type fakeQueryRepo struct {
	daily    []model.AnalyticsDaily
	topPages []model.AnalyticsPageDaily
	totalPV  int64
	totalUV  int64
}

func (f *fakeQueryRepo) QueryDailyRange(_ context.Context, _, _ string) ([]model.AnalyticsDaily, error) {
	return f.daily, nil
}

func (f *fakeQueryRepo) QueryTopPages(_ context.Context, _, _ string, _ int) ([]model.AnalyticsPageDaily, error) {
	return f.topPages, nil
}

func (f *fakeQueryRepo) QueryTotals(_ context.Context) (int64, int64, error) {
	return f.totalPV, f.totalUV, nil
}

// fakeRealtime 手写假实时层。
type fakeRealtime struct {
	today  svc.TodayStat
	online int64
}

func (f *fakeRealtime) TouchOnline(_ context.Context, _ string) error           { return nil }
func (f *fakeRealtime) OnlineCount(_ context.Context) (int64, error)            { return f.online, nil }
func (f *fakeRealtime) IncrToday(_ context.Context, _ model.AnalyticsEvent) error { return nil }
func (f *fakeRealtime) TodayCounters(_ context.Context) (svc.TodayStat, error)  { return f.today, nil }

func newQuerySvc(r svc.QueryReader, rt svc.Realtime) svc.QueryService {
	return svc.NewQueryService(r, rt, zap.NewNop())
}

func TestOverview(t *testing.T) {
	r := &fakeQueryRepo{totalPV: 1000, totalUV: 400}
	rt := &fakeRealtime{
		today:  svc.TodayStat{PV: 50, UV: 20, RegisteredPV: 30, RegisteredUV: 8, AnonymousPV: 20, AnonymousUV: 12},
		online: 7,
	}
	got, err := newQuerySvc(r, rt).Overview(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(50), got.TodayPV)
	assert.Equal(t, int64(20), got.TodayUV)
	assert.Equal(t, int64(7), got.Online)
	assert.Equal(t, int64(1000), got.TotalPV)
	assert.Equal(t, int64(400), got.TotalUV)
	assert.Equal(t, int64(30), got.Registered.TodayPV)
	assert.Equal(t, int64(8), got.Registered.TodayUV)
	assert.Equal(t, int64(20), got.Anonymous.TodayPV)
	assert.Equal(t, int64(12), got.Anonymous.TodayUV)
}

func TestTrendFieldSelection(t *testing.T) {
	daily := []model.AnalyticsDaily{{
		Date: "2026-06-24", PV: 100, UV: 40, Sessions: 25,
		RegisteredPV: 60, RegisteredUV: 15, AnonymousPV: 40, AnonymousUV: 25,
	}}
	r := &fakeQueryRepo{daily: daily}
	s := newQuerySvc(r, &fakeRealtime{})

	cases := []struct {
		metric, segment string
		want            int
	}{
		{"pv", "all", 100},
		{"uv", "all", 40},
		{"sessions", "all", 25},
		{"pv", "registered", 60},
		{"uv", "registered", 15},
		{"pv", "anonymous", 40},
		{"uv", "anonymous", 25},
		{"sessions", "registered", 25}, // sessions 不分档，回落到总会话
	}
	for _, c := range cases {
		pts, err := s.Trend(context.Background(), "2026-06-24", "2026-06-24", c.metric, c.segment)
		require.NoError(t, err, "%s/%s", c.metric, c.segment)
		require.Len(t, pts, 1)
		assert.Equal(t, "2026-06-24", pts[0].Date)
		assert.Equal(t, c.want, pts[0].Value, "metric=%s segment=%s", c.metric, c.segment)
	}
}

func TestTopPages(t *testing.T) {
	r := &fakeQueryRepo{topPages: []model.AnalyticsPageDaily{
		{Path: "/a", Title: "A", PV: 30, UV: 10},
		{Path: "/b", Title: "B", PV: 20, UV: 6},
	}}
	got, err := newQuerySvc(r, &fakeRealtime{}).TopPages(context.Background(), "2026-06-23", "2026-06-24", 5)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "/a", got[0].Path)
	assert.Equal(t, 30, got[0].PV)
}

// 确保 repo 实现满足 QueryReader 接口（编译期检查）。
var _ svc.QueryReader = (repo.Repository)(nil)
