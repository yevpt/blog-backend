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
			AddRow("article_comment", 1, 7, "active", "visible", 11, nil, 11, "低风险正文", "低风险原文", "low", "pending", "[3]", nil, nil).
			AddRow("article_comment", 2, 8, "active", "placeholder", nil, nil, 12, nil, "中风险原文", "medium", "pending", "[4]", nil, nil))
	mock.ExpectQuery("SELECT .* FROM moderation_revision_image AS revision_image LEFT JOIN moderation_image AS image_record").
		WillReturnRows(moderationViewImageRows().
			AddRow(101, 11, 1, "comments/original-a.jpg", "moderation/previews/a.jpg", "pending", false).
			AddRow(102, 12, 1, "comments/original-b.gif", "system/moderation/gif-review.jpg", "pending", true))

	got, err := repository.LoadModerationView(context.Background(), refs, moderation.Viewer{Role: moderation.ViewerPublic})

	require.NoError(t, err)
	low := got[refs[0].Key()]
	assert.Equal(t, "低风险正文", low.VisibleContent)
	assert.Equal(t, moderation.DisplayPending, low.DisplayVersion)
	assert.True(t, low.HasPendingRevision)
	assert.False(t, low.CanInteract)
	assert.Nil(t, low.PendingContent)
	assert.Empty(t, low.PendingRuleMatchIDs)
	require.Len(t, low.VisibleImages, 1)
	assert.Equal(t, "moderation/previews/a.jpg", low.VisibleImages[0].DisplayObjectKey)
	assert.False(t, low.VisibleImages[0].Approved)
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
			AddRow("guestbook", 2, 7, "active", "visible", 10, 10, 12, "旧正文", "新中风险正文", "medium", "pending", "[4]", nil, nil))
	mock.ExpectQuery("SELECT .* FROM moderation_revision_image AS revision_image LEFT JOIN moderation_image AS image_record").
		WillReturnRows(moderationViewImageRows().
			AddRow(201, 10, 1, "comments/approved.jpg", nil, "approved", false).
			AddRow(202, 12, 1, "comments/new.jpg", "moderation/previews/new.jpg", "pending", false))

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
	require.Len(t, view.VisibleImages, 1)
	assert.Equal(t, "comments/approved.jpg", view.VisibleImages[0].DisplayObjectKey)
	require.Len(t, view.PendingImages, 1)
	assert.Equal(t, "comments/new.jpg", view.PendingImages[0].DisplayObjectKey)
	assert.True(t, view.PendingImages[0].Approved)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModerationViewAuthorSeesPlaceholderPendingBodyAndImages(t *testing.T) {
	repository, mock := newRepository(t)
	ref := moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 3, RootID: 1}
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*LEFT JOIN moderation_revision AS materialized.*LEFT JOIN moderation_revision AS pending").
		WillReturnRows(moderationViewRows().
			AddRow("moment", 3, 7, "active", "placeholder", nil, nil, 12, nil, "中风险碎语正文", "medium", "pending", "[4]", nil, nil))
	mock.ExpectQuery("SELECT .* FROM moderation_revision_image AS revision_image LEFT JOIN moderation_image AS image_record").
		WillReturnRows(moderationViewImageRows().
			AddRow(301, 12, 1, "moments/new.jpg", "moderation/previews/new.jpg", "pending", false))

	got, err := repository.LoadModerationView(context.Background(), []moderation.SubjectRef{ref}, moderation.Viewer{
		Role: moderation.ViewerAuthor, UserID: 7,
	})

	require.NoError(t, err)
	view := got[ref.Key()]
	assert.Equal(t, "中风险碎语正文", view.VisibleContent)
	assert.Equal(t, moderation.DisplayNone, view.DisplayVersion)
	require.NotNil(t, view.PendingContent)
	assert.Equal(t, "中风险碎语正文", *view.PendingContent)
	require.Len(t, view.VisibleImages, 1)
	assert.Equal(t, "moments/new.jpg", view.VisibleImages[0].DisplayObjectKey)
	assert.True(t, view.VisibleImages[0].Approved)
	require.Len(t, view.PendingImages, 1)
	assert.Equal(t, "moments/new.jpg", view.PendingImages[0].DisplayObjectKey)
	assert.True(t, view.PendingImages[0].Approved)
	require.NoError(t, mock.ExpectationsWereMet())
}

func moderationViewImageRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "revision_id", "seq", "object_key", "preview_object_key", "status", "is_gif",
	})
}

func moderationViewRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"content_type", "content_id", "author_id", "lifecycle_state", "public_state",
		"materialized_revision_id", "approved_revision_id", "pending_revision_id",
		"materialized_content", "pending_content", "pending_risk_level", "pending_review_status", "pending_rule_match_ids",
		"rejected_revision_id", "rejected_content",
	})
}
