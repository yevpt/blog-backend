package article_test

import (
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
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

func articleTableColumns() []string {
	return []string{
		"id", "created_at", "updated_at", "deleted_at", "title", "cover_img_url",
		"short_content", "content", "user_id", "status", "comment_status",
		"password", "read_count", "cover_ai_generated", "content_ai_referenced",
	}
}

func expectArticleAiModels(mock sqlmock.Sqlmock, articleIDs ...uint) {
	mock.ExpectQuery("SELECT \\* FROM `article_ai_model`").
		WithArgs(uintArgs(articleIDs)...).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "article_id", "scope", "model_name", "seq",
		}))
}
