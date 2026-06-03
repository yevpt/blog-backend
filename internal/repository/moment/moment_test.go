package moment_test

import (
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMomentMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
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

func TestMomentRepository_List_LoadsUsersImagesLikesAndComments(t *testing.T) {
	db, mock, sqlDB := newMomentMockDB(t)
	defer sqlDB.Close()
	repo := momentrepo.NewMomentRepository(db)

	now := time.Now()
	viewerID := uint(7)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `moment`").
		WithArgs(uint8(1), uint(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `moment`").
		WithArgs(uint8(1), uint(1), 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "user_id", "content", "status",
			"comment_status", "read_count", "is_top",
		}).AddRow(9, now, now, nil, 1, "风很温柔", 1, 1, 3, true))
	mock.ExpectQuery("SELECT \\* FROM `user`").
		WithArgs(uint(1)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "username", "password", "nickname",
			"email", "phone", "site", "avatar_url", "mark", "status", "last_login_at",
		}).AddRow(1, now, now, nil, "vpt", "", nil, nil, nil, nil, nil, nil, 1, nil))
	mock.ExpectQuery("SELECT \\* FROM `media`").
		WithArgs(momentrepo.MomentMediaOwnerType, momentrepo.MomentImageType, uint8(1), uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "uploader_id", "owner_id", "owner_type",
			"type", "file_type", "name", "url", "size", "status", "seq", "read_count",
		}).AddRow(3, now, now, nil, 1, 9, 2, 0, "jpg", "cat.jpg", "moments/cat.jpg", 10, 1, 1, 0))
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(momentrepo.MomentLikeType, uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}).AddRow(9, 2))
	mock.ExpectQuery("SELECT moment_id, count\\(\\*\\) as count FROM `moment_comment`").
		WithArgs(uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"moment_id", "count"}).AddRow(9, 4))
	mock.ExpectQuery("SELECT `target_id` FROM `user_like`").
		WithArgs(momentrepo.MomentLikeType, viewerID, uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id"}).AddRow(9))

	resp, err := repo.List(momentrepo.ListFilter{UserID: ptrUint(1), Page: 1, PageSize: 10}, &viewerID)

	require.NoError(t, err)
	require.Len(t, resp.Moments, 1)
	assert.Equal(t, int64(1), resp.Total)
	assert.Equal(t, int64(2), resp.Moments[0].LikeCount)
	assert.Equal(t, int64(4), resp.Moments[0].CommentCount)
	assert.True(t, resp.Moments[0].IsLiked)
	assert.Len(t, resp.Moments[0].Images, 1)
	assert.Equal(t, "vpt", resp.Moments[0].User.Username)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMomentRepository_Save_ReplacesImages(t *testing.T) {
	db, mock, sqlDB := newMomentMockDB(t)
	defer sqlDB.Close()
	repo := momentrepo.NewMomentRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user`").
		WithArgs(uint(1), uint8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `moment`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, uint(1), "风很温柔", uint8(1), uint8(1), uint(0), false).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec("UPDATE `media` SET `deleted_at`=\\? WHERE \\(owner_id = \\? AND owner_type = \\?\\) AND `media`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(9), momentrepo.MomentMediaOwnerType).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `media`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, uint(1), uint(9), momentrepo.MomentMediaOwnerType, momentrepo.MomentImageType, "jpg", "cat.jpg", "moments/cat.jpg", uint(10), uint8(1), uint(1), uint(0)).
		WillReturnResult(sqlmock.NewResult(3, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `moment`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "user_id", "content", "status",
			"comment_status", "read_count", "is_top",
		}).AddRow(9, now, now, nil, 1, "风很温柔", 1, 1, 0, false))
	expectEmptyRelations(mock, now, uint(9), uint(1))

	resp, err := repo.Save(momentrepo.SaveData{
		Moment:     model.Moment{UserID: 1, Content: "风很温柔", Status: 1, CommentStatus: 1},
		Images:     []model.Media{{UploaderID: 1, Name: "cat.jpg", FileType: "jpg", URL: "moments/cat.jpg", Size: 10, Seq: 1}},
		OperatorID: 1,
	})

	require.NoError(t, err)
	assert.Equal(t, uint(9), resp.Moment.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMomentRepository_SetTop_RejectsFourthTop(t *testing.T) {
	db, mock, sqlDB := newMomentMockDB(t)
	defer sqlDB.Close()
	repo := momentrepo.NewMomentRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `moment`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "user_id", "content", "status",
			"comment_status", "read_count", "is_top",
		}).AddRow(9, now, now, nil, 1, "风", 1, 1, 0, false))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `moment`").
		WithArgs(uint(1), true, uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectRollback()

	_, err := repo.SetTop(9, 1, false)

	require.ErrorIs(t, err, momentrepo.ErrTopLimitExceeded)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMomentRepository_ToggleLike_CreatesLikeAndMessage(t *testing.T) {
	db, mock, sqlDB := newMomentMockDB(t)
	defer sqlDB.Close()
	repo := momentrepo.NewMomentRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `moment`").
		WithArgs(uint8(1), uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "user_id", "content", "status",
			"comment_status", "read_count", "is_top",
		}).AddRow(9, now, now, nil, 1, "风", 1, 1, 0, false))
	mock.ExpectQuery("SELECT \\* FROM `user_like`").
		WithArgs(uint(9), uint(7), momentrepo.MomentLikeType, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec("INSERT INTO `user_like`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, uint(7), uint(9), momentrepo.MomentLikeType).
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectExec("INSERT INTO `message`").
		WillReturnResult(sqlmock.NewResult(15, 1))
	mock.ExpectExec("INSERT INTO `user_message`").
		WillReturnResult(sqlmock.NewResult(16, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `moment`").
		WithArgs(uint(9), uint8(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "user_id", "content", "status",
			"comment_status", "read_count", "is_top",
		}).AddRow(9, now, now, nil, 1, "风", 1, 1, 0, false))
	expectEmptyRelations(mock, now, uint(9), uint(1))
	mock.ExpectQuery("SELECT `target_id` FROM `user_like`").
		WithArgs(momentrepo.MomentLikeType, uint(7), uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id"}).AddRow(9))

	resp, liked, err := repo.ToggleLike(9, 7)

	require.NoError(t, err)
	assert.True(t, liked)
	assert.True(t, resp.IsLiked)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func ptrUint(v uint) *uint {
	return &v
}

func expectEmptyRelations(mock sqlmock.Sqlmock, now time.Time, momentID uint, userID uint) {
	mock.ExpectQuery("SELECT \\* FROM `user`").
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "username", "password", "nickname",
			"email", "phone", "site", "avatar_url", "mark", "status", "last_login_at",
		}).AddRow(userID, now, now, nil, "vpt", "", nil, nil, nil, nil, nil, nil, 1, nil))
	mock.ExpectQuery("SELECT \\* FROM `media`").
		WithArgs(momentrepo.MomentMediaOwnerType, momentrepo.MomentImageType, uint8(1), momentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "uploader_id", "owner_id", "owner_type",
			"type", "file_type", "name", "url", "size", "status", "seq", "read_count",
		}))
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(momentrepo.MomentLikeType, momentID).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}))
	mock.ExpectQuery("SELECT moment_id, count\\(\\*\\) as count FROM `moment_comment`").
		WithArgs(momentID).
		WillReturnRows(sqlmock.NewRows([]string{"moment_id", "count"}))
}
