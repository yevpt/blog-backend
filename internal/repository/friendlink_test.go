package repository_test

import (
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/repository"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newFriendLinkMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
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

func friendLinkRows(id uint) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at",
		"name", "description", "email", "phone", "site", "avatar_url", "seq", "status",
	}).AddRow(id, now, now, nil, "友站", nil, nil, nil, "https://friend.example.com", nil, 2, 1)
}

func TestFriendLinkRepository_ListPublic_FiltersVisibleAndOrders(t *testing.T) {
	db, mock, sqlDB := newFriendLinkMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewFriendLinkRepository(db)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `friend_link` WHERE status = \\? AND `friend_link`.`deleted_at` IS NULL").
		WithArgs(uint8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `friend_link` WHERE status = \\? AND `friend_link`.`deleted_at` IS NULL ORDER BY seq ASC,id DESC LIMIT \\?").
		WithArgs(uint8(1), 10).
		WillReturnRows(friendLinkRows(1))

	links, total, err := repo.ListPublic(0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, links, 1)
	assert.Equal(t, "友站", links[0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFriendLinkRepository_ListAdmin_FiltersStatusWhenProvided(t *testing.T) {
	db, mock, sqlDB := newFriendLinkMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewFriendLinkRepository(db)

	status := uint8(0)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `friend_link` WHERE status = \\? AND `friend_link`.`deleted_at` IS NULL").
		WithArgs(status).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `friend_link` WHERE status = \\? AND `friend_link`.`deleted_at` IS NULL ORDER BY seq ASC,id DESC LIMIT \\? OFFSET \\?").
		WithArgs(status, 5, 10).
		WillReturnRows(friendLinkRows(1))

	links, total, err := repo.ListAdmin(10, 5, &status)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, links, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFriendLinkRepository_Update_ReturnsNilWhenMissing(t *testing.T) {
	db, mock, sqlDB := newFriendLinkMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewFriendLinkRepository(db)

	name := "新友站"
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `friend_link` WHERE `friend_link`.`id` = \\? AND `friend_link`.`deleted_at` IS NULL ORDER BY `friend_link`.`id` LIMIT \\?").
		WithArgs(uint(9), 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectRollback()

	link, err := repo.Update(9, repository.FriendLinkUpdateData{Name: &name})
	require.NoError(t, err)
	assert.Nil(t, link)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFriendLinkRepository_Delete_SoftDeletes(t *testing.T) {
	db, mock, sqlDB := newFriendLinkMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewFriendLinkRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `friend_link` WHERE `friend_link`.`id` = \\? AND `friend_link`.`deleted_at` IS NULL ORDER BY `friend_link`.`id` LIMIT \\?").
		WithArgs(uint(3), 1).
		WillReturnRows(friendLinkRows(3))
	mock.ExpectExec("UPDATE `friend_link` SET `deleted_at`=\\? WHERE `friend_link`.`id` = \\? AND `friend_link`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	link, err := repo.Delete(3)
	require.NoError(t, err)
	require.NotNil(t, link)
	assert.Equal(t, uint(3), link.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}
