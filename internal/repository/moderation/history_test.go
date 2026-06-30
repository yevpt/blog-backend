package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestLoadReviewHistoryReturnsPagedRevisionsImagesAndEvents(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT moderation_item.id AS item_id,count\\(moderation_revision.id\\) AS total FROM `moderation_item` LEFT JOIN moderation_revision ON moderation_revision.item_id = moderation_item.id WHERE moderation_item.id = \\? GROUP BY `moderation_item`.`id` LIMIT \\?").
		WithArgs(uint64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"item_id", "total"}).AddRow(10, 3))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` JOIN moderation_item ON moderation_item.id = moderation_revision.item_id WHERE moderation_item.id = \\? ORDER BY moderation_revision.version DESC,moderation_revision.id DESC LIMIT \\?").
		WithArgs(uint64(10), 2).
		WillReturnRows(reviewRecordRows().
			AddRow(uint64(10), "moment", uint64(7), uint64(42), uint64(3), "active", "visible", uint64(31), uint64(31), nil, nil, nil, nil, nil, uint64(31), uint64(3), "v3 原文", "v3 正文", "low", "auto_approve", "approved", uint8(1), uint8(1), "approved", nil, uint64(1), fixedTime, fixedTime).
			AddRow(uint64(10), "moment", uint64(7), uint64(42), uint64(3), "active", "visible", uint64(31), uint64(31), nil, nil, nil, nil, nil, uint64(30), uint64(2), "v2 原文", "v2 正文", "medium", "pre_review", "superseded", uint8(1), uint8(1), nil, nil, nil, nil, fixedTime))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision_image` WHERE revision_id IN \\(\\?,\\?\\) ORDER BY revision_id ASC,seq ASC,id ASC").
		WithArgs(uint64(31), uint64(30)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revision_id", "seq", "object_key", "sha256", "md5", "size", "media_type", "is_gif"}).
			AddRow(101, 30, 0, "moderation/history/moments/10/v2.jpg", "sha-v2", "md5-v2", 20, "image/jpeg", false).
			AddRow(102, 31, 0, "moments/42/7/v3.jpg", "sha-v3", "md5-v3", 30, "image/jpeg", false))
	mock.ExpectQuery("SELECT .* FROM `moderation_action_log` WHERE item_id = \\? ORDER BY created_at ASC,id ASC").
		WithArgs(uint64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revision_id", "actor_user_id", "action", "reason", "metadata_json", "created_at"}).
			AddRow(201, 30, 42, "resubmit", nil, nil, fixedTime).
			AddRow(202, 31, 1, "approve", "通过", `{\"source\":\"admin\"}`, fixedTime))

	page, err := repository.LoadReviewHistory(context.Background(), 10, 1, 2)

	require.NoError(t, err)
	assert.Equal(t, int64(3), page.Total)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 2, page.PageSize)
	require.Len(t, page.Revisions, 2)
	assert.Equal(t, []uint64{31, 30}, []uint64{page.Revisions[0].RevisionID, page.Revisions[1].RevisionID})
	require.Len(t, page.Images[30], 1)
	assert.Equal(t, "moderation/history/moments/10/v2.jpg", page.Images[30][0].ObjectKey)
	require.Len(t, page.Events, 2)
	assert.Equal(t, moderation.EventApprove, page.Events[1].Action)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadReviewHistoryReturnsItemNotFound(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT moderation_item.id AS item_id,count\\(moderation_revision.id\\) AS total FROM `moderation_item` LEFT JOIN moderation_revision ON moderation_revision.item_id = moderation_item.id WHERE moderation_item.id = \\? GROUP BY `moderation_item`.`id` LIMIT \\?").
		WithArgs(uint64(99), 1).
		WillReturnRows(sqlmock.NewRows([]string{"item_id", "total"}))

	_, err := repository.LoadReviewHistory(context.Background(), 99, 1, 20)

	require.ErrorIs(t, err, moderation.ErrItemNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}
