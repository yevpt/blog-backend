package dbschema

import (
	"os"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestApplySchemaIndexesCreatesMissingUserLikeLookupIndex(t *testing.T) {
	db, mock, cleanup := newSchemaIndexTestDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(schemaIndexExistsQuery)).
		WithArgs("user_like", userLikeLookupIndexName).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta(userLikeLookupIndexSQL)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, ApplySchemaIndexes(db))
}

func TestApplySchemaIndexesSkipsExistingUserLikeLookupIndex(t *testing.T) {
	db, mock, cleanup := newSchemaIndexTestDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(schemaIndexExistsQuery)).
		WithArgs("user_like", userLikeLookupIndexName).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))

	require.NoError(t, ApplySchemaIndexes(db))
}

func TestApplySchemaIndexesReturnsLookupError(t *testing.T) {
	db, mock, cleanup := newSchemaIndexTestDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(schemaIndexExistsQuery)).
		WithArgs("user_like", userLikeLookupIndexName).
		WillReturnError(assert.AnError)

	err := ApplySchemaIndexes(db)
	require.ErrorContains(t, err, "检查索引 "+userLikeLookupIndexName)
}

func TestUserLikeLookupMigrationMatchesSchema(t *testing.T) {
	content, err := os.ReadFile("../../migrations/20260830_user_like_lookup_index.sql")
	require.NoError(t, err)

	assert.Contains(t, string(content), "ADD INDEX `"+userLikeLookupIndexName+"` ("+userLikeLookupIndexColumnsSQL+")")
}

func newSchemaIndexTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)

	cleanup := func() {
		mock.ExpectClose()
		require.NoError(t, sqlDB.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}
	return db, mock, cleanup
}
