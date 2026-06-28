package article_test

import (
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	article "github.com/vpt/blog-backend/internal/repository/article"
)

func TestArticleRepository_ListRecommended(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT article\\.\\*, article_recommend.seq FROM `article` JOIN article_recommend ON article_recommend.article_id = article.id AND article_recommend.deleted_at IS NULL WHERE article.deleted_at IS NULL AND `article`.`deleted_at` IS NULL ORDER BY article_recommend.seq ASC,article.id DESC LIMIT \\?").
		WithArgs(100).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url", "mobile_cover_img_url",
			"short_content", "content", "user_id", "status", "comment_status",
			"password", "read_count", "cover_ai_generated", "content_ai_referenced", "seq",
		}).AddRow(3, now, now, nil, "A", nil, nil, "short", "body", 1, 1, 1, nil, 0, false, false, 0).
			AddRow(1, now, now, nil, "B", nil, nil, "short", "body", 1, 1, 1, nil, 0, false, false, 10))

	rows, err := repo.ListRecommended()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, uint(3), rows[0].Article.ID)
	assert.Equal(t, uint(0), rows[0].Seq)
	assert.Equal(t, uint(1), rows[1].Article.ID)
	assert.Equal(t, uint(10), rows[1].Seq)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_ReorderRecommended_Success(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	mock.ExpectQuery("SELECT article_recommend.article_id FROM `article_recommend` JOIN article ON article.id = article_recommend.article_id AND article.deleted_at IS NULL WHERE article_recommend.deleted_at IS NULL AND `article_recommend`.`deleted_at` IS NULL ORDER BY article_recommend.seq ASC,article_recommend.article_id ASC").
		WillReturnRows(sqlmock.NewRows([]string{"article_id"}).AddRow(3).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `article_recommend` SET `seq`=\\?,`updated_at`=\\? WHERE article_id = \\?").
		WithArgs(0, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `article_recommend` SET `seq`=\\?,`updated_at`=\\? WHERE article_id = \\?").
		WithArgs(10, sqlmock.AnyArg(), 3).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.ReorderRecommended([]uint{1, 3})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_ReorderRecommended_Mismatch(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := article.NewArticleRepository(db)

	mock.ExpectQuery("SELECT article_recommend.article_id FROM `article_recommend` JOIN article ON article.id = article_recommend.article_id AND article.deleted_at IS NULL WHERE article_recommend.deleted_at IS NULL AND `article_recommend`.`deleted_at` IS NULL ORDER BY article_recommend.seq ASC,article_recommend.article_id ASC").
		WillReturnRows(sqlmock.NewRows([]string{"article_id"}).AddRow(3).AddRow(1))

	err := repo.ReorderRecommended([]uint{3})
	require.ErrorIs(t, err, article.ErrRecommendOrderMismatch)
	assert.NoError(t, mock.ExpectationsWereMet())
}
