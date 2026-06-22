package notification_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	notificationrepo "github.com/vpt/blog-backend/internal/repository/notification"
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

// 收件箱命中唯一约束时不应重复插入，返回 created=false。
func TestCreateInbox_DuplicateDoesNotInsert(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := notificationrepo.NewRepository(db)

	// ON DUPLICATE KEY 命中：RowsAffected 为 0 代表未新建。
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO .notification_inbox.").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	created, err := repo.CreateInbox(context.Background(), &model.NotificationInbox{
		EventID:         7,
		RecipientUserID: 3,
		DeliveredAt:     time.Now(),
	})

	require.NoError(t, err)
	assert.False(t, created)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// 邮件任务命中 idempotency_key 唯一约束时应被安全忽略，返回 created=false。
func TestCreateEmailTask_DuplicateIdempotencyKeyIgnored(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := notificationrepo.NewRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO .notification_email_task.").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	created, err := repo.CreateEmailTask(context.Background(), &model.NotificationEmailTask{
		EventID:         7,
		RecipientUserID: 3,
		ToEmail:         "a@example.com",
		EventType:       "comment_created",
		Purpose:         "notification",
		IdempotencyKey:  "evt:7:user:3",
	})

	require.NoError(t, err)
	assert.False(t, created)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// 全部行租约未过期时抢占更新影响 0 行，方法应直接返回空且不回读。
func TestLeasePendingEvents_SkipsUnexpiredLease(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := notificationrepo.NewRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .notification_event. SET").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	events, err := repo.LeasePendingEvents(context.Background(), "worker-1", 300, 10)

	require.NoError(t, err)
	assert.Empty(t, events)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// 存在租约过期行时抢占更新影响 1 行，应回读并返回该事件。
func TestLeasePendingEvents_ClaimsExpiredLease(t *testing.T) {
	db, mock, sqlDB := newMockDB(t)
	defer sqlDB.Close()
	repo := notificationrepo.NewRepository(db)

	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .notification_event. SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT \\* FROM .notification_event.").
		WillReturnRows(sqlmock.NewRows([]string{"id", "dispatch_status", "next_process_at"}).
			AddRow(42, "processing", now))

	events, err := repo.LeasePendingEvents(context.Background(), "worker-1", 300, 10)

	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, uint(42), events[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// 额度预留：未达上限时条件自增成功返回 true，已达上限时影响 0 行返回 false。
func TestReserveQuota_IncrementsAndRejectsOverLimit(t *testing.T) {
	key := notificationrepo.QuotaUsageKey{
		QuotaDate:   time.Now().Truncate(24 * time.Hour),
		ScopeType:   "purpose",
		ScopeID:     0,
		Purpose:     "notification",
		WindowType:  "day",
		WindowStart: time.Now().Truncate(24 * time.Hour),
	}

	t.Run("未达上限时占用成功", func(t *testing.T) {
		db, mock, sqlDB := newMockDB(t)
		defer sqlDB.Close()
		repo := notificationrepo.NewRepository(db)

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO .email_quota_usage.").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("UPDATE .email_quota_usage. SET .used_count.").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		allowed, err := repo.ReserveQuota(context.Background(), key, 150)

		require.NoError(t, err)
		assert.True(t, allowed)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("已达上限时拒绝", func(t *testing.T) {
		db, mock, sqlDB := newMockDB(t)
		defer sqlDB.Close()
		repo := notificationrepo.NewRepository(db)

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO .email_quota_usage.").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE .email_quota_usage. SET .used_count.").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		allowed, err := repo.ReserveQuota(context.Background(), key, 150)

		require.NoError(t, err)
		assert.False(t, allowed)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
