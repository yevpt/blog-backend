package adminlog_test

import (
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/vpt/blog-backend/internal/model"
	adminlog "github.com/vpt/blog-backend/internal/repository/adminlog"
)

func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	return gormDB, mock, sqlDB
}

func TestRepository_Create(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := adminlog.NewRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `admin_operation_log`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.Create(context.Background(), &model.AdminOperationLog{
		OperatorID: 1, TargetUserID: 7, Action: "grant_vip",
	})
	require.NoError(t, err)
}

func TestRepository_ListByTargetUser(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := adminlog.NewRepository(db)

	mock.ExpectQuery("SELECT count").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT \\* FROM `admin_operation_log`").
		WithArgs(uint(7), 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "operator_id", "target_user_id", "action", "detail", "created_at",
		}).AddRow(1, 1, 7, "grant_vip", nil, nil))

	items, total, err := repo.ListByTargetUser(context.Background(), 7, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, items, 1)
}
