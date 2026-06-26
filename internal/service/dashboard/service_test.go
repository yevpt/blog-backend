package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	repo "github.com/vpt/blog-backend/internal/repository/dashboard"
	svc "github.com/vpt/blog-backend/internal/service/dashboard"
)

type fakeRepo struct {
	counts    repo.Counts
	gotSince  time.Time
	gotDayStr time.Time
}

func (f *fakeRepo) Summary(_ context.Context, since, dayStart time.Time) (repo.Counts, error) {
	f.gotSince = since
	f.gotDayStr = dayStart
	return f.counts, nil
}

func TestOverviewMapsCountsAndDayBoundary(t *testing.T) {
	fr := &fakeRepo{counts: repo.Counts{
		Articles: 128, Categories: 12, Tags: 46, Music: 32, FriendLinks: 18,
		NewComments: 5, NewGuestbook: 3, NewMoments: 2,
		UsersTotal: 1204, UsersTodayNew: 8, UsersTodayActive: 96,
	}}
	tz, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	got, err := svc.NewService(fr, tz).Overview(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(128), got.Content.Articles)
	assert.Equal(t, int64(18), got.Content.FriendLinks)
	assert.Equal(t, int64(5), got.Interactions.NewComments)
	assert.Equal(t, int64(96), got.Users.TodayActive)

	// dayStart 为 Asia/Shanghai 当日零点；since 为 7 天前。
	assert.Equal(t, 0, fr.gotDayStr.Hour())
	assert.Equal(t, "Asia/Shanghai", fr.gotDayStr.Location().String())
	assert.WithinDuration(t, time.Now().AddDate(0, 0, -7), fr.gotSince, time.Minute)
}
