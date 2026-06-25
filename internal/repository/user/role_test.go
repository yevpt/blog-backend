package user_test

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/repository/user"
	"github.com/vpt/blog-backend/pkg/roles"
)

func TestUserRepository_GrantVipRole_InsertsWhenMissing(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectQuery(`SELECT .+ FROM \x60user_role\x60`).
		WithArgs(uint(7), roles.VipRoleId, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "role_id"}))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO \x60user_role\x60`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.GrantVipRole(7)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_GrantVipRole_IdempotentWhenExists(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectQuery(`SELECT .+ FROM \x60user_role\x60`).
		WithArgs(uint(7), roles.VipRoleId, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "role_id"}).AddRow(1, 7, roles.VipRoleId))

	err := repo.GrantVipRole(7)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_RevokeVipRole_DeletesRole(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM \x60user_role\x60`).
		WithArgs(uint(7), roles.VipRoleId).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.RevokeVipRole(7)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUserRepository_RevokeVipRole_IdempotentWhenMissing(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := user.NewUserRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM \x60user_role\x60`).
		WithArgs(uint(7), roles.VipRoleId).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.RevokeVipRole(7)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
