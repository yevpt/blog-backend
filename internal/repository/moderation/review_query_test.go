package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestListReviewRecordsReturnsPendingRevisionAndMomentOptions(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `moderation_revision` JOIN moderation_item").
		WithArgs(moderation.ReviewPending).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` JOIN moderation_item.*ORDER BY moderation_revision.created_at DESC,moderation_revision.id DESC LIMIT \\?").
		WithArgs(moderation.ReviewPending, 20).
		WillReturnRows(reviewRecordRows().AddRow(
			uint64(10), "moment", uint64(7), uint64(42), uint64(3), "active", "placeholder",
			nil, nil, uint64(20), nil, nil, nil, nil, uint64(20), uint64(1), "待审原文", "待审正文",
			"medium", "pre_review", "pending", uint8(1), uint8(0), nil, nil, nil, nil, fixedTime,
		))

	page, err := repository.ListReviewRecords(context.Background(), moderation.ReviewFilter{
		Page: 1, PageSize: 20, ReviewStatus: moderation.ReviewPending,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	item := page.Items[0]
	assert.Equal(t, moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 7}, item.Subject)
	assert.Equal(t, uint64(20), item.RevisionID)
	require.NotNil(t, item.MomentOptions)
	assert.Equal(t, moderation.MomentOptions{Status: 1, CommentStatus: 0}, *item.MomentOptions)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadReviewRecordReturnsCanonicalCommentRelation(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` JOIN moderation_item.*WHERE .*moderation_item.id = \\?.*moderation_revision.id = \\?.*LIMIT \\?").
		WithArgs(uint64(10), uint64(20), 1).
		WillReturnRows(reviewRecordRows().AddRow(
			uint64(10), "article_comment", uint64(7), uint64(42), uint64(3), "active", "visible",
			uint64(19), uint64(19), uint64(20), nil, nil, nil, nil, uint64(20), uint64(2), "待审原文", "待审正文",
			"medium", "pre_review", "pending", nil, nil, nil, nil, nil, nil, fixedTime,
		))
	mock.ExpectQuery("SELECT .* FROM `article_comment` WHERE .*`id` = \\?.*LIMIT \\?").
		WithArgs(uint64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "article_id", "user_id", "content"}).
			AddRow(7, 3, 42, "当前公开正文"))

	record, err := repository.LoadReviewRecord(context.Background(), 10, 20)

	require.NoError(t, err)
	assert.Equal(t, moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3}, record.Subject)
	assert.Equal(t, uint64(42), record.AuthorID)
	assert.Equal(t, moderation.ExistingRevision(19), record.State.Approved)
	assert.Equal(t, moderation.ExistingRevision(20), record.State.Pending)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadCurrentReviewRecordPrefersPendingRevision(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT `pending_revision_id` FROM `moderation_item` WHERE id = \\?.*LIMIT \\?").
		WithArgs(uint64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"pending_revision_id"}).AddRow(uint64(20)))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` JOIN moderation_item.*WHERE .*moderation_item.id = \\?.*moderation_revision.id = \\?.*LIMIT \\?").
		WithArgs(uint64(10), uint64(20), 1).
		WillReturnRows(reviewRecordRows().AddRow(
			uint64(10), "moment", uint64(7), uint64(42), uint64(3), "active", "placeholder",
			nil, nil, uint64(20), nil, nil, nil, nil, uint64(20), uint64(1), "原文", "正文",
			"medium", "pre_review", "pending", uint8(1), uint8(1), nil, nil, nil, nil, fixedTime,
		))
	mock.ExpectQuery("SELECT .* FROM `moment` WHERE `moment`.`id` = \\?.*LIMIT \\?").
		WithArgs(uint64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "content"}).AddRow(7, 42, ""))

	record, err := repository.LoadCurrentReviewRecord(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, uint64(20), record.RevisionID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func reviewRecordRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"item_id", "content_type", "content_id", "author_id", "lock_version", "lifecycle_state", "public_state",
		"materialized_revision_id", "approved_revision_id", "pending_revision_id",
		"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
		"revision_id", "revision_version", "submitted_content", "published_content", "risk_level",
		"policy_action", "review_status", "moment_status", "moment_comment_status",
		"decision_type", "decision_reason", "reviewer_id", "reviewed_at", "created_at",
	})
}
