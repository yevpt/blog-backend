package analytics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestBackfill_InclusiveLoop(t *testing.T) {
	var got []string
	s := svc.NewBackfillService(func(_ context.Context, date string) error {
		got = append(got, date)
		return nil
	})

	days, err := s.Backfill(context.Background(), "2026-06-01", "2026-06-03")

	require.NoError(t, err)
	assert.Equal(t, 3, days)
	require.Len(t, got, 3)
	assert.Equal(t, "2026-06-01", got[0])
	assert.Equal(t, "2026-06-03", got[2])
}

func TestBackfill_StopsOnError(t *testing.T) {
	calls := 0
	s := svc.NewBackfillService(func(_ context.Context, _ string) error {
		calls++
		if calls == 2 {
			return errors.New("boom")
		}
		return nil
	})

	days, err := s.Backfill(context.Background(), "2026-06-01", "2026-06-03")

	require.Error(t, err)
	assert.Equal(t, 1, days)
	assert.Equal(t, 2, calls)
}

func TestBackfill_RejectsInvalidRange(t *testing.T) {
	s := svc.NewBackfillService(func(context.Context, string) error {
		t.Fatal("invalid range must not call rollup")
		return nil
	})

	days, err := s.Backfill(context.Background(), "2026-06-03", "2026-06-01")

	require.Error(t, err)
	assert.Equal(t, 0, days)
}

func TestBackfill_RejectsRangeTooLarge(t *testing.T) {
	s := svc.NewBackfillService(func(context.Context, string) error {
		t.Fatal("oversized range must not call rollup")
		return nil
	})

	days, err := s.Backfill(context.Background(), "2026-01-01", "2026-04-03")

	require.Error(t, err)
	assert.Equal(t, 0, days)
}

func TestBackfill_RejectsNilRollup(t *testing.T) {
	s := svc.NewBackfillService(nil)

	days, err := s.Backfill(context.Background(), "2026-06-01", "2026-06-01")

	require.Error(t, err)
	assert.Equal(t, 0, days)
}
