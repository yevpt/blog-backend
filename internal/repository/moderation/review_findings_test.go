package moderation_test

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestLoadItemStateReturnsTransitionSnapshotAndLockVersion(t *testing.T) {
	repository, mock := newRepository(t)
	approved := uint64(90)
	mock.ExpectQuery("SELECT .* FROM `moderation_item` WHERE content_type = \\? AND content_id = \\?.*LIMIT \\?").
		WithArgs(moderation.SubjectArticleComment, uint64(7), 1).
		WillReturnRows(activeItemRows(4, nil))

	got, err := repository.LoadItemState(context.Background(), moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3})

	require.NoError(t, err)
	assert.Equal(t, uint64(10), got.ItemID)
	assert.Equal(t, uint64(4), got.LockVersion)
	assert.Equal(t, moderation.LifecycleActive, got.State.LifecycleState)
	assert.Equal(t, moderation.PublicVisible, got.State.PublicState)
	assert.Equal(t, moderation.ExistingRevision(approved), got.State.Approved)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFirstCreateRequiresExplicitCreateSubject(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()
	command.Subject.ID = 0
	command.ExpectedLockVersion = 0
	command.CreateSubject = false

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrInvalidCommand)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExistingReplyRequiresExplicitParentRelation(t *testing.T) {
	repository, mock := newRepository(t)
	command := moderation.ApplyTransitionCommand{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectArticleCommentReply, ID: 9, RootID: 7, ParentID: nil},
		AuthorID: 42, ExpectedLockVersion: 4,
		Next: moderation.ItemState{LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible},
	}

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrInvalidCommand)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNoopMaterializationAcceptsZeroRowsAfterSubjectLock(t *testing.T) {
	repository, mock := newRepository(t)
	command := moderation.ApplyTransitionCommand{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3},
		AuthorID: 42, ExpectedLockVersion: 4,
		Next: moderation.ItemState{
			LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible,
			Materialized: moderation.ExistingRevision(90), Approved: moderation.ExistingRevision(90),
		},
		Materialize: moderation.ExistingRevision(90),
	}
	mock.ExpectBegin()
	expectLockedItem(mock, 4, nil)
	mock.ExpectQuery("SELECT .* FROM `article_comment`.*FOR UPDATE").
		WithArgs(uint64(7), uint64(42), uint64(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "article_id", "user_id", "content"}).AddRow(7, 3, 42, "相同正文"))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id"}).AddRow(90, 10))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` WHERE .*id = \\?.*item_id = \\?.*LIMIT \\?").
		WithArgs(uint64(90), uint64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id", "published_content"}).AddRow(90, 10, "相同正文"))
	mock.ExpectExec("UPDATE `article_comment` SET ").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	_, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExistingTransitionFailsWhenSubjectLockFindsNoRow(t *testing.T) {
	repository, mock := newRepository(t)
	command := moderation.ApplyTransitionCommand{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectGuestbook, ID: 7, RootID: 3},
		AuthorID: 42, ExpectedLockVersion: 4,
		Next: moderation.ItemState{LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible},
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*FOR UPDATE").
		WithArgs(moderation.SubjectGuestbook, uint64(7), 1).
		WillReturnRows(activeItemRowsFor("guestbook", 7, 4, nil))
	mock.ExpectQuery("SELECT .* FROM `guestbook`.*FOR UPDATE").
		WithArgs(uint64(7), uint64(42), uint64(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_user_id", "from_user_id", "content"}))
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrSubjectNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommentAndMomentTransitionsFailWhenSubjectLockFindsNoRow(t *testing.T) {
	tests := []struct {
		name        string
		ref         moderation.SubjectRef
		subjectType string
		table       string
		lockArgs    []driver.Value
	}{
		{name: "comment", ref: moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3}, subjectType: "article_comment", table: "article_comment", lockArgs: []driver.Value{uint64(7), uint64(42), uint64(3), 1}},
		{name: "moment", ref: moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 7}, subjectType: "moment", table: "moment", lockArgs: []driver.Value{uint64(7), uint64(42), 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository, mock := newRepository(t)
			command := moderation.ApplyTransitionCommand{
				Subject: tt.ref, AuthorID: 42, ExpectedLockVersion: 4,
				Next: moderation.ItemState{LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible},
			}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .* FROM `moderation_item`.*FOR UPDATE").
				WithArgs(tt.ref.Type, uint64(7), 1).
				WillReturnRows(activeItemRowsFor(tt.subjectType, 7, 4, nil))
			mock.ExpectQuery("SELECT .* FROM `" + tt.table + "`.*FOR UPDATE").
				WithArgs(tt.lockArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectRollback()

			_, err := repository.ApplyTransition(context.Background(), command)

			assert.ErrorIs(t, err, moderation.ErrSubjectNotFound)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestGuestbookNoopMaterializationAcceptsZeroRowsAfterSubjectLock(t *testing.T) {
	repository, mock := newRepository(t)
	command := moderation.ApplyTransitionCommand{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectGuestbook, ID: 7, RootID: 3},
		AuthorID: 42, ExpectedLockVersion: 4,
		Next: moderation.ItemState{
			LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible,
			Materialized: moderation.ExistingRevision(90), Approved: moderation.ExistingRevision(90),
		},
		Materialize: moderation.ExistingRevision(90),
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*FOR UPDATE").
		WithArgs(moderation.SubjectGuestbook, uint64(7), 1).
		WillReturnRows(activeItemRowsFor("guestbook", 7, 4, nil))
	mock.ExpectQuery("SELECT .* FROM `guestbook`.*FOR UPDATE").
		WithArgs(uint64(7), uint64(42), uint64(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_user_id", "from_user_id", "content"}).AddRow(7, 3, 42, "相同正文"))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id"}).AddRow(90, 10))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` WHERE .*LIMIT \\?").
		WithArgs(uint64(90), uint64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id", "published_content"}).AddRow(90, 10, "相同正文"))
	mock.ExpectExec("UPDATE `guestbook` SET ").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	_, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMomentNoopMaterializationAcceptsZeroRowsAfterSubjectLock(t *testing.T) {
	repository, mock := newRepository(t)
	command := moderation.ApplyTransitionCommand{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 7},
		AuthorID: 42, ExpectedLockVersion: 4,
		Next: moderation.ItemState{
			LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible,
			Materialized: moderation.ExistingRevision(90), Approved: moderation.ExistingRevision(90),
		},
		Materialize: moderation.ExistingRevision(90),
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*FOR UPDATE").
		WithArgs(moderation.SubjectMoment, uint64(7), 1).
		WillReturnRows(activeItemRowsFor("moment", 7, 4, nil))
	mock.ExpectQuery("SELECT .* FROM `moment`.*FOR UPDATE").
		WithArgs(uint64(7), uint64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "content"}).AddRow(7, 42, "相同正文"))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id"}).AddRow(90, 10))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` WHERE .*LIMIT \\?").
		WithArgs(uint64(90), uint64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id", "published_content"}).AddRow(90, 10, "相同正文"))
	mock.ExpectExec("UPDATE `moment` SET ").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	_, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionRejectsCrossItemRevisionPointersBeforeUpdate(t *testing.T) {
	repository, mock := newRepository(t)
	command := moderation.ApplyTransitionCommand{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3},
		AuthorID: 42, ExpectedLockVersion: 4,
		Next: moderation.ItemState{
			LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible,
			Materialized: moderation.ExistingRevision(999), Approved: moderation.ExistingRevision(90),
		},
		Materialize: moderation.ExistingRevision(999),
	}
	mock.ExpectBegin()
	expectLockedItem(mock, 4, nil)
	expectLockedArticleSubject(mock, "正文")
	mock.ExpectQuery("SELECT .* FROM `moderation_revision`.*id IN.*ORDER BY id ASC.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id"}).AddRow(90, 10).AddRow(999, 99))
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrRevisionStateConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionRejectsSupersedeThatIsNotCurrentPending(t *testing.T) {
	repository, mock := newRepository(t)
	pending := uint64(90)
	wrong := uint64(89)
	command := moderation.ApplyTransitionCommand{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3},
		AuthorID: 42, ExpectedLockVersion: 4, ExpectedPendingID: &pending,
		Next:                moderation.ItemState{LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible},
		SupersedeRevisionID: &wrong,
	}
	mock.ExpectBegin()
	expectLockedItem(mock, 4, &pending)
	expectLockedArticleSubject(mock, "正文")
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrPendingRevisionConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionRejectsReviewThatIsNotCurrentPending(t *testing.T) {
	repository, mock := newRepository(t)
	pending := uint64(90)
	command := moderation.ApplyTransitionCommand{
		Subject: moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3},
		AuthorID: 42, ExpectedLockVersion: 4, ExpectedPendingID: &pending,
		Next: moderation.ItemState{LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible},
		Review: &moderation.RevisionReview{RevisionID: 89, Status: moderation.ReviewApproved, ReviewedAt: fixedTime},
	}
	mock.ExpectBegin()
	expectLockedItem(mock, 4, &pending)
	expectLockedArticleSubject(mock, "正文")
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrPendingRevisionConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionLocksProfileAndReturnsSameDomainReplayBeforeItemLock(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()
	mock.ExpectBegin()
	expectIdempotencyProfileLock(mock, 42)
	mock.ExpectQuery("SELECT .*moderation_revision.*UNION ALL.*moderation_attempt").
		WithArgs(uint64(42), "request-1", uint64(42), "request-1").
		WillReturnRows(storedResultRows().AddRow("revision", 77, 10, "article_comment", 7, "pending", "visible", fixedTime))
	mock.ExpectCommit()

	got, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	assert.Equal(t, uint64(77), got.RevisionID)
	assert.Equal(t, uint64(10), got.ItemID)
	assert.Equal(t, uint64(7), got.Subject.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionRejectsExistingBlockedKeyBeforeItemLock(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()
	mock.ExpectBegin()
	expectIdempotencyProfileLock(mock, 42)
	mock.ExpectQuery("SELECT .*moderation_revision.*UNION ALL.*moderation_attempt").
		WithArgs(uint64(42), "request-1", uint64(42), "request-1").
		WillReturnRows(storedResultRows().AddRow("attempt", 88, nil, "article_comment", nil, "blocked", "", fixedTime))
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrIdempotencyDomainConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecordBlockedAttemptLocksProfileChecksBothDomainsThenWrites(t *testing.T) {
	repository, mock := newRepository(t)
	attempt := moderation.BlockedAttempt{
		UserID: 42, SubjectType: moderation.SubjectMoment, IdempotencyKey: "blocked-2",
		RulesetVersion: 3, RuleMatchIDs: []uint64{7}, CreatedAt: fixedTime,
	}
	mock.ExpectBegin()
	expectIdempotencyProfileLock(mock, 42)
	mock.ExpectQuery("SELECT .*moderation_revision.*UNION ALL.*moderation_attempt").
		WithArgs(uint64(42), "blocked-2", uint64(42), "blocked-2").
		WillReturnRows(storedResultRows())
	mock.ExpectExec("INSERT INTO `moderation_attempt`").WillReturnResult(sqlmock.NewResult(33, 1))
	mock.ExpectQuery("SELECT .* FROM `moderation_attempt` WHERE user_id = \\? AND idempotency_key = \\?").
		WithArgs(uint64(42), "blocked-2", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "content_type", "item_id", "idempotency_key", "ruleset_version", "rule_match_ids", "created_at"}).
			AddRow(33, 42, "moment", nil, "blocked-2", 3, "[7]", fixedTime))
	mock.ExpectCommit()

	got, err := repository.RecordBlockedAttempt(context.Background(), attempt)

	require.NoError(t, err)
	assert.Equal(t, uint64(33), got.AttemptID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecordBlockedAttemptReturnsExistingAttemptWithoutSecondWrite(t *testing.T) {
	repository, mock := newRepository(t)
	attempt := moderation.BlockedAttempt{UserID: 42, SubjectType: moderation.SubjectMoment, IdempotencyKey: "blocked-replay", CreatedAt: fixedTime}
	mock.ExpectBegin()
	expectIdempotencyProfileLock(mock, 42)
	mock.ExpectQuery("SELECT .*moderation_revision.*UNION ALL.*moderation_attempt").
		WithArgs(uint64(42), "blocked-replay", uint64(42), "blocked-replay").
		WillReturnRows(storedResultRows().AddRow("attempt", 88, nil, "moment", nil, "blocked", "", fixedTime))
	mock.ExpectCommit()

	got, err := repository.RecordBlockedAttempt(context.Background(), attempt)

	require.NoError(t, err)
	assert.Equal(t, uint64(88), got.AttemptID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProfileChangeLocksProfileBeforeItem(t *testing.T) {
	repository, mock := newRepository(t)
	command := moderation.ApplyTransitionCommand{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3},
		AuthorID: 42, ExpectedLockVersion: 4,
		Next:          moderation.ItemState{LifecycleState: moderation.LifecycleActive, PublicState: moderation.PublicVisible},
		ProfileChange: &moderation.ProfileChange{UserID: 42, CleanApprovalDelta: 1, UpdatedAt: fixedTime},
	}
	mock.ExpectBegin()
	expectIdempotencyProfileLock(mock, 42)
	expectLockedItem(mock, 4, nil)
	expectLockedArticleSubject(mock, "正文")
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `user_moderation_profile` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	_, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectIdempotencyProfileLock(mock sqlmock.Sqlmock, userID uint64) {
	mock.ExpectExec("INSERT INTO `user_moderation_profile` .*ON DUPLICATE KEY UPDATE `user_id`=`user_id`").
		WithArgs(userID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT `user_id` FROM `user_moderation_profile` WHERE user_id = \\?.*FOR UPDATE").
		WithArgs(userID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(userID))
}

func expectNoIdempotencyResult(mock sqlmock.Sqlmock, userID uint64, key string) {
	expectIdempotencyProfileLock(mock, userID)
	mock.ExpectQuery("SELECT .*moderation_revision.*UNION ALL.*moderation_attempt").
		WithArgs(userID, key, userID, key).
		WillReturnRows(storedResultRows())
}

func storedResultRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"domain", "record_id", "item_id", "content_type", "content_id", "review_status", "public_state", "created_at"})
}

func activeItemRowsFor(subjectType string, contentID, lockVersion uint64, pendingID *uint64) *sqlmock.Rows {
	var pending any
	if pendingID != nil {
		pending = *pendingID
	}
	return sqlmock.NewRows([]string{
		"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
		"materialized_revision_id", "approved_revision_id", "pending_revision_id",
		"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
		"lock_version", "created_at", "updated_at",
	}).AddRow(10, subjectType, contentID, 42, "active", "visible", 90, 90, pending, nil, nil, nil, nil, lockVersion, fixedTime, fixedTime)
}

func expectLockedArticleSubject(mock sqlmock.Sqlmock, content string) {
	mock.ExpectQuery("SELECT .* FROM `article_comment`.*FOR UPDATE").
		WithArgs(uint64(7), uint64(42), uint64(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "article_id", "user_id", "content"}).AddRow(7, 3, 42, content))
}
