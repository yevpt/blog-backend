package moderation_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newCleanupRepository(t *testing.T) (moderation.CleanupRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{NowFunc: func() time.Time { return fixedTime }})
	require.NoError(t, err)
	return moderation.NewCleanupRepository(gdb), mock
}

func TestCleanupAuditDeletesOnlyExpiredUnreferencedRowsWithinBatch(t *testing.T) {
	repository, mock := newCleanupRepository(t)
	cutoff := fixedTime.AddDate(0, 0, -30)
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM moderation_attempt WHERE created_at < \\? ORDER BY id LIMIT \\?").
		WithArgs(cutoff, 50).WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectExec("DELETE FROM moderation_action_log WHERE created_at < \\? ORDER BY id LIMIT \\?").
		WithArgs(cutoff, 50).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectQuery("SELECT id FROM moderation_revision.*NOT EXISTS.*materialized_revision_id.*approved_revision_id.*pending_revision_id.*LIMIT \\?.*FOR UPDATE").
		WithArgs(cutoff, 50).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5).AddRow(6))
	mock.ExpectExec("DELETE FROM `moderation_revision_image` WHERE revision_id IN ").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("DELETE FROM `moderation_revision` WHERE id IN ").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	got, err := repository.CleanupAudit(context.Background(), moderation.AuditCleanupCommand{
		AttemptBefore: cutoff, ActionLogBefore: cutoff, RevisionBefore: cutoff, Limit: 50,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(4), got.Attempts)
	assert.Equal(t, int64(3), got.ActionLogs)
	assert.Equal(t, int64(2), got.Revisions)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListStaleImagesRequiresNoRevisionReference(t *testing.T) {
	repository, mock := newCleanupRepository(t)
	cutoff := fixedTime.AddDate(0, 0, -180)
	mock.ExpectQuery("SELECT .* FROM `moderation_image`.*NOT EXISTS.*moderation_revision_image.*ORDER BY last_used_at ASC.*LIMIT \\?").
		WithArgs(cutoff, 20).
		WillReturnRows(sqlmock.NewRows([]string{"sha256", "size", "preview_object_key", "last_used_at"}).
			AddRow("sha", 10, "moderation/previews/a.jpg", cutoff.Add(-time.Hour)))

	got, err := repository.ListStaleImages(context.Background(), cutoff, 20)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "sha", got[0].SHA256)
	require.NoError(t, mock.ExpectationsWereMet())
}
