package moderationemail_test

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/repository/moderationemail"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestSkipStaleTasksOnlySkipsBoundedNonPendingRevisions(t *testing.T) {
	repo, mock := newRepository(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec("UPDATE moderation_review_email_task AS task JOIN \\( SELECT candidate.id FROM moderation_review_email_task AS candidate JOIN moderation_revision AS revision ON revision.id = candidate.revision_id JOIN moderation_item AS item ON item.id = candidate.item_id WHERE candidate.status = \\? AND .* ORDER BY candidate.created_at, candidate.id LIMIT \\? \\) AS stale ON stale.id = task.id SET task.status = \\?, task.updated_at = \\? WHERE task.status = \\?").
		WithArgs("pending", "pending", 25, "skipped", now, "pending").
		WillReturnResult(sqlmock.NewResult(0, 3))

	err := repo.SkipStaleTasks(context.Background(), 25, now)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateBatchLocksOldestTasksAndBindsThemTransactionally(t *testing.T) {
	repo, mock := newRepository(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT task.id FROM moderation_review_email_task AS task .* ORDER BY task.created_at,task.id LIMIT \\? FOR UPDATE").
		WithArgs("pending", now, now, "pending", 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4).AddRow(7))
	mock.ExpectQuery("SELECT `id` FROM `moderation_review_email_batch` WHERE status IN \\(\\?,\\?\\) ORDER BY created_at,id LIMIT \\? FOR UPDATE").
		WithArgs("pending", "sending", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("INSERT INTO `moderation_review_email_batch`").
		WithArgs(uint(1), "owner@example.com", "待审核内容提醒（2 条）", "pending", 2, now, nil, 0, now, nil, nil, nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec("UPDATE `moderation_review_email_task` SET .* WHERE id IN \\(\\?,\\?\\) AND status = \\?").
		WithArgs(uint64(9), "batched", sqlmock.AnyArg(), uint64(4), uint64(7), "pending").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	count, err := repo.CreateBatch(context.Background(), moderationemail.AdminRecipient{UserID: 1, Email: "owner@example.com"}, 2, now)

	require.NoError(t, err)
	assert.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHasOpenBatchUsesDeterministicBoundedQuery(t *testing.T) {
	repo, mock := newRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id` FROM `moderation_review_email_batch` WHERE status IN (?,?) ORDER BY created_at,id LIMIT ?")).
		WithArgs("pending", "sending", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))

	open, err := repo.HasOpenBatch(context.Background())

	require.NoError(t, err)
	assert.True(t, open)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateBatchDoesNotCreateSecondOpenBatch(t *testing.T) {
	repo, mock := newRepository(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*moderation_review_email_task.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))
	mock.ExpectQuery("SELECT `id` FROM `moderation_review_email_batch` WHERE status IN \\(\\?,\\?\\) ORDER BY created_at,id LIMIT \\? FOR UPDATE").
		WithArgs("pending", "sending", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(5))
	mock.ExpectCommit()

	count, err := repo.CreateBatch(context.Background(), moderationemail.AdminRecipient{UserID: 1, Email: "owner@example.com"}, 10, now)

	require.NoError(t, err)
	assert.Zero(t, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOldestPendingTaskLoadsStableModerationSnapshot(t *testing.T) {
	repo, mock := newRepository(t)
	createdAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT task.id,task.revision_id,task.item_id,item.content_type,item.author_id,revision.submitted_content,task.available_at,task.created_at FROM moderation_review_email_task AS task .* ORDER BY task.created_at,task.id LIMIT \\?").
		WithArgs("pending", "pending", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revision_id", "item_id", "content_type", "author_id", "submitted_content", "available_at", "created_at"}).
			AddRow(3, 4, 5, "moment", 6, "待审核正文", createdAt, createdAt))

	task, err := repo.OldestPendingTask(context.Background())

	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, uint64(4), task.RevisionID)
	assert.Equal(t, "待审核正文", task.SubmittedContent)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLastSuccessfulSendUsesNewestSentBatch(t *testing.T) {
	repo, mock := newRepository(t)
	sentAt := time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT `sent_at` FROM `moderation_review_email_batch` WHERE status = \\? AND sent_at IS NOT NULL ORDER BY sent_at DESC,id DESC LIMIT \\?").
		WithArgs("sent", 1).
		WillReturnRows(sqlmock.NewRows([]string{"sent_at"}).AddRow(sentAt))

	got, err := repo.LastSuccessfulSend(context.Background())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, sentAt, *got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLeaseBatchesRecoversExpiredSendingLease(t *testing.T) {
	repo, mock := newRepository(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	leaseUntil := now.Add(time.Minute)
	mock.ExpectExec("UPDATE `moderation_review_email_batch` SET .* WHERE next_attempt_at <= \\? AND \\(status = \\? OR \\(status = \\? AND lease_until < \\?\\)\\) ORDER BY created_at,id LIMIT \\?").
		WithArgs(leaseUntil, "worker-a", "sending", sqlmock.AnyArg(), now, "pending", "sending", now, 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `moderation_review_email_batch` WHERE locked_by = \\? AND lease_until = \\? AND status = \\? ORDER BY created_at,id LIMIT \\?").
		WithArgs("worker-a", leaseUntil, "sending", 2).
		WillReturnRows(sqlmock.NewRows(batchColumns()).AddRow(batchValues(8, leaseUntil)...))

	batches, err := repo.LeaseBatches(context.Background(), "worker-a", time.Minute, 2, now)

	require.NoError(t, err)
	require.Len(t, batches, 1)
	assert.Equal(t, uint64(8), batches[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkBatchSentUpdatesBatchAndTasksAtomically(t *testing.T) {
	repo, mock := newRepository(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `moderation_review_email_batch` SET .* WHERE id = \\? AND status = \\?").
		WithArgs(nil, nil, "message-9", now, "sent", sqlmock.AnyArg(), uint64(9), "sending").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `moderation_review_email_task` SET .* WHERE batch_id = \\? AND status = \\?").
		WithArgs("sent", sqlmock.AnyArg(), uint64(9), "batched").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := repo.MarkBatchSent(context.Background(), 9, "message-9", now)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadBatchTasksIsBoundedAndOrdered(t *testing.T) {
	repo, mock := newRepository(t)
	createdAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT task.id,task.revision_id,task.item_id,item.content_type,item.author_id,revision.submitted_content,task.available_at,task.created_at FROM moderation_review_email_task AS task .* WHERE task.batch_id = \\? AND task.status = \\? ORDER BY task.created_at,task.id LIMIT \\?").
		WithArgs(uint64(9), "batched", 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revision_id", "item_id", "content_type", "author_id", "submitted_content", "available_at", "created_at"}).
			AddRow(3, 4, 5, "moment", 6, "正文", createdAt, createdAt))

	tasks, err := repo.LoadBatchTasks(context.Background(), 9, 50)

	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, uint64(3), tasks[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkBatchRetryPersistsStableMessageIDAndReleasesLease(t *testing.T) {
	repo, mock := newRepository(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	next := now.Add(time.Minute)
	mock.ExpectExec("UPDATE `moderation_review_email_batch` SET .* WHERE id = \\? AND status = \\?").
		WithArgs("smtp unavailable", nil, nil, "message-9", next, "pending", now, uint64(9), "sending").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.MarkBatchRetry(context.Background(), 9, "message-9", next, "smtp unavailable", now)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadAdminRecipientRequiresActiveVerifiedAdmin(t *testing.T) {
	repo, mock := newRepository(t)
	mock.ExpectQuery("SELECT user.id AS user_id,user.email FROM `user` JOIN user_role ON user_role.user_id = user.id JOIN role ON role.id = user_role.role_id WHERE user.id = \\? AND user.status = \\? AND \\(user.email IS NOT NULL AND user.email <> ''\\) AND user.email_verified_at IS NOT NULL AND role.name = \\? ORDER BY user.id LIMIT \\?").
		WithArgs(uint(1), 1, "admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email"}))

	_, err := repo.LoadAdminRecipient(context.Background(), 1)

	require.ErrorIs(t, err, moderationemail.ErrRecipientUnavailable)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadAdminRecipientReturnsConfiguredAdminOnly(t *testing.T) {
	repo, mock := newRepository(t)
	mock.ExpectQuery("SELECT user.id AS user_id,user.email FROM `user` JOIN user_role ON user_role.user_id = user.id JOIN role ON role.id = user_role.role_id WHERE user.id = \\? AND user.status = \\? AND \\(user.email IS NOT NULL AND user.email <> ''\\) AND user.email_verified_at IS NOT NULL AND role.name = \\? ORDER BY user.id LIMIT \\?").
		WithArgs(uint(12), 1, "admin", 1).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "email"}).AddRow(12, "admin@example.com"))

	recipient, err := repo.LoadAdminRecipient(context.Background(), 12)

	require.NoError(t, err)
	assert.Equal(t, moderationemail.AdminRecipient{UserID: 12, Email: "admin@example.com"}, recipient)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateBatchRollsBackWhenBindingIsIncomplete(t *testing.T) {
	repo, mock := newRepository(t)
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*moderation_review_email_task.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4).AddRow(7))
	mock.ExpectQuery("SELECT `id` FROM `moderation_review_email_batch` WHERE status IN").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("INSERT INTO `moderation_review_email_batch`").WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec("UPDATE `moderation_review_email_task`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	_, err := repo.CreateBatch(context.Background(), moderationemail.AdminRecipient{UserID: 1, Email: "owner@example.com"}, 2, now)

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newRepository(t *testing.T) (moderationemail.Repository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)
	return moderationemail.NewRepository(gdb), mock
}

func batchColumns() []string {
	return []string{"id", "recipient_user_id", "to_email", "subject", "status", "item_count", "scheduled_at", "sent_at", "attempts", "next_attempt_at", "lease_until", "locked_by", "message_id", "last_error", "created_at", "updated_at"}
}

func batchValues(id uint64, leaseUntil time.Time) []driver.Value {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	return []driver.Value{id, 1, "owner@example.com", "待审核内容提醒（2 条）", "sending", 2, now, nil, 0, now, leaseUntil, "worker-a", nil, nil, now, now}
}
