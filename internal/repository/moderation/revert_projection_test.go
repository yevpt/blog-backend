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
	mock.ExpectExec("UPDATE `moderation_item` SET .*`public_state`=\\?.*WHERE.*id = \\?").
		WithArgs("placeholder", fixedTime, itemID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `moment_media` WHERE moment_id = \\?").
		WithArgs(momentID).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := repository.RevertPublicProjection(context.Background(), itemID, momentID)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRevertPublicProjectionRejectsZeroIDs(t *testing.T) {
	repository, _ := newRepository(t)

	assert.ErrorIs(t, repository.RevertPublicProjection(context.Background(), 0, 8), moderation.ErrInvalidCommand)
	assert.ErrorIs(t, repository.RevertPublicProjection(context.Background(), 10, 0), moderation.ErrInvalidCommand)
}
