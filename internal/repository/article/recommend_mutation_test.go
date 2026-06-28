package article_test

import (
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
	article "github.com/vpt/blog-backend/internal/repository/article"
)

func TestArticleRepository_Save_NewRecommend_AppendsSeq(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	expectSaveArticleRelationWrites(mock, 7)
	mock.ExpectQuery("SELECT \\* FROM `article_recommend` WHERE article_id = \\? AND `article_recommend`.`deleted_at` IS NULL ORDER BY `article_recommend`.`id` LIMIT \\?").
		WithArgs(uint(7), 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `article_recommend` JOIN article ON article.id = article_recommend.article_id AND article.deleted_at IS NULL WHERE article_recommend.deleted_at IS NULL AND `article_recommend`.`deleted_at` IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(article_recommend.seq\\), 0\\) FROM `article_recommend` JOIN article ON article.id = article_recommend.article_id AND article.deleted_at IS NULL WHERE article_recommend.deleted_at IS NULL AND `article_recommend`.`deleted_at` IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{"COALESCE(MAX(article_recommend.seq), 0)"}).AddRow(20))
	mock.ExpectQuery("SELECT \\* FROM `article_recommend` WHERE article_id = \\? ORDER BY `article_recommend`.`id` LIMIT \\?").
		WithArgs(uint(7), 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec("INSERT INTO `article_recommend`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	expectFindAdminDetailAfterSave(mock, 7, now)

	result, err := repo.Save(article.ArticleSaveData{
		Article: model.Article{
			Base:          model.Base{ID: 7},
			Title:         "A",
			Content:       "body",
			UserID:        1,
			Status:        1,
			CommentStatus: 1,
		},
		CategoryIDs: []uint{3},
		Recommend:   true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_Save_ExistingRecommend_PreservesSeq(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	expectSaveArticleRelationWrites(mock, 7)
	mock.ExpectQuery("SELECT \\* FROM `article_recommend` WHERE article_id = \\? AND `article_recommend`.`deleted_at` IS NULL ORDER BY `article_recommend`.`id` LIMIT \\?").
		WithArgs(uint(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "article_id", "seq",
		}).AddRow(1, now, now, nil, 7, 30))
	mock.ExpectCommit()
	expectFindAdminDetailAfterSave(mock, 7, now)

	result, err := repo.Save(article.ArticleSaveData{
		Article: model.Article{
			Base:          model.Base{ID: 7},
			Title:         "A",
			Content:       "body",
			UserID:        1,
			Status:        1,
			CommentStatus: 1,
		},
		CategoryIDs: []uint{3},
		Recommend:   true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func expectSaveArticleRelationWrites(mock sqlmock.Sqlmock, articleID uint) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `id` FROM `article`").
		WithArgs(articleID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(articleID))
	mock.ExpectExec("UPDATE `article` SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_category` WHERE article_id = \\?").
		WithArgs(articleID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `article_category`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `article_tag` WHERE article_id = \\?").
		WithArgs(articleID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `article_music` WHERE article_id = \\?").
		WithArgs(articleID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `article_ai_model` WHERE article_id = \\?").
		WithArgs(articleID).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func expectFindAdminDetailAfterSave(mock sqlmock.Sqlmock, articleID uint, now time.Time) {
	mock.ExpectQuery("SELECT \\* FROM `article`").
		WithArgs(articleID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced",
		}).AddRow(articleID, now, now, nil, "A", nil, nil, nil, "body", 1, 1, 1, nil, 0, false, false))
	expectEmptyArticleAggregateQueries(mock, articleID, 1)
}
