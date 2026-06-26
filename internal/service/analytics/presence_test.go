package analytics_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

type stubActiveRepo struct {
	updates int
}

func (s *stubActiveRepo) UpdateLastActiveAt(uint) error {
	s.updates++
	return nil
}

func newPresenceTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()}), mr
}

func TestUserPresence_TouchAndIsOnline(t *testing.T) {
	rdb, mr := newPresenceTestRedis(t)
	ctx := context.Background()
	p := svc.NewUserPresence(rdb, &stubActiveRepo{}, nil, 90*time.Second)

	require.NoError(t, p.TouchUserOnline(ctx, 42))
	online, err := p.IsUserOnline(ctx, 42)
	require.NoError(t, err)
	assert.True(t, online)

	// 将 score 设为 91s 前，模拟窗口外
	oldScore := float64(time.Now().Add(-91 * time.Second).Unix())
	_, err = mr.ZAdd(svc.UserOnlineKey, oldScore, "42")
	require.NoError(t, err)
	online, err = p.IsUserOnline(ctx, 42)
	require.NoError(t, err)
	assert.False(t, online)
}

func TestUserPresence_BatchIsUserOnline(t *testing.T) {
	rdb, _ := newPresenceTestRedis(t)
	ctx := context.Background()
	p := svc.NewUserPresence(rdb, &stubActiveRepo{}, nil, 90*time.Second)

	require.NoError(t, p.TouchUserOnline(ctx, 1))
	require.NoError(t, p.TouchUserOnline(ctx, 3))

	m, err := p.BatchIsUserOnline(ctx, []uint{1, 2, 3})
	require.NoError(t, err)
	assert.True(t, m[1])
	assert.False(t, m[2])
	assert.True(t, m[3])
}

func TestUserPresence_TouchActiveAt_Throttled(t *testing.T) {
	rdb, _ := newPresenceTestRedis(t)
	ctx := context.Background()
	repo := &stubActiveRepo{}
	p := svc.NewUserPresence(rdb, repo, nil, 90*time.Second)

	require.NoError(t, p.TouchActiveAt(ctx, 5))
	require.NoError(t, p.TouchActiveAt(ctx, 5))
	assert.Equal(t, 1, repo.updates)

	// 节流 key 过期后可再次写库
	lockKey := "user:presence:db_touch:5"
	rdb.Del(ctx, lockKey)
	require.NoError(t, p.TouchActiveAt(ctx, 5))
	assert.Equal(t, 2, repo.updates)
}

func TestUserPresence_BatchIsUserOnline_Empty(t *testing.T) {
	rdb, _ := newPresenceTestRedis(t)
	m, err := svc.NewUserPresence(rdb, &stubActiveRepo{}, nil, 90*time.Second).
		BatchIsUserOnline(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, m)
}

func TestUserPresence_TouchUserOnline_MemberFormat(t *testing.T) {
	rdb, mr := newPresenceTestRedis(t)
	ctx := context.Background()
	require.NoError(t, svc.NewUserPresence(rdb, &stubActiveRepo{}, nil, 90*time.Second).
		TouchUserOnline(ctx, 7))

	score, err := mr.ZScore(svc.UserOnlineKey, strconv.Itoa(7))
	require.NoError(t, err)
	assert.Greater(t, score, float64(0))
}
