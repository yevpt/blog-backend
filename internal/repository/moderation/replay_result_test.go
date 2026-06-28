package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestStoredRevisionResultCarriesStableDecisionFields(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT .*revision.risk_level.*revision.policy_action.*revision.published_content.*UNION ALL").
		WithArgs(uint64(42), "replay", uint64(42), "replay").
		WillReturnRows(sqlmock.NewRows([]string{
			"domain", "record_id", "item_id", "author_id", "content_type", "content_id",
			"review_status", "public_state", "created_at", "revision_version", "lock_version",
			"risk_level", "policy_action", "content",
		}).AddRow("revision", 9, 4, 42, "moment", 8, "pending", "visible", fixedTime, 2, 5, "low", "post_review", "首次正文"))

	got, err := repository.FindResultByIdempotencyKey(context.Background(), 42, "replay")

	require.NoError(t, err)
	assert.Equal(t, uint64(42), got.AuthorID)
	assert.Equal(t, moderation.RiskLow, got.RiskLevel)
	assert.Equal(t, moderation.ActionPostReview, got.PolicyAction)
	assert.Equal(t, "首次正文", got.Content)
	assert.Equal(t, uint64(2), got.RevisionVersion)
	assert.Equal(t, uint64(5), got.LockVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}
