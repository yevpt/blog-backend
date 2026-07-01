package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestListPublishRecoveryCandidatesFindsApprovedPlaceholderMoment(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	repository := moderation.NewPublishRecoveryRepository(gdb)
	previousID := uint64(19)
	mock.ExpectQuery("SELECT .*previous_revision_id.*FROM moderation_item AS item.*content_type = \\?.*lifecycle_state = \\?.*public_state = \\?.*approved_revision_id IS NOT NULL.*ORDER BY item.updated_at ASC, item.id ASC LIMIT \\?").
		WithArgs("moment", "active", "placeholder", 50).
		WillReturnRows(sqlmock.NewRows([]string{
			"item_id", "revision_id", "author_id", "moment_id", "previous_revision_id",
		}).AddRow(10, 20, 7, 88, previousID))

	rows, err := repository.ListPublishRecoveryCandidates(context.Background(), 50)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, uint64(20), rows[0].RevisionID)
	require.NotNil(t, rows[0].PreviousRevisionID)
	assert.Equal(t, previousID, *rows[0].PreviousRevisionID)
	require.NoError(t, mock.ExpectationsWereMet())
}
