package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestApplyTransitionCorrectsMaterializedContentAndCreatesNotificationAtomically(t *testing.T) {
	repository, mock := newRepository(t)
	pendingID := uint64(20)
	corrected := "管理员修正正文"
	reason := "移除不当表述"
	reviewerID := uint64(1)
	command := moderation.ApplyTransitionCommand{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3},
		AuthorID: 42, ExpectedLockVersion: 4, ExpectedPendingID: &pendingID,
		Next: moderation.ItemState{
			LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible,
			Materialized: moderation.ExistingRevision(20), Approved: moderation.ExistingRevision(20),
		},
		Review: &moderation.RevisionReview{
			RevisionID: 20, Status: moderation.ReviewApproved, Decision: "corrected",
			Reason: &reason, ReviewerID: &reviewerID, ReviewedAt: fixedTime,
			PublishedContent: &corrected,
		},
		Materialize: moderation.ExistingRevision(20),
		Log: &moderation.ActionLog{
			Revision: moderation.ExistingRevision(20), ActorUserID: &reviewerID,
			SubjectUserID: uint64Pointer(42), Action: moderation.EventCorrectAndApprove,
			Reason: &reason, CreatedAt: fixedTime,
		},
		Notification: &moderation.NotificationIntent{
			RecipientUserID: 42, Title: "内容经管理员修正后已发布",
			ContentExcerpt: reason, ItemID: 10, RevisionID: 20, Decision: "corrected",
		},
	}

	mock.ExpectBegin()
	expectLockedItem(mock, 4, &pendingID)
	expectLockedArticleSubject(mock, "待审正文")
	mock.ExpectQuery("SELECT .* FROM `moderation_revision`.*id IN.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id"}).AddRow(20, 10))
	mock.ExpectExec("UPDATE `moderation_revision` SET .*published_content.*review_status").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` WHERE .*id = \\?.*item_id = \\?.*LIMIT \\?").
		WithArgs(uint64(20), uint64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id", "published_content"}).AddRow(20, 10, corrected))
	mock.ExpectExec("UPDATE `article_comment` SET `content`=\\? WHERE .*id = \\?.*user_id = \\?.*article_id = \\?").
		WithArgs(corrected, uint64(7), uint64(42), uint64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `moderation_action_log`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO `notification_event`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	_, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionRejectsInvalidNotificationIntentBeforeTransaction(t *testing.T) {
	command := transitionCommand()
	command.Notification = &moderation.NotificationIntent{Title: "缺少接收人"}
	repository, mock := newRepository(t)

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrInvalidCommand)
	require.NoError(t, mock.ExpectationsWereMet())
}
