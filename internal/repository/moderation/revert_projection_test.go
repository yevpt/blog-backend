package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestRevertPublicProjectionSetsPlaceholderAndClearsMomentMedia(t *testing.T) {
	repository, mock := newRepository(t)
	const itemID, momentID = 10, 8

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*WHERE.*id = \\?.*LIMIT.*FOR UPDATE").
		WithArgs(itemID, 1).WillReturnRows(revertItemRow(itemID, momentID, "visible"))
	mock.ExpectExec("UPDATE `moderation_item` SET .*`public_state`=\\?.*WHERE.*id = \\?.*public_state IN").
		WithArgs("placeholder", fixedTime, itemID, "visible", "placeholder").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `moment_media` WHERE moment_id = \\?").
		WithArgs(momentID).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := repository.RevertPublicProjection(context.Background(), itemID, momentID)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRevertPublicProjectionPreservesEmergencyHiddenState(t *testing.T) {
	repository, mock := newRepository(t)
	const itemID, momentID = 10, 8

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*WHERE.*id = \\?.*LIMIT.*FOR UPDATE").
		WithArgs(itemID, 1).WillReturnRows(revertItemRow(itemID, momentID, "emergency_hidden"))
	mock.ExpectCommit()

	require.NoError(t, repository.RevertPublicProjection(context.Background(), itemID, momentID))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRevertPublicProjectionRejectsZeroIDs(t *testing.T) {
	repository, _ := newRepository(t)

	assert.ErrorIs(t, repository.RevertPublicProjection(context.Background(), 0, 8), moderation.ErrInvalidCommand)
	assert.ErrorIs(t, repository.RevertPublicProjection(context.Background(), 10, 0), moderation.ErrInvalidCommand)
}

func revertItemRow(itemID, momentID uint64, publicState string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
		"materialized_revision_id", "approved_revision_id", "pending_revision_id",
		"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
		"lock_version", "created_at", "updated_at",
	}).AddRow(itemID, "moment", momentID, 42, "active", publicState,
		101, 101, nil, nil, nil, nil, nil, uint64(2), fixedTime, fixedTime)
}
