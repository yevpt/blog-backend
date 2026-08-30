package migration_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/migration"
)

const (
	lockName = "blog_backend_schema_migrations"

	createLedgerSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
  version varchar(191) NOT NULL,
  checksum char(64) NOT NULL,
  dirty tinyint(1) NOT NULL DEFAULT 0,
  applied_at datetime(6) NULL,
  execution_ms bigint unsigned NOT NULL DEFAULT 0,
  PRIMARY KEY (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='数据库迁移台账'`
)

var testCatalog = []migration.Migration{
	{Version: "20260101_baseline.sql", SQL: "SELECT 1;", Checksum: "checksum-1"},
	{Version: "20260102_add_name.sql", SQL: "ALTER TABLE users ADD COLUMN name varchar(64);", Checksum: "checksum-2"},
}

func TestRunnerStatus_DoesNotCreateMissingLedger(t *testing.T) {
	runner, mock, cleanup := newRunner(t)
	defer cleanup()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM information_schema\\.tables").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	report, err := runner.Status(context.Background())

	require.NoError(t, err)
	assert.False(t, report.Initialized)
	assert.Equal(t, []migration.StatusEntry{
		{Version: testCatalog[0].Version, State: migration.StatePending},
		{Version: testCatalog[1].Version, State: migration.StatePending},
	}, report.Entries)
}

func TestRunnerStatus_ReportsDirtyMigration(t *testing.T) {
	runner, mock, cleanup := newRunner(t)
	defer cleanup()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM information_schema\\.tables").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT version, checksum, dirty FROM schema_migrations ORDER BY version").
		WillReturnRows(sqlmock.NewRows([]string{"version", "checksum", "dirty"}).
			AddRow(testCatalog[0].Version, testCatalog[0].Checksum, true))

	report, err := runner.Status(context.Background())

	require.NoError(t, err)
	assert.True(t, report.Initialized)
	assert.Equal(t, migration.StateDirty, report.Entries[0].State)
	assert.Equal(t, migration.StatePending, report.Entries[1].State)
}

func TestRunnerStatus_TreatsEmptyLedgerAsUninitialized(t *testing.T) {
	runner, mock, cleanup := newRunner(t)
	defer cleanup()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM information_schema\\.tables").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT version, checksum, dirty FROM schema_migrations ORDER BY version").
		WillReturnRows(sqlmock.NewRows([]string{"version", "checksum", "dirty"}))

	report, err := runner.Status(context.Background())

	require.NoError(t, err)
	assert.False(t, report.Initialized)
	assert.Equal(t, migration.StatePending, report.Entries[0].State)
}

func TestRunnerAdopt_RecordsCatalogPrefix(t *testing.T) {
	runner, mock, cleanup := newRunner(t)
	defer cleanup()
	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(createLedgerSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT version, checksum, dirty FROM schema_migrations ORDER BY version").
		WillReturnRows(sqlmock.NewRows([]string{"version", "checksum", "dirty"}))
	for _, item := range testCatalog {
		mock.ExpectExec("INSERT INTO schema_migrations").
			WithArgs(item.Version, item.Checksum).
			WillReturnResult(sqlmock.NewResult(1, 1))
	}
	expectUnlock(mock)

	count, err := runner.Adopt(context.Background(), testCatalog[1].Version)

	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestRunnerUp_AppliesPendingMigrationAndMarksItClean(t *testing.T) {
	runner, mock, cleanup := newRunner(t)
	defer cleanup()
	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(createLedgerSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT version, checksum, dirty FROM schema_migrations ORDER BY version").
		WillReturnRows(sqlmock.NewRows([]string{"version", "checksum", "dirty"}).
			AddRow(testCatalog[0].Version, testCatalog[0].Checksum, false))
	mock.ExpectExec("INSERT INTO schema_migrations").
		WithArgs(testCatalog[1].Version, testCatalog[1].Checksum).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(testCatalog[1].SQL)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE schema_migrations SET dirty = 0").
		WithArgs(sqlmock.AnyArg(), testCatalog[1].Version).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectUnlock(mock)

	count, err := runner.Up(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRunnerUp_RejectsUninitializedLedger(t *testing.T) {
	runner, mock, cleanup := newRunner(t)
	defer cleanup()
	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(createLedgerSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT version, checksum, dirty FROM schema_migrations ORDER BY version").
		WillReturnRows(sqlmock.NewRows([]string{"version", "checksum", "dirty"}))
	expectUnlock(mock)

	_, err := runner.Up(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, migration.ErrLedgerUninitialized)
}

func TestRunnerUp_RejectsChecksumMismatch(t *testing.T) {
	runner, mock, cleanup := newRunner(t)
	defer cleanup()
	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(createLedgerSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT version, checksum, dirty FROM schema_migrations ORDER BY version").
		WillReturnRows(sqlmock.NewRows([]string{"version", "checksum", "dirty"}).
			AddRow(testCatalog[0].Version, "changed", false))
	expectUnlock(mock)

	_, err := runner.Up(context.Background())

	require.Error(t, err)
	assert.True(t, errors.Is(err, migration.ErrChecksumMismatch))
}

func TestRunnerUp_LeavesDirtyMarkerWhenSQLFails(t *testing.T) {
	runner, mock, cleanup := newRunner(t)
	defer cleanup()
	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(createLedgerSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT version, checksum, dirty FROM schema_migrations ORDER BY version").
		WillReturnRows(sqlmock.NewRows([]string{"version", "checksum", "dirty"}).
			AddRow(testCatalog[0].Version, testCatalog[0].Checksum, false))
	mock.ExpectExec("INSERT INTO schema_migrations").
		WithArgs(testCatalog[1].Version, testCatalog[1].Checksum).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(testCatalog[1].SQL)).
		WillReturnError(errors.New("ddl failed"))
	expectUnlock(mock)

	_, err := runner.Up(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "已保留 dirty 标记")
}

func TestRunnerUp_RejectsMigrationHistoryGap(t *testing.T) {
	runner, mock, cleanup := newRunner(t)
	defer cleanup()
	expectLock(mock)
	mock.ExpectExec(regexp.QuoteMeta(createLedgerSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT version, checksum, dirty FROM schema_migrations ORDER BY version").
		WillReturnRows(sqlmock.NewRows([]string{"version", "checksum", "dirty"}).
			AddRow(testCatalog[1].Version, testCatalog[1].Checksum, false))
	expectUnlock(mock)

	_, err := runner.Up(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "顺序不连续")
}

func newRunner(t *testing.T) (*migration.Runner, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	cleanup := func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}
	return migration.NewRunner(db, testCatalog), mock, cleanup
}

func expectLock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT GET_LOCK").
		WithArgs(lockName, 30).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(1))
}

func expectUnlock(mock sqlmock.Sqlmock) {
	mock.ExpectExec("SELECT RELEASE_LOCK").
		WithArgs(lockName).
		WillReturnResult(sqlmock.NewResult(0, 0))
}
