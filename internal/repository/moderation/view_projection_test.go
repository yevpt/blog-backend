package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestModerationViewPublicProjectionNeverLeaksPendingBody(t *testing.T) {
	repository, mock := newRepository(t)
	refs := []moderation.SubjectRef{
		{Type: moderation.SubjectArticleComment, ID: 1, RootID: 9},
		{Type: moderation.SubjectArticleComment, ID: 2, RootID: 9},
	}
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*LEFT JOIN moderation_revision AS materialized.*LEFT JOIN moderation_revision AS pending").
		WillReturnRows(moderationViewRows().
			AddRow("article_comment", 1, 7, "active", "visible", 11, nil, 11, "低风险正文", "低风险原文", "low", "pending", "[3]").
			AddRow("article_comment", 2, 8, "active", "placeholder", nil, nil, 12, nil, "中风险原文", "medium", "pending", "[4]"))

	got, err := repository.LoadModerationView(context.Background(), refs, moderation.Viewer{Role: moderation.ViewerPublic})

	require.NoError(t, err)
	low := got[refs[0].Key()]
	assert.Equal(t, "低风险正文", low.VisibleContent)
	assert.Equal(t, moderation.DisplayPending, low.DisplayVersion)
	assert.True(t, low.HasPendingRevision)
	assert.False(t, low.CanInteract)
	assert.Nil(t, low.PendingContent)
	assert.Empty(t, low.PendingRuleMatchIDs)
	medium := got[refs[1].Key()]
	assert.Empty(t, medium.VisibleContent)
	assert.Equal(t, moderation.DisplayNone, medium.DisplayVersion)
	assert.Nil(t, medium.PendingContent)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModerationViewAuthorGetsPendingEditorContentButKeepsApprovedDisplay(t *testing.T) {
	repository, mock := newRepository(t)
	ref := moderation.SubjectRef{Type: moderation.SubjectGuestbook, ID: 2, RootID: 1}
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*LEFT JOIN moderation_revision AS materialized.*LEFT JOIN moderation_revision AS pending").
		WillReturnRows(moderationViewRows().
			AddRow("guestbook", 2, 7, "active", "visible", 10, 10, 12, "旧正文", "新中风险正文", "medium", "pending", "[4]"))

	got, err := repository.LoadModerationView(context.Background(), []moderation.SubjectRef{ref}, moderation.Viewer{
		Role: moderation.ViewerAuthor, UserID: 7,
	})

	require.NoError(t, err)
	view := got[ref.Key()]
	assert.Equal(t, "旧正文", view.VisibleContent)
	assert.Equal(t, moderation.DisplayLastApproved, view.DisplayVersion)
	require.NotNil(t, view.PendingContent)
	assert.Equal(t, "新中风险正文", *view.PendingContent)
	require.NotNil(t, view.PendingRiskLevel)
	assert.Equal(t, moderation.RiskMedium, *view.PendingRiskLevel)
	require.NotNil(t, view.PendingReviewStatus)
	assert.Equal(t, moderation.ReviewPending, *view.PendingReviewStatus)
	assert.Equal(t, []uint64{4}, view.PendingRuleMatchIDs)
	assert.False(t, view.CanInteract)
	require.NoError(t, mock.ExpectationsWereMet())
}

func moderationViewRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"content_type", "content_id", "author_id", "lifecycle_state", "public_state",
		"materialized_revision_id", "approved_revision_id", "pending_revision_id",
		"materialized_content", "pending_content", "pending_risk_level", "pending_review_status", "pending_rule_match_ids",
	})
}
