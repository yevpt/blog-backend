package comment_test

import (
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commentrepo "github.com/vpt/blog-backend/internal/repository/comment"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newCommentMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
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

func TestCommentRepository_Create_ClosedArticleReturnsCommentClosed(t *testing.T) {
	db, mock, sqlDB := newCommentMockDB(t)
	defer sqlDB.Close()
	repo := commentrepo.NewCommentRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT `id`,`comment_status` FROM `article`").
		WithArgs(uint(3), []uint8{1, 2}, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "comment_status"}).AddRow(3, 0))

	resp, err := repo.Create(commentrepo.Target{Type: commentrepo.TargetArticle, ID: 3}, 7, "好文章")

	require.ErrorIs(t, err, commentrepo.ErrTargetCommentClosed)
	assert.Nil(t, resp)
	assert.WithinDuration(t, now, time.Now(), time.Second)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_Reply_ParentReplyBecomesToUser(t *testing.T) {
	db, mock, sqlDB := newCommentMockDB(t)
	defer sqlDB.Close()
	repo := commentrepo.NewCommentRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `article_comment`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "user_id", "content",
		}).AddRow(9, now, now, nil, 3, 8, "原评论"))
	mock.ExpectQuery("SELECT \\* FROM `comment_reply`").
		WithArgs(uint(11), uint8(commentrepo.TargetArticle), uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "comment_type", "comment_id",
			"to_user_id", "from_user_id", "parent_reply_id", "content",
		}).AddRow(11, now, now, nil, uint8(commentrepo.TargetArticle), 9, 8, 6, 0, "父回复"))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `comment_reply`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, uint8(commentrepo.TargetArticle), uint(9), uint(6), uint(7), uint(11), "收到").
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `user`").
		WithArgs(uint(7), uint(6)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "username", "password", "nickname",
			"email", "phone", "site", "avatar_url", "mark", "status", "last_login_at",
		}).
			AddRow(7, now, now, nil, "from", "", nil, nil, nil, nil, nil, nil, 1, nil).
			AddRow(6, now, now, nil, "to", "", nil, nil, nil, nil, nil, nil, 1, nil))

	resp, err := repo.Reply(commentrepo.ReplyData{
		Target:        commentrepo.Target{Type: commentrepo.TargetArticle},
		CommentID:     9,
		ParentReplyID: 11,
		FromUserID:    7,
		Content:       "收到",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint(6), resp.Reply.ToUserID)
	assert.Equal(t, uint(7), resp.Reply.FromUserID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_Create_MomentCommentCreatesMessage(t *testing.T) {
	db, mock, sqlDB := newCommentMockDB(t)
	defer sqlDB.Close()
	repo := commentrepo.NewCommentRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT `id`,`comment_status` FROM `moment`").
		WithArgs(uint(9), uint8(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "comment_status"}).AddRow(9, 1))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `moment_comment`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, uint(9), uint(7), "好碎语").
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectQuery("SELECT `id`,`user_id` FROM `moment`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).AddRow(9, 1))
	mock.ExpectExec("INSERT INTO `message`").
		WillReturnResult(sqlmock.NewResult(15, 1))
	mock.ExpectExec("INSERT INTO `user_message`").
		WillReturnResult(sqlmock.NewResult(16, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `user`").
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "username", "password", "nickname",
			"email", "phone", "site", "avatar_url", "mark", "status", "last_login_at",
		}).AddRow(7, now, now, nil, "from", "", nil, nil, nil, nil, nil, nil, 1, nil))

	resp, err := repo.Create(commentrepo.Target{Type: commentrepo.TargetMoment, ID: 9}, 7, "好碎语")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint(12), resp.Comment.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_DeleteComment_FindsCommentByTypeAndID(t *testing.T) {
	db, mock, sqlDB := newCommentMockDB(t)
	defer sqlDB.Close()
	repo := commentrepo.NewCommentRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `article_comment`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "user_id", "content",
		}).AddRow(9, now, now, nil, 3, 7, "原评论"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `comment_reply` SET `deleted_at`=\\? WHERE \\(comment_type = \\? AND comment_id = \\?\\) AND `comment_reply`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint8(commentrepo.TargetArticle), uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE `article_comment` SET `deleted_at`=\\? WHERE `article_comment`.`id` = \\? AND `article_comment`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	resp, err := repo.DeleteComment(commentrepo.Target{Type: commentrepo.TargetArticle}, 9, 7, false)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint(3), resp.TargetID)
	assert.NoError(t, mock.ExpectationsWereMet())
}
