package moderation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	repositorymock "github.com/vpt/blog-backend/internal/repository/moderation/mock"
	"github.com/vpt/blog-backend/internal/service/moderation"
	"github.com/vpt/blog-backend/pkg/config"
	"go.uber.org/mock/gomock"
)

func TestOperationsServiceUpdateControlValidatesAndReturnsNewVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	reason := "维护"
	repo.EXPECT().UpdateControl(gomock.Any(), moderationrepo.UpdateControlCommand{
		RegistrationMode: moderationrepo.RegistrationClosed,
		PublishingMode:   moderationrepo.PublishingPreReviewAll,
		Reason:           &reason, OperatorID: 1, ExpectedLockVersion: 3, ChangedAt: now,
	}).Return(nil)
	service := newOperationsService(repo, now)

	got, err := service.UpdateControl(context.Background(), moderation.UpdateControlCommand{
		RegistrationMode: moderation.RegistrationClosed,
		PublishingMode:   moderation.PublishingPreReviewAll,
		Reason:           reason, OperatorID: 1, ExpectedLockVersion: 3,
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(4), got.LockVersion)
	assert.Equal(t, reason, *got.Reason)
}

func TestOperationsServiceEmergencyHideUsesStateMachineSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	record := pendingReviewRecord()
	record.State.Pending = moderationrepo.RevisionRef{}
	record.State.Materialized = moderationrepo.ExistingRevision(record.RevisionID)
	record.State.Approved = moderationrepo.ExistingRevision(record.RevisionID)
	repo.EXPECT().LoadCurrentReviewRecord(gomock.Any(), record.ItemID).Return(record, nil)
	repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, cmd moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
			assert.Equal(t, moderationrepo.PublicEmergencyHidden, cmd.Next.PublicState)
			require.NotNil(t, cmd.Next.StateBeforeEmergency)
			assert.Equal(t, moderationrepo.PublicVisible, *cmd.Next.StateBeforeEmergency)
			assert.Equal(t, moderationrepo.EventEmergencyHide, cmd.Log.Action)
			return moderationrepo.AppliedTransition{ItemID: record.ItemID, LockVersion: record.LockVersion + 1}, nil
		})
	service := newOperationsService(repo, now)

	got, err := service.HideItem(context.Background(), moderation.EmergencyItemCommand{
		ItemID: record.ItemID, ActorID: 1, Reason: "临时下架",
	})

	require.NoError(t, err)
	assert.Equal(t, moderation.PublicEmergencyHidden, got.PublicState)
	assert.Equal(t, record.LockVersion+1, got.LockVersion)
}

func TestOperationsServiceRestoreDeletedItemIsRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	record := pendingReviewRecord()
	record.State.LifecycleState = moderationrepo.LifecycleDeleted
	record.State.PublicState = moderationrepo.PublicHidden
	record.State.Pending = moderationrepo.RevisionRef{}
	record.State.Materialized = moderationrepo.RevisionRef{}
	deletedAt := time.Now()
	record.State.DeletedAt = &deletedAt
	repo.EXPECT().LoadCurrentReviewRecord(gomock.Any(), record.ItemID).Return(record, nil)
	service := newOperationsService(repo, time.Now())

	_, err := service.RestoreItem(context.Background(), moderation.EmergencyItemCommand{ItemID: record.ItemID, ActorID: 1})

	require.ErrorIs(t, err, moderation.ErrAlreadyDeleted)
}

func TestOperationsServiceUserBatchAppliesConfiguredBounds(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	reason := "批量隔离"
	repo.EXPECT().ApplyUserEmergencyBatch(gomock.Any(), moderationrepo.UserEmergencyBatchCommand{
		UserID: 42, ActorID: 1, Cursor: 9, Limit: 20, Hide: true, Reason: &reason, Now: now,
	}).Return(moderationrepo.EmergencyBatchResult{Processed: 2, NextCursor: 11, HasMore: true}, nil)
	service := newOperationsService(repo, now)

	got, err := service.HideUserContent(context.Background(), moderation.UserEmergencyBatchCommand{
		UserID: 42, ActorID: 1, Cursor: 9, Limit: 100, Reason: reason,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, got.Processed)
	assert.True(t, got.HasMore)
}

func newOperationsService(repo moderationrepo.Repository, now time.Time) moderation.OperationsService {
	return moderation.NewOperationsService(repo, nil, config.ModerationConfig{
		Review: config.ModerationReviewConfig{ReasonMaxChars: 1000},
		Control: config.ModerationControlConfig{
			UserHideBatchSize: 20, UserHideMaxItemsPerRequest: 100,
		},
	}, func() time.Time { return now })
}
