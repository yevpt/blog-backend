package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"go.uber.org/zap"
)

type fakePublicRepo struct {
	totalPV          int64
	total, reg, anon int64
	pages            []model.AnalyticsPageDaily
	lastLimit        int
}

func (f *fakePublicRepo) QueryTotals(context.Context) (int64, int64, error) {
	return f.totalPV, 0, nil
}
func (f *fakePublicRepo) QueryTotalsSegmented(context.Context) (int64, int64, int64, error) {
	return f.total, f.reg, f.anon, nil
}
func (f *fakePublicRepo) QueryTopPagesPublic(_ context.Context, _, _ string, limit int) ([]model.AnalyticsPageDaily, error) {
	f.lastLimit = limit
	return f.pages, nil
}

func TestPublicSummary_NilRedis(t *testing.T) {
	r := &fakePublicRepo{totalPV: 100, total: 40, reg: 10, anon: 30}
	rt := &fakeRealtime{today: svc.TodayStat{PV: 5, UV: 3}, online: 2}
	s := svc.NewPublicService(r, rt, nil, time.Minute, zap.NewNop())
	out, err := s.Summary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(5), out.TodayPV)
	assert.Equal(t, int64(2), out.Online)
	assert.Equal(t, int64(100), out.TotalPV)
	assert.Equal(t, int64(40), out.TotalUV)
	assert.Equal(t, int64(10), out.RegisteredUV)
	assert.Equal(t, int64(30), out.AnonymousUV)
}

func TestPublicPopular_MapsAndDelegates(t *testing.T) {
	r := &fakePublicRepo{pages: []model.AnalyticsPageDaily{{Path: "/a", Title: "A", PV: 9, UV: 4}}}
	s := svc.NewPublicService(r, &fakeRealtime{}, nil, time.Minute, zap.NewNop())
	out, err := s.Popular(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "/a", out[0].Path)
	assert.Equal(t, 9, out[0].PV)
	assert.Equal(t, 7, r.lastLimit)
}
