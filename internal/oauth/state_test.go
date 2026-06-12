package oauth_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/oauth"
)

func newStateStore(t *testing.T, ttl time.Duration) (*oauth.StateStore, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})

	return oauth.NewStateStore(rdb, ttl), mr
}

func TestStateStore_CreateAndConsume(t *testing.T) {
	store, _ := newStateStore(t, 10*time.Minute)
	ctx := context.Background()

	state, err := store.Create(ctx, oauth.FlowContext{
		Source:      "github",
		Action:      oauth.ActionLogin,
		Verifier:    "pkce-verifier",
		RedirectURI: "https://front.example.com/oauth/done",
	})
	require.NoError(t, err)
	require.NotEmpty(t, state)

	flow, err := store.Consume(ctx, state)
	require.NoError(t, err)

	assert.Equal(t, state, flow.State)
	assert.Equal(t, "github", flow.Source)
	assert.Equal(t, oauth.ActionLogin, flow.Action)
	assert.Equal(t, "pkce-verifier", flow.Verifier)
	assert.Equal(t, "https://front.example.com/oauth/done", flow.RedirectURI)
	assert.False(t, flow.CreatedAt.IsZero())
}

func TestStateStore_ConsumeIsOneTime(t *testing.T) {
	store, _ := newStateStore(t, 10*time.Minute)
	ctx := context.Background()

	state, err := store.Create(ctx, oauth.FlowContext{
		Source:   "github",
		Action:   oauth.ActionLogin,
		Verifier: "pkce-verifier",
	})
	require.NoError(t, err)

	_, err = store.Consume(ctx, state)
	require.NoError(t, err)

	_, err = store.Consume(ctx, state)
	assert.ErrorIs(t, err, oauth.ErrInvalidState)
}

func TestStateStore_ConsumeExpiredState(t *testing.T) {
	store, mr := newStateStore(t, time.Minute)
	ctx := context.Background()

	state, err := store.Create(ctx, oauth.FlowContext{
		Source:   "github",
		Action:   oauth.ActionLogin,
		Verifier: "pkce-verifier",
	})
	require.NoError(t, err)

	mr.FastForward(time.Minute + time.Second)

	_, err = store.Consume(ctx, state)
	assert.ErrorIs(t, err, oauth.ErrInvalidState)
}

func TestStateStore_ConsumeMissingState(t *testing.T) {
	store, _ := newStateStore(t, 10*time.Minute)

	_, err := store.Consume(context.Background(), "missing-state")

	assert.ErrorIs(t, err, oauth.ErrInvalidState)
}
