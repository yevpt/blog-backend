package moderation_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var fixedTime = time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)

func newRepository(t *testing.T) (moderation.Repository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{NowFunc: func() time.Time { return fixedTime }})
	require.NoError(t, err)
	return moderation.NewRepository(gdb), mock
}

func activeItemRows(lockVersion uint64, pendingID *uint64) *sqlmock.Rows {
	var pending any
	if pendingID != nil {
		pending = *pendingID
	}
	return sqlmock.NewRows([]string{
		"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
		"materialized_revision_id", "approved_revision_id", "pending_revision_id",
		"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
		"lock_version", "created_at", "updated_at",
	}).AddRow(10, "article_comment", 7, 42, "active", "visible", 90, 90, pending, nil, nil, nil, nil, lockVersion, fixedTime, fixedTime)
}

func expectLockedItem(mock sqlmock.Sqlmock, lockVersion uint64, pendingID *uint64) {
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*FOR UPDATE").
		WithArgs("article_comment", uint64(7), 1).
		WillReturnRows(activeItemRows(lockVersion, pendingID))
}

func transitionCommand() moderation.ApplyTransitionCommand {
	return moderation.ApplyTransitionCommand{
		Subject:             moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3},
		AuthorID:            42,
		ExpectedLockVersion: 4,
		ExpectedPendingID:   nil,
		Next: moderation.ItemState{
			LifecycleState: moderation.LifecycleActive,
			PublicState:    moderation.PublicVisible,
			Materialized:   moderation.NewRevision(),
			Approved:       moderation.NewRevision(),
		},
		Revision: &moderation.RevisionDraft{
			SubmitterID: 42, IdempotencyKey: "request-1", SubmittedContent: "原文", PublishedContent: "安全正文",
			RiskLevel: moderation.RiskLow, PolicyAction: moderation.ActionAutoApprove,
			ReviewStatus: moderation.ReviewApproved, RulesetVersion: 2, RuleMatchIDs: []uint64{1},
		},
		Materialize: moderation.NewRevision(),
		Log:         &moderation.ActionLog{ActorUserID: uint64Pointer(42), Action: moderation.EventSubmit, CreatedAt: fixedTime},
	}
}

func TestApplyTransitionLocksCreatesVersionMaterializesAndCommits(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()
	command.Revision.Images = []moderation.RevisionImageDraft{{
		ImageFingerprint: moderation.ImageFingerprint{SHA256: "sha", MD5: "md5", Size: 10},
		Seq:              1, ObjectKey: "moments/42/a.jpg", MediaType: "image/jpeg",
	}}

	mock.ExpectBegin()
	expectNoIdempotencyResult(mock, 42, "request-1")
	expectLockedItem(mock, 4, nil)
	expectLockedArticleSubject(mock, "旧正文")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(MAX(version), 0) FROM `moderation_revision` WHERE item_id = ?")).
		WithArgs(uint64(10)).WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))
	mock.ExpectExec("INSERT INTO `moderation_revision`").WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectExec("INSERT INTO `moderation_revision_image`").
		WithArgs(uint64(101), uint(1), "moments/42/a.jpg", "sha", "md5", uint64(10), "image/jpeg", false, fixedTime, fixedTime).
		WillReturnResult(sqlmock.NewResult(201, 1))
	mock.ExpectExec("UPDATE `moderation_item` SET .*`lock_version`=lock_version \\+ 1.*WHERE .*lock_version = \\?").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(10), uint64(4)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `article_comment` SET `content`=\\? WHERE .*id = \\?.*user_id = \\?.*article_id = \\?").
		WithArgs("安全正文", uint64(7), uint64(42), uint64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `moderation_action_log`").WillReturnResult(sqlmock.NewResult(501, 1))
	mock.ExpectCommit()

	got, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	assert.Equal(t, uint64(101), got.RevisionID)
	assert.Equal(t, uint64(3), got.RevisionVersion)
	assert.Equal(t, uint64(5), got.LockVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionCreatesFirstSubjectItemAndRevisionAtomically(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()
	command.Subject.ID = 0
	command.ExpectedLockVersion = 0
	command.CreateSubject = true

	mock.ExpectBegin()
	expectNoIdempotencyResult(mock, 42, "request-1")
	mock.ExpectQuery("SELECT `id` FROM `article` WHERE .*id = \\?.*status IN \\(\\?,\\?\\).*comment_status = \\?.*LIMIT \\?").
		WithArgs(uint64(3), uint(1), uint(2), uint8(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	mock.ExpectExec("INSERT INTO `article_comment`").WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectExec("INSERT INTO `moderation_item`").WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(MAX(version), 0) FROM `moderation_revision` WHERE item_id = ?")).
		WithArgs(uint64(10)).WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(0))
	mock.ExpectExec("INSERT INTO `moderation_revision`").WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `moderation_action_log`").WillReturnResult(sqlmock.NewResult(501, 1))
	mock.ExpectCommit()

	got, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	assert.Equal(t, uint64(7), got.Subject.ID)
	assert.Equal(t, uint64(10), got.ItemID)
	assert.Equal(t, uint64(101), got.RevisionID)
	assert.Equal(t, uint64(2), got.LockVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionCreatesPreReviewMomentHiddenInBusinessTable(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()
	command.Subject = moderation.SubjectRef{Type: moderation.SubjectMoment}
	command.ExpectedLockVersion = 0
	command.CreateSubject = true
	command.Next = moderation.ItemState{
		LifecycleState: moderation.LifecycleActive,
		PublicState:    moderation.PublicPlaceholder,
		Pending:        moderation.NewRevision(),
	}
	command.Revision.PolicyAction = moderation.ActionPreReview
	command.Revision.ReviewStatus = moderation.ReviewPending
	command.Materialize = moderation.RevisionRef{}

	mock.ExpectBegin()
	expectNoIdempotencyResult(mock, 42, "request-1")
	mock.ExpectQuery("SELECT `id` FROM `user` WHERE .*id = \\?.*status = \\?.*LIMIT \\?").
		WithArgs(uint64(42), uint8(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectExec("INSERT INTO `moment`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), nil, uint(42), "", uint8(0), uint8(1), uint(0), false).
		WillReturnResult(sqlmock.NewResult(8, 1))
	mock.ExpectExec("INSERT INTO `moderation_item`").WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version\\), 0\\)").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(0))
	mock.ExpectExec("INSERT INTO `moderation_revision`").WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `moderation_action_log`").WillReturnResult(sqlmock.NewResult(501, 1))
	mock.ExpectCommit()

	got, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	assert.Equal(t, uint64(8), got.Subject.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionSupersedesExpectedPendingRevision(t *testing.T) {
	repository, mock := newRepository(t)
	oldPending := uint64(90)
	command := transitionCommand()
	command.ExpectedPendingID = &oldPending
	command.SupersedeRevisionID = &oldPending

	mock.ExpectBegin()
	expectNoIdempotencyResult(mock, 42, "request-1")
	expectLockedItem(mock, 4, &oldPending)
	expectLockedArticleSubject(mock, "旧正文")
	mock.ExpectQuery("SELECT .* FROM `moderation_revision`.*FOR UPDATE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id"}).AddRow(oldPending, 10))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version\\), 0\\)").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))
	mock.ExpectExec("INSERT INTO `moderation_revision`").WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectExec("UPDATE `moderation_revision` SET .*`review_status`=\\?.*WHERE .*id = \\?.*item_id = \\?.*review_status = \\?").
		WithArgs(moderation.ReviewSuperseded, sqlmock.AnyArg(), oldPending, uint64(10), moderation.ReviewPending).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `article_comment` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `moderation_action_log`").WillReturnResult(sqlmock.NewResult(501, 1))
	mock.ExpectCommit()

	_, err := repository.ApplyTransition(context.Background(), command)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionRejectsOptimisticLockMismatch(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()

	mock.ExpectBegin()
	expectNoIdempotencyResult(mock, 42, "request-1")
	expectLockedItem(mock, 5, nil)
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrOptimisticLock)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionRejectsStalePendingRevision(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()
	expected := uint64(89)
	actual := uint64(90)
	command.ExpectedPendingID = &expected

	mock.ExpectBegin()
	expectNoIdempotencyResult(mock, 42, "request-1")
	expectLockedItem(mock, 4, &actual)
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrPendingRevisionConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionRollsBackWhenMaterializationFails(t *testing.T) {
	repository, mock := newRepository(t)
	command := transitionCommand()

	mock.ExpectBegin()
	expectNoIdempotencyResult(mock, 42, "request-1")
	expectLockedItem(mock, 4, nil)
	expectLockedArticleSubject(mock, "旧正文")
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version\\), 0\\)").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))
	mock.ExpectExec("INSERT INTO `moderation_revision`").WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `article_comment` SET ").WillReturnError(errors.New("write failed"))
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	require.Error(t, err)
	assert.ErrorContains(t, err, "write failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyTransitionDeleteRejectsMismatchedReplyParent(t *testing.T) {
	repository, mock := newRepository(t)
	command := moderation.ApplyTransitionCommand{
		Subject:             moderation.SubjectRef{Type: moderation.SubjectArticleCommentReply, ID: 9, RootID: 999, ParentID: uint64Pointer(5)},
		AuthorID:            42,
		ExpectedLockVersion: 4,
		Next: moderation.ItemState{
			LifecycleState: moderation.LifecycleDeleted,
			PublicState:    moderation.PublicHidden,
			DeletedAt:      &fixedTime,
		},
		DeleteSubject: true,
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*FOR UPDATE").
		WithArgs(moderation.SubjectArticleCommentReply, uint64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
			"materialized_revision_id", "approved_revision_id", "pending_revision_id",
			"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
			"lock_version", "created_at", "updated_at",
		}).AddRow(12, "article_comment_reply", 9, 42, "active", "visible", 80, 80, nil, nil, nil, nil, nil, 4, fixedTime, fixedTime))
	mock.ExpectQuery("SELECT .* FROM `article_comment_reply`.*FOR UPDATE").
		WithArgs(uint64(9), uint64(42), uint64(999), uint64(5), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "comment_id", "from_user_id", "parent_reply_id", "content"}))
	mock.ExpectRollback()

	_, err := repository.ApplyTransition(context.Background(), command)

	assert.ErrorIs(t, err, moderation.ErrSubjectNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecordBlockedAttemptIsIdempotentAndStoresNoBody(t *testing.T) {
	repository, mock := newRepository(t)
	attempt := moderation.BlockedAttempt{
		UserID: 42, SubjectType: moderation.SubjectMoment, IdempotencyKey: "blocked-1",
		RulesetVersion: 3, RuleMatchIDs: []uint64{7, 9}, CreatedAt: fixedTime,
	}

	mock.ExpectBegin()
	expectNoIdempotencyResult(mock, 42, "blocked-1")
	mock.ExpectExec("INSERT INTO `moderation_attempt` .*ON DUPLICATE KEY UPDATE `id`=`id`").
		WithArgs(uint64(42), moderation.SubjectMoment, nil, "blocked-1", uint64(3), "[7,9]", fixedTime).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT .* FROM `moderation_attempt` WHERE user_id = \\? AND idempotency_key = \\?").
		WithArgs(uint64(42), "blocked-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "content_type", "item_id", "idempotency_key", "ruleset_version", "rule_match_ids", "created_at"}).
			AddRow(33, 42, "moment", nil, "blocked-1", 3, "[7,9]", fixedTime))
	mock.ExpectCommit()

	got, err := repository.RecordBlockedAttempt(context.Background(), attempt)

	require.NoError(t, err)
	assert.Equal(t, moderation.ResultBlocked, got.Kind)
	assert.Equal(t, uint64(33), got.AttemptID)
	assert.Empty(t, got.Content)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFindResultByIdempotencyKeyReadsBothDomainsAndRejectsCollision(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT .* FROM .*moderation_revision.*UNION ALL.*moderation_attempt").
		WithArgs(uint64(42), "same-key", uint64(42), "same-key").
		WillReturnRows(sqlmock.NewRows([]string{"domain", "record_id", "item_id", "content_type", "content_id", "review_status", "public_state", "created_at"}).
			AddRow("revision", 9, 4, "moment", 8, "pending", "visible", fixedTime).
			AddRow("attempt", 10, nil, "moment", nil, "blocked", "", fixedTime))

	got, err := repository.FindResultByIdempotencyKey(context.Background(), 42, "same-key")

	assert.Nil(t, got)
	assert.ErrorIs(t, err, moderation.ErrIdempotencyDomainConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFindResultByIdempotencyKeyReturnsPendingAndVisibleContentSeparately(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery(`SELECT .*visible\.published_content.*FROM .*moderation_revision.*LEFT JOIN moderation_revision AS visible.*UNION ALL.*moderation_attempt`).
		WithArgs(uint64(42), "medium-edit", uint64(42), "medium-edit").
		WillReturnRows(sqlmock.NewRows([]string{
			"domain", "record_id", "item_id", "author_id", "content_type", "content_id", "review_status", "public_state", "created_at",
			"revision_version", "lock_version", "risk_level", "policy_action", "content", "visible_content",
		}).AddRow("revision", 9, 4, 42, "article_comment", 8, "pending", "visible", fixedTime,
			2, 3, "medium", "pre_review", "中风险编辑", "最后通过正文"))

	got, err := repository.FindResultByIdempotencyKey(context.Background(), 42, "medium-edit")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, uint64(42), got.AuthorID)
	assert.Equal(t, "中风险编辑", got.Content)
	assert.Equal(t, "最后通过正文", got.VisibleContent)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadPolicyContextDefaultsMissingProfileWithinOneReadTransaction(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_control` WHERE id = \\?").
		WithArgs(uint64(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"publishing_mode", "lock_version"}).AddRow("pre_review_all", 6))
	mock.ExpectQuery("SELECT .* FROM `user_moderation_profile` WHERE user_id = \\?").
		WithArgs(uint64(42), 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectCommit()

	got, err := repository.LoadPolicyContext(context.Background(), 42)

	require.NoError(t, err)
	assert.Equal(t, moderation.TrustNew, got.TrustLevel)
	assert.Equal(t, moderation.SanctionActive, got.SanctionState)
	assert.Equal(t, moderation.PublishingPreReviewAll, got.PublishingMode)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadSubjectUsesTypedTableMappings(t *testing.T) {
	tests := []struct {
		name       string
		ref        moderation.SubjectRef
		table      string
		columns    []string
		values     []driver.Value
		wantRoot   uint64
		wantParent uint64
	}{
		{name: "article comment", ref: moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 1}, table: "article_comment", columns: []string{"id", "article_id", "user_id", "content"}, values: []driver.Value{1, 11, 42, "a"}, wantRoot: 11},
		{name: "moment comment", ref: moderation.SubjectRef{Type: moderation.SubjectMomentComment, ID: 2}, table: "moment_comment", columns: []string{"id", "moment_id", "user_id", "content"}, values: []driver.Value{2, 12, 42, "b"}, wantRoot: 12},
		{name: "guestbook", ref: moderation.SubjectRef{Type: moderation.SubjectGuestbook, ID: 3}, table: "guestbook", columns: []string{"id", "owner_user_id", "from_user_id", "content"}, values: []driver.Value{3, 13, 42, "c"}, wantRoot: 13},
		{name: "article reply", ref: moderation.SubjectRef{Type: moderation.SubjectArticleCommentReply, ID: 4}, table: "article_comment_reply", columns: []string{"id", "comment_id", "parent_reply_id", "from_user_id", "content"}, values: []driver.Value{4, 14, 24, 42, "d"}, wantRoot: 14, wantParent: 24},
		{name: "moment reply", ref: moderation.SubjectRef{Type: moderation.SubjectMomentCommentReply, ID: 5}, table: "moment_comment_reply", columns: []string{"id", "comment_id", "parent_reply_id", "from_user_id", "content"}, values: []driver.Value{5, 15, 25, 42, "e"}, wantRoot: 15, wantParent: 25},
		{name: "guestbook reply", ref: moderation.SubjectRef{Type: moderation.SubjectGuestbookReply, ID: 6}, table: "guestbook_reply", columns: []string{"id", "comment_id", "parent_reply_id", "from_user_id", "content"}, values: []driver.Value{6, 16, 26, 42, "f"}, wantRoot: 16, wantParent: 26},
		{name: "moment", ref: moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 7}, table: "moment", columns: []string{"id", "user_id", "content"}, values: []driver.Value{7, 42, "g"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository, mock := newRepository(t)
			mock.ExpectQuery("SELECT .* FROM `"+tt.table+"` WHERE .*`id` = \\?.*LIMIT \\?").
				WithArgs(tt.ref.ID, 1).
				WillReturnRows(sqlmock.NewRows(tt.columns).AddRow(tt.values...))

			got, err := repository.LoadSubject(context.Background(), tt.ref)

			require.NoError(t, err)
			assert.Equal(t, uint64(42), got.AuthorID)
			assert.Equal(t, tt.wantRoot, got.Ref.RootID)
			if tt.ref.Type == moderation.SubjectArticleCommentReply || tt.ref.Type == moderation.SubjectMomentCommentReply || tt.ref.Type == moderation.SubjectGuestbookReply {
				require.NotNil(t, got.Ref.ParentID)
				assert.Equal(t, tt.wantParent, *got.Ref.ParentID)
			} else {
				assert.Nil(t, got.Ref.ParentID)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestLoadEnabledRulesUsesDeterministicOrder(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT .* FROM `moderation_rule` WHERE enabled = \\? ORDER BY priority ASC,id ASC").
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "rule_type", "pattern", "risk_level", "priority", "enabled", "ruleset_version", "created_at", "updated_at"}).
			AddRow(2, "规则二", "regexp", "x+", "medium", 10, true, 3, fixedTime, fixedTime).
			AddRow(5, "规则五", "keyword", "词", "high", 10, true, 3, fixedTime, fixedTime))

	got, err := repository.LoadEnabledRules(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, uint64(2), got[0].ID)
	assert.Equal(t, uint64(5), got[1].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadModerationViewBatchesAndHidesPendingDetailsFromPublic(t *testing.T) {
	repository, mock := newRepository(t)
	refs := []moderation.SubjectRef{
		{Type: moderation.SubjectArticleComment, ID: 7},
		{Type: moderation.SubjectMoment, ID: 8},
	}
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*LEFT JOIN moderation_revision AS materialized ON materialized.id = moderation_item.materialized_revision_id AND materialized.item_id = moderation_item.id.*LEFT JOIN moderation_revision AS pending ON pending.id = moderation_item.pending_revision_id AND pending.item_id = moderation_item.id.*content_type IN.*content_id IN").
		WillReturnRows(sqlmock.NewRows([]string{
			"content_type", "content_id", "author_id", "lifecycle_state", "public_state",
			"materialized_revision_id", "approved_revision_id", "pending_revision_id",
			"materialized_content", "pending_content", "pending_risk_level", "pending_review_status", "pending_rule_match_ids",
		}).
			AddRow("article_comment", 7, 42, "active", "visible", 11, 10, 11, "低风险正文", "低风险正文", "low", "pending", "[1]").
			AddRow("moment", 8, 43, "active", "visible", 20, 20, 21, "旧通过正文", "中风险正文", "medium", "pending", "[2]"))
	mock.ExpectQuery("SELECT .* FROM moderation_revision_image AS revision_image").
		WillReturnRows(moderationViewImageRows())

	got, err := repository.LoadModerationView(context.Background(), refs, moderation.Viewer{Role: moderation.ViewerPublic})

	require.NoError(t, err)
	assert.Equal(t, "低风险正文", got[refs[0].Key()].VisibleContent)
	assert.Equal(t, "旧通过正文", got[refs[1].Key()].VisibleContent)
	assert.Nil(t, got[refs[0].Key()].PendingContent)
	assert.Empty(t, got[refs[0].Key()].PendingRuleMatchIDs)
	assert.False(t, got[refs[0].Key()].CanInteract)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadModerationViewReturnsPendingDetailsOnlyToAuthor(t *testing.T) {
	repository, mock := newRepository(t)
	ref := moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 8}
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*LEFT JOIN moderation_revision AS materialized ON materialized.id = moderation_item.materialized_revision_id AND materialized.item_id = moderation_item.id.*LEFT JOIN moderation_revision AS pending ON pending.id = moderation_item.pending_revision_id AND pending.item_id = moderation_item.id").
		WillReturnRows(sqlmock.NewRows([]string{
			"content_type", "content_id", "author_id", "lifecycle_state", "public_state",
			"materialized_revision_id", "approved_revision_id", "pending_revision_id",
			"materialized_content", "pending_content", "pending_risk_level", "pending_review_status", "pending_rule_match_ids",
		}).AddRow("moment", 8, 43, "active", "visible", 20, 20, 21, "旧正文", "待审正文", "medium", "pending", "[2,3]"))
	mock.ExpectQuery("SELECT .* FROM moderation_revision_image AS revision_image").
		WillReturnRows(moderationViewImageRows())

	got, err := repository.LoadModerationView(context.Background(), []moderation.SubjectRef{ref}, moderation.Viewer{Role: moderation.ViewerAuthor, UserID: 43})

	require.NoError(t, err)
	require.NotNil(t, got[ref.Key()].PendingContent)
	assert.Equal(t, "待审正文", *got[ref.Key()].PendingContent)
	assert.Equal(t, []uint64{2, 3}, got[ref.Key()].PendingRuleMatchIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func uint64Pointer(value uint64) *uint64 { return &value }
