package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestLoadControlReturnsSingleton(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT .* FROM `moderation_control` WHERE id = \\?").
		WithArgs(uint64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "registration_mode", "publishing_mode", "reason", "operator_id", "changed_at", "lock_version",
		}).AddRow(1, "closed", "pre_review_all", "maintenance", 1, fixedTime, 3))

	got, err := repository.LoadControl(context.Background())

	require.NoError(t, err)
	assert.Equal(t, moderation.RegistrationClosed, got.RegistrationMode)
	assert.Equal(t, moderation.PublishingPreReviewAll, got.PublishingMode)
	assert.Equal(t, uint64(3), got.LockVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateControlUsesOptimisticLock(t *testing.T) {
	repository, mock := newRepository(t)
	reason := "maintenance"
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `moderation_control` SET .*`lock_version`=lock_version \\+ 1.*WHERE .*lock_version = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repository.UpdateControl(context.Background(), moderation.UpdateControlCommand{
		RegistrationMode: moderation.RegistrationClosed,
		PublishingMode:   moderation.PublishingPreReviewAll,
		Reason:           &reason, OperatorID: 1, ExpectedLockVersion: 3, ChangedAt: fixedTime,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyUserEmergencyBatchIsBoundedAndReportsCursor(t *testing.T) {
	repository, mock := newRepository(t)
	reason := "spam cleanup"
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM `moderation_item` WHERE .*author_id = \\?.*id > \\?.*public_state = \\?.*ORDER BY id ASC.*LIMIT \\?.*FOR UPDATE").
		WithArgs(uint64(42), uint64(5), moderation.LifecycleActive, moderation.PublicVisible, 3).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10).AddRow(11).AddRow(12))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO `moderation_action_log`").WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	got, err := repository.ApplyUserEmergencyBatch(context.Background(), moderation.UserEmergencyBatchCommand{
		UserID: 42, ActorID: 1, Cursor: 5, Limit: 2, Hide: true, Reason: &reason, Now: fixedTime,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, got.Processed)
	assert.Equal(t, uint64(11), got.NextCursor)
	assert.True(t, got.HasMore)
	require.NoError(t, mock.ExpectationsWereMet())
}
