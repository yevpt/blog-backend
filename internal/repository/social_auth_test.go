package repository_test

import (
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"github.com/vpt/blog-backend/pkg/roles"
)

func TestSocialAuthRepository_FindSocialUser_Found(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewSocialAuthRepository(db)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"uuid", "source", "access_token", "refresh_token", "open_id", "is_active",
	}).AddRow(11, now, now, nil, "remote-123", "github", "access-token", nil, nil, true)
	mock.ExpectQuery(`SELECT \* FROM \x60social_user\x60`).
		WithArgs("github", "remote-123", 1).
		WillReturnRows(rows)

	socialUser, err := repo.FindSocialUser("github", "remote-123")

	require.NoError(t, err)
	require.NotNil(t, socialUser)
	assert.Equal(t, uint(11), socialUser.ID)
	assert.Equal(t, "github", socialUser.Source)
	assert.Equal(t, "remote-123", socialUser.UUID)
}

func TestSocialAuthRepository_FindSocialUser_NotFound(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewSocialAuthRepository(db)

	mock.ExpectQuery(`SELECT \* FROM \x60social_user\x60`).
		WithArgs("github", "missing", 1).
		WillReturnRows(sqlmock.NewRows(nil))

	socialUser, err := repo.FindSocialUser("github", "missing")

	require.NoError(t, err)
	assert.Nil(t, socialUser)
}

func TestSocialAuthRepository_CreateUserWithSocialAuth(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewSocialAuthRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO \x60user\x60`).
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectExec(`INSERT INTO \x60user_role\x60`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO \x60social_user\x60`).
		WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectExec(`INSERT INTO \x60social_user_auth\x60`).
		WillReturnResult(sqlmock.NewResult(21, 1))
	mock.ExpectCommit()

	nickname := "Octo"
	user := &model.User{
		Username: "github_remote-123",
		Password: "hashed",
		Nickname: &nickname,
		Status:   1,
	}
	socialUser := &model.SocialUser{
		UUID:        "remote-123",
		Source:      "github",
		AccessToken: "access-token",
		IsActive:    true,
	}

	err := repo.CreateUserWithSocialAuth(user, roles.NormalRoleId, socialUser)

	require.NoError(t, err)
	assert.Equal(t, uint(7), user.ID)
	assert.Equal(t, uint(11), socialUser.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSocialAuthRepository_FindBindingByUserAndSource(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewSocialAuthRepository(db)

	rows := sqlmock.NewRows([]string{"social_user_auth_id", "social_user_id", "source"}).
		AddRow(21, 11, "github")
	mock.ExpectQuery(`SELECT .+ FROM \x60social_user_auth\x60`).
		WithArgs(uint(7), "github", 1).
		WillReturnRows(rows)

	binding, err := repo.FindBindingByUserAndSource(7, "github")

	require.NoError(t, err)
	require.NotNil(t, binding)
	assert.Equal(t, uint(21), binding.AuthID)
	assert.Equal(t, uint(11), binding.SocialUserID)
	assert.Equal(t, "github", binding.Source)
}

func TestSocialAuthRepository_Unbind(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewSocialAuthRepository(db)

	rows := sqlmock.NewRows([]string{"social_user_auth_id", "social_user_id", "source"}).
		AddRow(21, 11, "github")
	mock.ExpectQuery(`SELECT .+ FROM \x60social_user_auth\x60`).
		WithArgs(uint(7), "github", 1).
		WillReturnRows(rows)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE \x60social_user_auth\x60 SET \x60deleted_at\x60=`).
		WithArgs(sqlmock.AnyArg(), uint(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.Unbind(7, "github")

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
