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

func newTagMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
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

func TestTagRepository_Delete_ClearsRelationsAndSoftDeletesTag(t *testing.T) {
	db, mock, sqlDB := newTagMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewTagRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `tag`").
		WithArgs(uint(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name",
			"url", "icon", "description", "cover_img_url", "seq",
		}).AddRow(3, now, now, nil, "Go", nil, nil, nil, nil, 0))
	mock.ExpectExec("DELETE FROM `article_tag` WHERE tag_id = \\?").
		WithArgs(uint(3)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE `tag` SET `deleted_at`=\\? WHERE `tag`.`id` = \\? AND `tag`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tag, err := repo.Delete(3)
	require.NoError(t, err)
	require.NotNil(t, tag)
	assert.Equal(t, uint(3), tag.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTagRepository_AddArticles_SkipsExistingRelations(t *testing.T) {
	db, mock, sqlDB := newTagMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewTagRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `tag`").
		WithArgs(uint(5), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name",
			"url", "icon", "description", "cover_img_url", "seq",
		}).AddRow(5, now, now, nil, "Go", nil, nil, nil, nil, 0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `article`").
		WithArgs(uint(8), uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec("INSERT INTO `article_tag`").
		WithArgs(uint(8), uint(5), uint(9), uint(5)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	affected, err := repo.AddArticles(5, []uint{8, 9})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTagRepository_AddArticles_MissingArticleRollsBack(t *testing.T) {
	db, mock, sqlDB := newTagMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewTagRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `tag`").
		WithArgs(uint(5), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name",
			"url", "icon", "description", "cover_img_url", "seq",
		}).AddRow(5, now, now, nil, "Go", nil, nil, nil, nil, 0))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `article`").
		WithArgs(uint(8), uint(99)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	affected, err := repo.AddArticles(5, []uint{8, 99})
	require.ErrorIs(t, err, repository.ErrTagArticleMissing)
	assert.Equal(t, int64(0), affected)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTagRepository_RemoveArticles_DeletesOnlyTagRelations(t *testing.T) {
	db, mock, sqlDB := newTagMockDB(t)
	defer sqlDB.Close()
	repo := repository.NewTagRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `tag`").
		WithArgs(uint(5), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name",
			"url", "icon", "description", "cover_img_url", "seq",
		}).AddRow(5, now, now, nil, "Go", nil, nil, nil, nil, 0))
	mock.ExpectExec("DELETE FROM `article_tag` WHERE tag_id = \\? AND article_id IN \\(\\?,\\?\\)").
		WithArgs(uint(5), uint(8), uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	affected, err := repo.RemoveArticles(5, []uint{8, 9})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
	assert.NoError(t, mock.ExpectationsWereMet())
}
