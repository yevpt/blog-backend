package analytics_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	repo "github.com/vpt/blog-backend/internal/repository/analytics"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newRepo(t *testing.T) (repo.Repository, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	return repo.NewRepository(gdb), mock
}

func TestInsertEvents(t *testing.T) {
	r, mock := newRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `analytics_events`")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := r.InsertEvents(context.Background(), []model.AnalyticsEvent{{EventType: "page_view", VisitorID: "v"}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertDaily(t *testing.T) {
	r, mock := newRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `analytics_daily`")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := r.UpsertDaily(context.Background(), model.AnalyticsDaily{Date: "2026-06-24", PV: 10, UV: 5})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertSession_IncrementsPVCount(t *testing.T) {
	r, mock := newRepo(t)
	mock.ExpectBegin()
	// 冲突分支必须自增 pv_count（而非覆盖）。正则匹配渲染后的 ON DUPLICATE KEY UPDATE。
	mock.ExpectExec("ON DUPLICATE KEY UPDATE.*pv_count.*pv_count \\+ 1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := r.UpsertSession(context.Background(), model.AnalyticsSession{
		SessionID: "s1", VisitorID: "v1", PVCount: 1, IsBounce: true,
		FirstSeen: time.Unix(1000, 0), LastSeen: time.Unix(1100, 0),
		EntryPath: "/a", ExitPath: "/b",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
