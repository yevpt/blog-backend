package repository_test

import (
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	return gormDB, mock, sqlDB
}

func TestUserRepository_FindByIdentifier_Found(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewUserRepository(db)

	email := "test@example.com"
	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"username", "password", "nickname", "email", "phone",
		"site", "avatar_url", "mark", "status", "last_login_at",
	}).AddRow(1, nil, nil, nil, email, "hashed", nil, email, nil, nil, nil, nil, 1, nil)

	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs(email, email, email, 1).
		WillReturnRows(rows)

	user, err := repo.FindByIdentifier(email)
	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, uint(1), user.ID)
}

func TestUserRepository_FindByIdentifier_NotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewUserRepository(db)

	mock.ExpectQuery(`SELECT \* FROM \x60user\x60`).
		WithArgs("noone", "noone", "noone", 1).
		WillReturnRows(sqlmock.NewRows(nil))

	user, err := repo.FindByIdentifier("noone")
	require.NoError(t, err)
	assert.Nil(t, user)
}

func TestUserRepository_ExistsByEmail_True(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewUserRepository(db)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery(`SELECT count\(\*\) FROM \x60user\x60`).
		WithArgs("taken@example.com").
		WillReturnRows(rows)

	exists, err := repo.ExistsByEmail("taken@example.com")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestUserRepository_FindRolesByUserID(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewUserRepository(db)

	rows := sqlmock.NewRows([]string{"name"}).AddRow("ROLE_NORMAL")
	mock.ExpectQuery(`SELECT .+ FROM \x60user_role\x60`).
		WithArgs(uint(1)).
		WillReturnRows(rows)

	roles, err := repo.FindRolesByUserID(1)
	require.NoError(t, err)
	assert.Equal(t, []string{"ROLE_NORMAL"}, roles)
}

func TestUserRepository_Create_Success(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewUserRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO \x60user\x60`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO \x60user_role\x60`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	nickname := "alice"
	email := "alice@example.com"
	user := &model.User{
		Username: email,
		Password: "hashed",
		Nickname: &nickname,
		Email:    &email,
		Status:   1,
	}
	err := repo.Create(user, 3)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
