package moderationemail_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	moderationemailrepo "github.com/vpt/blog-backend/internal/repository/moderationemail"
	moderationemailservice "github.com/vpt/blog-backend/internal/service/moderationemail"
	moderationemailmock "github.com/vpt/blog-backend/internal/service/moderationemail/mock"
)

var errRecipientUnavailable = errors.New("recipient unavailable")

func newPlanner(
	now time.Time,
	repo *moderationemailmock.MockRepository,
	directory *moderationemailmock.MockDirectory,
) *moderationemailservice.Planner {
	return moderationemailservice.NewPlanner(repo, directory, moderationemailservice.Config{
		RecipientUserID: 1,
		MinInterval:     30 * time.Minute,
	}, func() time.Time { return now })
}

func pendingTask(availableAt time.Time) *moderationemailrepo.PendingTask {
	return &moderationemailrepo.PendingTask{ID: 1, AvailableAt: availableAt}
}

func recipient() moderationemailrepo.AdminRecipient {
	return moderationemailrepo.AdminRecipient{UserID: 1, Email: "owner@example.com"}
}

func TestPlannerNoTasksStopsAfterStaleCleanup(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := moderationemailmock.NewMockRepository(ctrl)
	directory := moderationemailmock.NewMockDirectory(ctrl)
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	gomock.InOrder(
		repo.EXPECT().SkipStaleTasks(gomock.Any(), 20, now),
		repo.EXPECT().HasOpenBatch(gomock.Any()).Return(false, nil),
		repo.EXPECT().OldestPendingTask(gomock.Any()).Return(nil, nil),
	)

	created, err := newPlanner(now, repo, directory).PlanOnce(context.Background(), "worker-1", 20)

	require.NoError(t, err)
	assert.Zero(t, created)
}

func TestPlannerFirstTaskWaitsUntilAggregationBoundary(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name        string
		availableAt time.Time
		wantCreated int
	}{
		{name: "before 60 seconds", availableAt: now.Add(time.Nanosecond), wantCreated: 0},
		{name: "at 60 seconds", availableAt: now, wantCreated: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := moderationemailmock.NewMockRepository(ctrl)
			directory := moderationemailmock.NewMockDirectory(ctrl)
			calls := []any{
				repo.EXPECT().SkipStaleTasks(gomock.Any(), 20, now),
				repo.EXPECT().HasOpenBatch(gomock.Any()).Return(false, nil),
				repo.EXPECT().OldestPendingTask(gomock.Any()).Return(pendingTask(tt.availableAt), nil),
				repo.EXPECT().LastSuccessfulSend(gomock.Any()).Return(nil, nil),
			}
			if tt.wantCreated > 0 {
				calls = append(calls,
					directory.EXPECT().LoadAdminRecipient(gomock.Any(), uint(1)).Return(recipient(), nil),
					repo.EXPECT().CreateBatch(gomock.Any(), recipient(), 20, now).Return(tt.wantCreated, nil),
				)
			}
			gomock.InOrder(calls...)

			created, err := newPlanner(now, repo, directory).PlanOnce(context.Background(), "worker-1", 20)

			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, created)
		})
	}
}

func TestPlannerWaitsUntilCooldownBoundary(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name        string
		lastSent    time.Time
		wantCreated int
	}{
		{name: "before 30 minutes", lastSent: now.Add(-30*time.Minute + time.Nanosecond), wantCreated: 0},
		{name: "at 30 minutes", lastSent: now.Add(-30 * time.Minute), wantCreated: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := moderationemailmock.NewMockRepository(ctrl)
			directory := moderationemailmock.NewMockDirectory(ctrl)
			calls := []any{
				repo.EXPECT().SkipStaleTasks(gomock.Any(), 20, now),
				repo.EXPECT().HasOpenBatch(gomock.Any()).Return(false, nil),
				repo.EXPECT().OldestPendingTask(gomock.Any()).Return(pendingTask(now.Add(-time.Hour)), nil),
				repo.EXPECT().LastSuccessfulSend(gomock.Any()).Return(&tt.lastSent, nil),
			}
			if tt.wantCreated > 0 {
				calls = append(calls,
					directory.EXPECT().LoadAdminRecipient(gomock.Any(), uint(1)).Return(recipient(), nil),
					repo.EXPECT().CreateBatch(gomock.Any(), recipient(), 20, now).Return(tt.wantCreated, nil),
				)
			}
			gomock.InOrder(calls...)

			created, err := newPlanner(now, repo, directory).PlanOnce(context.Background(), "worker-1", 20)

			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, created)
		})
	}
}

func TestPlannerUsesLaterCooldownForNewTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := moderationemailmock.NewMockRepository(ctrl)
	directory := moderationemailmock.NewMockDirectory(ctrl)
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	lastSent := now.Add(-29*time.Minute - 50*time.Second)
	gomock.InOrder(
		repo.EXPECT().SkipStaleTasks(gomock.Any(), 20, now),
		repo.EXPECT().HasOpenBatch(gomock.Any()).Return(false, nil),
		repo.EXPECT().OldestPendingTask(gomock.Any()).Return(pendingTask(now.Add(-10*time.Second)), nil),
		repo.EXPECT().LastSuccessfulSend(gomock.Any()).Return(&lastSent, nil),
	)

	created, err := newPlanner(now, repo, directory).PlanOnce(context.Background(), "worker-1", 20)

	require.NoError(t, err)
	assert.Zero(t, created)
}

func TestPlannerStaleOnlyTasksAreRemovedBeforePlanning(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := moderationemailmock.NewMockRepository(ctrl)
	directory := moderationemailmock.NewMockDirectory(ctrl)
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	gomock.InOrder(
		repo.EXPECT().SkipStaleTasks(gomock.Any(), 20, now),
		repo.EXPECT().HasOpenBatch(gomock.Any()).Return(false, nil),
		repo.EXPECT().OldestPendingTask(gomock.Any()).Return(nil, nil),
	)

	created, err := newPlanner(now, repo, directory).PlanOnce(context.Background(), "worker-1", 20)

	require.NoError(t, err)
	assert.Zero(t, created)
}

func TestPlannerOpenBatchBlocksPlanning(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := moderationemailmock.NewMockRepository(ctrl)
	directory := moderationemailmock.NewMockDirectory(ctrl)
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	gomock.InOrder(
		repo.EXPECT().SkipStaleTasks(gomock.Any(), 20, now),
		repo.EXPECT().HasOpenBatch(gomock.Any()).Return(true, nil),
	)

	created, err := newPlanner(now, repo, directory).PlanOnce(context.Background(), "worker-1", 20)

	require.NoError(t, err)
	assert.Zero(t, created)
}

func TestPlannerInvalidRecipientBacksOffWithoutCreatingBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := moderationemailmock.NewMockRepository(ctrl)
	directory := moderationemailmock.NewMockDirectory(ctrl)
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	gomock.InOrder(
		repo.EXPECT().SkipStaleTasks(gomock.Any(), 20, now),
		repo.EXPECT().HasOpenBatch(gomock.Any()).Return(false, nil),
		repo.EXPECT().OldestPendingTask(gomock.Any()).Return(pendingTask(now), nil),
		repo.EXPECT().LastSuccessfulSend(gomock.Any()).Return(nil, nil),
		directory.EXPECT().LoadAdminRecipient(gomock.Any(), uint(1)).Return(moderationemailrepo.AdminRecipient{}, errRecipientUnavailable),
	)
	planner := newPlanner(now, repo, directory)

	created, err := planner.PlanOnce(context.Background(), "worker-1", 20)

	assert.ErrorIs(t, err, errRecipientUnavailable)
	assert.Zero(t, created)

	created, err = planner.PlanOnce(context.Background(), "worker-1", 20)

	require.NoError(t, err)
	assert.Zero(t, created)
}

func TestPlannerForwardsBatchLimitToAuthoritativeCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := moderationemailmock.NewMockRepository(ctrl)
	directory := moderationemailmock.NewMockDirectory(ctrl)
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	gomock.InOrder(
		repo.EXPECT().SkipStaleTasks(gomock.Any(), 7, now),
		repo.EXPECT().HasOpenBatch(gomock.Any()).Return(false, nil),
		repo.EXPECT().OldestPendingTask(gomock.Any()).Return(pendingTask(now), nil),
		repo.EXPECT().LastSuccessfulSend(gomock.Any()).Return(nil, nil),
		directory.EXPECT().LoadAdminRecipient(gomock.Any(), uint(1)).Return(recipient(), nil),
		repo.EXPECT().CreateBatch(gomock.Any(), recipient(), 7, now).Return(7, nil),
	)

	created, err := newPlanner(now, repo, directory).PlanOnce(context.Background(), "worker-1", 7)

	require.NoError(t, err)
	assert.Equal(t, 7, created)
}
