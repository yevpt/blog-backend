package moderation_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestModerationViewAuthorSeesRejectedHiddenContent(t *testing.T) {
	repository, mock := newRepository(t)
	ref := moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 9}
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*LEFT JOIN moderation_revision AS materialized.*LEFT JOIN moderation_revision AS pending").
		WillReturnRows(moderationViewRows().
			AddRow("moment", 9, 7, "active", "hidden", nil, nil, nil, nil, nil, nil, nil, nil, uint64(21), "被驳回正文"))
	mock.ExpectQuery("SELECT .* FROM moderation_revision_image AS revision_image LEFT JOIN moderation_image AS image_record").
		WillReturnRows(moderationViewImageRows())

	got, err := repository.LoadModerationView(context.Background(), []moderation.SubjectRef{ref}, moderation.Viewer{
		Role: moderation.ViewerAuthor, UserID: 7,
	})

	require.NoError(t, err)
	view := got[ref.Key()]
	assert.Equal(t, "被驳回正文", view.VisibleContent)
	require.NotNil(t, view.LastReviewStatus)
	assert.Equal(t, moderation.ReviewRejected, *view.LastReviewStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestModerationViewPublicDoesNotSeeRejectedHiddenContent(t *testing.T) {
	repository, mock := newRepository(t)
	ref := moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 9}
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*LEFT JOIN moderation_revision AS materialized.*LEFT JOIN moderation_revision AS pending").
		WillReturnRows(moderationViewRows().
			AddRow("moment", 9, 7, "active", "hidden", nil, nil, nil, nil, nil, nil, nil, nil, uint64(21), "被驳回正文"))
	mock.ExpectQuery("SELECT .* FROM moderation_revision_image AS revision_image LEFT JOIN moderation_image AS image_record").
		WillReturnRows(moderationViewImageRows())

	got, err := repository.LoadModerationView(context.Background(), []moderation.SubjectRef{ref}, moderation.Viewer{
		Role: moderation.ViewerPublic,
	})

	require.NoError(t, err)
	view := got[ref.Key()]
	assert.Empty(t, view.VisibleContent)
	assert.Nil(t, view.LastReviewStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}
