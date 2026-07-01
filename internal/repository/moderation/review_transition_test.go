package moderation_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestApplyReviewTransitionPersistsNotificationsAtomically(t *testing.T) {
	repository, mock := newRepository(t)
	command := correctedReviewTransitionCommand()

	mock.ExpectBegin()
	expectCorrectedReviewWrites(mock)
	mock.ExpectExec("INSERT INTO `notification_event`").WillReturnResult(sqlmock.NewResult(1, 1))
	expectInteractionNotificationInsert(mock).WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	_, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyReviewTransitionRollsBackWhenInteractionNotificationInsertFails(t *testing.T) {
	repository, mock := newRepository(t)
	command := correctedReviewTransitionCommand()

	mock.ExpectBegin()
	expectCorrectedReviewWrites(mock)
	mock.ExpectExec("INSERT INTO `notification_event`").WillReturnResult(sqlmock.NewResult(1, 1))
	expectInteractionNotificationInsert(mock).WillReturnError(errors.New("interaction notification write failed"))
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	require.ErrorContains(t, err, "interaction notification write failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyReviewTransitionReplayDoesNotInsertNotification(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()
	command.InteractionNotification = correctedReviewTransitionCommand().InteractionNotification
	mock.ExpectBegin()
	expectIdempotencyProfileLock(mock, 42)
	mock.ExpectQuery("SELECT .*moderation_revision.*UNION ALL.*moderation_attempt").
		WithArgs(uint64(42), "request-1", uint64(42), "request-1").
		WillReturnRows(storedResultRows().AddRow("revision", 77, 10, "article_comment", 7, "pending", "visible", fixedTime))
	mock.ExpectCommit()

	got, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	require.NotNil(t, got.Replay)
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

func correctedReviewTransitionCommand() moderation.ApplyTransitionCommand {
	pendingID := uint64(20)
	corrected := "管理员修正正文"
	reason := "移除不当表述"
	reviewerID := uint64(1)
	commentID := uint64(7)
	return moderation.ApplyTransitionCommand{
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
		InteractionNotification: &moderation.InteractionNotificationIntent{
			Type: "comment_created", ActorUserID: 42, RecipientUserID: 99,
			SourceType: "article_comment", RootType: "article", RootID: 3,
			ContentExcerpt: corrected, CommentID: &commentID,
			RootSnapshot:  &moderation.NotificationSnapshot{Type: "article", ID: 3, Title: "文章标题"},
			QuoteSnapshot: &moderation.NotificationSnapshot{Type: "comment", ID: 7, Excerpt: "引用内容"},
		},
	}
}

func expectCorrectedReviewWrites(mock sqlmock.Sqlmock) {
	pendingID := uint64(20)
	reviewerID := uint64(1)
	expectLockedItem(mock, 4, &pendingID)
	expectLockedArticleSubject(mock, "待审正文")
	mock.ExpectQuery("SELECT .* FROM `moderation_revision`.*id IN.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id"}).AddRow(20, 10))
	mock.ExpectExec("UPDATE `moderation_revision` SET .*published_content.*review_status").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE moderation_image AS image_record JOIN moderation_revision_image AS revision_image").
		WithArgs(moderation.ImageApproved, fixedTime, reviewerID, fixedTime, fixedTime, uint64(20)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` WHERE .*id = \\?.*item_id = \\?.*LIMIT \\?").
		WithArgs(uint64(20), uint64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id", "published_content"}).AddRow(20, 10, "管理员修正正文"))
	mock.ExpectExec("UPDATE `article_comment` SET `content`=\\? WHERE .*id = \\?.*user_id = \\?.*article_id = \\?").
		WithArgs("管理员修正正文", uint64(7), uint64(42), uint64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `moderation_action_log`").WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectInteractionNotificationInsert(mock sqlmock.Sqlmock) *sqlmock.ExpectedExec {
	metadata := jsonArgument{expected: map[string]any{
		"recipient_user_ids": []any{float64(99)},
		"comment_id":         float64(7),
		"root_snapshot": map[string]any{
			"type": "article", "id": float64(3), "title": "文章标题",
		},
		"quote_snapshot": map[string]any{
			"type": "comment", "id": float64(7), "excerpt": "引用内容",
		},
	}}
	return mock.ExpectExec("INSERT INTO `notification_event`").WithArgs(
		fixedTime, fixedTime, nil, "comment_created", uint(42), "article_comment", uint(7),
		"article", uint(3), "", "管理员修正正文", metadata, "pending", 0, fixedTime,
		nil, nil, nil,
	)
}

type jsonArgument struct {
	expected any
}

func (a jsonArgument) Match(value driver.Value) bool {
	encoded, ok := value.(string)
	if !ok {
		return false
	}
	var actual any
	if json.Unmarshal([]byte(encoded), &actual) != nil {
		return false
	}
	return assert.ObjectsAreEqual(a.expected, actual)
}
