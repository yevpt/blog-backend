package user_test

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/repository/user"
)

func TestUserRepository_BatchFetchActiveLogin_ReturnsTimesKeyedByID(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	activeAt := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	loginAt := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT .+ FROM \x60user\x60 WHERE id IN`).
		WithArgs(uint(1), uint(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "last_active_at", "last_login_at"}).
			AddRow(1, activeAt, loginAt).
			AddRow(2, nil, nil))

	result, err := repo.BatchFetchActiveLogin([]uint{1, 2})
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.NotNil(t, result[1].LastActiveAt)
	require.NotNil(t, result[1].LastLoginAt)
	assert.True(t, activeAt.Equal(*result[1].LastActiveAt))
	assert.True(t, loginAt.Equal(*result[1].LastLoginAt))
	assert.Nil(t, result[2].LastActiveAt)
	assert.Nil(t, result[2].LastLoginAt)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_BatchFetchActiveLogin_UnknownIDAbsentFromResult(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectQuery(`SELECT .+ FROM \x60user\x60 WHERE id IN`).
		WithArgs(uint(99)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "last_active_at", "last_login_at"}))

	result, err := repo.BatchFetchActiveLogin([]uint{99})
	require.NoError(t, err)
	assert.Empty(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_BatchFetchActiveLogin_EmptyIDsSkipsQuery(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	result, err := repo.BatchFetchActiveLogin(nil)
	require.NoError(t, err)
	assert.Empty(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}
