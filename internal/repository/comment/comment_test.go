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

func TestCommentRepository_List_UsesReplyCountAndLikeState(t *testing.T) {
	db, mock, sqlDB := newCommentMockDB(t)
	defer sqlDB.Close()
	repo := commentrepo.NewCommentRepository(db)

	now := time.Now()
	viewerID := uint(9)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `article`.*status IN \\(\\?,\\?\\)").
		WithArgs(uint(50), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `article_comment`").
		WithArgs(uint(50)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `article_comment`").
		WithArgs(uint(50), 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "user_id", "content",
		}).AddRow(9, now, now, nil, 50, 7, "好文章"))
	mock.ExpectQuery("SELECT \\* FROM `user` WHERE id IN").
		WithArgs(uint(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "username", "password", "nickname",
			"email", "phone", "site", "avatar_url", "mark", "status", "last_login_at",
		}).AddRow(7, now, now, nil, "from", "", nil, nil, nil, nil, nil, nil, 1, nil))
	mock.ExpectQuery("SELECT comment_id, count\\(\\*\\) as count FROM `article_comment_reply`").
		WithArgs(uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"comment_id", "count"}).AddRow(9, 3))
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(uint8(commentrepo.ArticleCommentLikeType), uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}).AddRow(9, 4))
	mock.ExpectQuery("SELECT .*target_id.* FROM `user_like`").
		WithArgs(uint8(commentrepo.ArticleCommentLikeType), viewerID, uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id"}).AddRow(9))

	resp, err := repo.List(commentrepo.Target{Type: commentrepo.TargetArticle, ID: 50}, &viewerID, 1, 10)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Comments, 1)
	assert.Equal(t, int64(3), resp.Comments[0].ReplyCount)
	assert.Equal(t, int64(4), resp.Comments[0].LikeCount)
	assert.True(t, resp.Comments[0].IsLiked)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_ListReplies_UsesArticleReplyTable(t *testing.T) {
	db, mock, sqlDB := newCommentMockDB(t)
	defer sqlDB.Close()
	repo := commentrepo.NewCommentRepository(db)

	now := time.Now()
	viewerID := uint(9)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `article`.*status IN \\(\\?,\\?\\)").
		WithArgs(uint(50), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `article_comment`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "user_id", "content",
		}).AddRow(9, now, now, nil, 50, 8, "原评论"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `article_comment_reply`").
		WithArgs(uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `article_comment_reply`").
		WithArgs(uint(9), 5).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "comment_id", "to_user_id", "from_user_id", "parent_reply_id", "content",
		}).AddRow(12, now, now, nil, 9, 8, 7, 0, "收到"))
	mock.ExpectQuery("SELECT \\* FROM `user` WHERE id IN").
		WithArgs(uint(7), uint(8)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "username", "password", "nickname",
			"email", "phone", "site", "avatar_url", "mark", "status", "last_login_at",
		}).
			AddRow(7, now, now, nil, "from", "", nil, nil, nil, nil, nil, nil, 1, nil).
			AddRow(8, now, now, nil, "to", "", nil, nil, nil, nil, nil, nil, 1, nil))
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(uint8(commentrepo.ArticleCommentReplyLikeType), uint(12)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}).AddRow(12, 2))
	mock.ExpectQuery("SELECT .*target_id.* FROM `user_like`").
		WithArgs(uint8(commentrepo.ArticleCommentReplyLikeType), viewerID, uint(12)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id"}).AddRow(12))

	resp, err := repo.ListReplies(commentrepo.Target{Type: commentrepo.TargetArticle, ID: 50}, 9, &viewerID, 1, 5)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Replies, 1)
	assert.Equal(t, uint(12), resp.Replies[0].Reply.ID)
	assert.Equal(t, int64(2), resp.Replies[0].LikeCount)
	assert.True(t, resp.Replies[0].IsLiked)
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
	mock.ExpectQuery("SELECT \\* FROM `article_comment_reply`").
		WithArgs(uint(11), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "comment_id", "to_user_id", "from_user_id", "parent_reply_id", "content",
		}).AddRow(11, now, now, nil, 9, 8, 6, 0, "父回复"))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `article_comment_reply`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, uint(9), uint(6), uint(7), uint(11), "收到").
		WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM `user` WHERE id IN").
		WithArgs(uint(7), uint(6)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "username", "password", "nickname",
			"email", "phone", "site", "avatar_url", "mark", "status", "last_login_at",
		}).
			AddRow(7, now, now, nil, "from", "", nil, nil, nil, nil, nil, nil, 1, nil).
			AddRow(6, now, now, nil, "to", "", nil, nil, nil, nil, nil, nil, 1, nil))
	mock.ExpectQuery("SELECT target_id, count\\(\\*\\) as count FROM `user_like`").
		WithArgs(uint8(commentrepo.ArticleCommentReplyLikeType), uint(12)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id", "count"}))

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

func TestCommentRepository_ToggleLike_ReturnsLatestState(t *testing.T) {
	db, mock, sqlDB := newCommentMockDB(t)
	defer sqlDB.Close()
	repo := commentrepo.NewCommentRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `article_comment`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "user_id", "content",
		}).AddRow(9, now, now, nil, 3, 8, "原评论"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `user_like`").
		WithArgs(uint(9), uint(7), uint8(commentrepo.ArticleCommentLikeType), 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec("INSERT INTO `user_like`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, uint(7), uint(9), uint8(commentrepo.ArticleCommentLikeType)).
		WillReturnResult(sqlmock.NewResult(15, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `user_like`").
		WithArgs(uint(9), uint8(commentrepo.ArticleCommentLikeType)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	resp, err := repo.ToggleLike(commentrepo.Target{Type: commentrepo.TargetArticle}, 9, 7)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.IsLiked)
	assert.Equal(t, int64(3), resp.LikeCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentRepository_DeleteComment_DeletesArticleReplyTable(t *testing.T) {
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
	mock.ExpectExec("UPDATE `article_comment_reply` SET `deleted_at`=\\? WHERE comment_id = \\? AND `article_comment_reply`.`deleted_at` IS NULL").
		WithArgs(sqlmock.AnyArg(), uint(9)).
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
