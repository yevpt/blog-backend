package guestbook_test

import (
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	guestbookrepo "github.com/vpt/blog-backend/internal/repository/guestbook"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newGuestbookMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	return gormDB, mock, sqlDB
}

func TestGuestbookRepository_List_LoadsMessagesUsersAndLikes(t *testing.T) {
	db, mock, sqlDB := newGuestbookMockDB(t)
	defer sqlDB.Close()
	repo := guestbookrepo.NewGuestbookRepository(db)

	now := time.Now()
	viewerID := uint(7)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user`").
		WithArgs(uint(1), uint8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `guestbook`").
		WithArgs(uint(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `guestbook`").
		WithArgs(uint(1), 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "owner_user_id", "from_user_id", "content",
		}).AddRow(9, now, now, nil, 1, 8, "你好"))
	mock.ExpectQuery("SELECT \\* FROM `user`").
		WithArgs(uint(8)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "username", "password", "nickname",
			"email", "phone", "site", "avatar_url", "mark", "status", "last_login_at",
		}).AddRow(8, now, now, nil, "alice", "", nil, nil, nil, nil, nil, nil, 1, nil))
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(guestbookrepo.LikeType, uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}).AddRow(9, 2))
	mock.ExpectQuery("SELECT `target_id` FROM `user_like`").
		WithArgs(guestbookrepo.LikeType, viewerID, uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id"}).AddRow(9))

	resp, err := repo.List(1, &viewerID, 1, 10)

	require.NoError(t, err)
	require.Len(t, resp.Messages, 1)
	assert.Equal(t, int64(1), resp.Total)
	assert.Equal(t, int64(2), resp.Messages[0].LikeCount)
	assert.True(t, resp.Messages[0].IsLiked)
	assert.Equal(t, "alice", resp.Messages[0].User.Username)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGuestbookRepository_ToggleLike_CreatesLike(t *testing.T) {
	db, mock, sqlDB := newGuestbookMockDB(t)
	defer sqlDB.Close()
	repo := guestbookrepo.NewGuestbookRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `guestbook`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "owner_user_id", "from_user_id", "content",
		}).AddRow(9, now, now, nil, 1, 8, "你好"))
	mock.ExpectQuery("SELECT \\* FROM `user_like`").
		WithArgs(uint(9), uint(7), guestbookrepo.LikeType, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec("INSERT INTO `user_like`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, uint(7), uint(9), guestbookrepo.LikeType).
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_like`").
		WithArgs(uint(9), guestbookrepo.LikeType).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	resp, err := repo.ToggleLike(9, 7)

	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.ID)
	assert.True(t, resp.IsLiked)
	assert.Equal(t, int64(1), resp.LikeCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGuestbookRepository_Delete_AllowsOwnerUser(t *testing.T) {
	db, mock, sqlDB := newGuestbookMockDB(t)
	defer sqlDB.Close()
	repo := guestbookrepo.NewGuestbookRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `guestbook`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "owner_user_id", "from_user_id", "content",
		}).AddRow(9, now, now, nil, 7, 8, "你好"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `guestbook` SET `deleted_at`=\\? WHERE `guestbook`.`id` = \\? AND `guestbook`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	resp, err := repo.Delete(9, 7, false)

	require.NoError(t, err)
	assert.Equal(t, uint(7), resp.OwnerUserID)
	assert.NoError(t, mock.ExpectationsWereMet())
}
