package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestApplyPublishedImageKeysUpdatesKeysAndRebuildsMomentMedia(t *testing.T) {
	repository, mock := newRepository(t)
	const itemID, revisionID, momentID, authorID = 10, 101, 8, 42

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*WHERE.*id = \\?.*LIMIT.*FOR UPDATE").
		WithArgs(itemID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
			"materialized_revision_id", "approved_revision_id", "pending_revision_id",
			"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
			"lock_version", "created_at", "updated_at",
		}).AddRow(itemID, "moment", momentID, authorID, "active", "visible",
			revisionID, revisionID, nil, nil, nil, nil, nil,
			uint64(2), fixedTime, fixedTime))
	mock.ExpectExec("UPDATE `moderation_revision_image` SET .*`object_key`=\\?.*WHERE.*revision_id = \\?.*seq = \\?").
		WithArgs("moments/42/8/sha.jpg", fixedTime, revisionID, uint(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `moment_media` WHERE moment_id = \\?").
		WithArgs(momentID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision_image` WHERE revision_id = .*ORDER BY seq ASC,id ASC").
		WithArgs(revisionID).
		WillReturnRows(sqlmock.NewRows([]string{
			"revision_id", "seq", "object_key", "sha256", "md5", "size", "media_type", "is_gif",
		}).AddRow(revisionID, 1, "moments/42/8/sha.jpg", "sha", "md5", 10, "image/jpeg", false))
	mock.ExpectExec("INSERT INTO `moment_media`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repository.ApplyPublishedImageKeys(context.Background(), moderation.PublishedImageCommand{
		ItemID: itemID, RevisionID: revisionID, MomentID: momentID, AuthorID: authorID,
		ImageKeys: []moderation.PublishedImageKey{{Seq: 1, ObjectKey: "moments/42/8/sha.jpg"}},
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyPublishedImageKeysAppliesAuditMovesBeforeRebuild(t *testing.T) {
	repository, mock := newRepository(t)
	const itemID, revisionID, momentID, authorID = 10, 102, 8, 42

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*WHERE.*id = \\?.*LIMIT.*FOR UPDATE").
		WithArgs(itemID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
			"materialized_revision_id", "approved_revision_id", "pending_revision_id",
			"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
			"lock_version", "created_at", "updated_at",
		}).AddRow(itemID, "moment", momentID, authorID, "active", "visible",
			revisionID, revisionID, nil, nil, nil, nil, nil,
			uint64(3), fixedTime, fixedTime))
	mock.ExpectExec("UPDATE `moderation_revision_image` SET .*`object_key`=\\?.*WHERE.*revision_id = \\?.*seq = \\?").
		WithArgs("moments/42/8/new.jpg", fixedTime, revisionID, uint(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `moderation_revision_image` SET .*`object_key`=\\?.*WHERE.*object_key = \\?.*revision_id IN.*moderation_revision.*item_id = \\?").
		WithArgs("moderation/history/moments/10/old.jpg", fixedTime, "moments/42/8/old.jpg", itemID).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM `moment_media` WHERE moment_id = \\?").
		WithArgs(momentID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision_image` WHERE revision_id = .*ORDER BY seq ASC,id ASC").
		WithArgs(revisionID).
		WillReturnRows(sqlmock.NewRows([]string{
			"revision_id", "seq", "object_key", "sha256", "md5", "size", "media_type", "is_gif",
		}).AddRow(revisionID, 1, "moments/42/8/new.jpg", "newsha", "newmd5", 20, "image/jpeg", false))
	mock.ExpectExec("INSERT INTO `moment_media`").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	err := repository.ApplyPublishedImageKeys(context.Background(), moderation.PublishedImageCommand{
		ItemID: itemID, RevisionID: revisionID, MomentID: momentID, AuthorID: authorID,
		ImageKeys:  []moderation.PublishedImageKey{{Seq: 1, ObjectKey: "moments/42/8/new.jpg"}},
		AuditMoves: []moderation.AuditImageMove{{OldObjectKey: "moments/42/8/old.jpg", NewObjectKey: "moderation/history/moments/10/old.jpg"}},
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyPublishedImageKeysRejectsMissingRevisionImage(t *testing.T) {
	repository, mock := newRepository(t)
	const itemID, revisionID, momentID, authorID = 10, 101, 8, 42

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*WHERE.*id = \\?.*LIMIT.*FOR UPDATE").
		WithArgs(itemID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
			"materialized_revision_id", "approved_revision_id", "pending_revision_id",
			"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
			"lock_version", "created_at", "updated_at",
		}).AddRow(itemID, "moment", momentID, authorID, "active", "visible",
			revisionID, revisionID, nil, nil, nil, nil, nil,
			uint64(2), fixedTime, fixedTime))
	mock.ExpectExec("UPDATE `moderation_revision_image` SET .*WHERE.*revision_id = \\?.*seq = \\?").
		WithArgs("moments/42/8/missing.jpg", fixedTime, revisionID, uint(9)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := repository.ApplyPublishedImageKeys(context.Background(), moderation.PublishedImageCommand{
		ItemID: itemID, RevisionID: revisionID, MomentID: momentID, AuthorID: authorID,
		ImageKeys: []moderation.PublishedImageKey{{Seq: 9, ObjectKey: "moments/42/8/missing.jpg"}},
	})

	assert.ErrorIs(t, err, moderation.ErrRevisionStateConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyPublishedImageKeysRestoresPlaceholderAfterRecovery(t *testing.T) {
	repository, mock := newRepository(t)
	const itemID, revisionID, momentID, authorID = 10, 101, 8, 42

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*WHERE.*id = \\?.*LIMIT.*FOR UPDATE").
		WithArgs(itemID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
			"materialized_revision_id", "approved_revision_id", "pending_revision_id",
			"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
			"lock_version", "created_at", "updated_at",
		}).AddRow(itemID, "moment", momentID, authorID, "active", "placeholder",
			revisionID, revisionID, nil, nil, nil, nil, nil,
			uint64(2), fixedTime, fixedTime))
	mock.ExpectExec("UPDATE `moderation_item` SET .*`public_state`=\\?.*WHERE.*id = \\?.*public_state = \\?").
		WithArgs("visible", fixedTime, itemID, "placeholder").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `moment_media` WHERE moment_id = \\?").
		WithArgs(momentID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision_image` WHERE revision_id = .*ORDER BY seq ASC,id ASC").
		WithArgs(revisionID).WillReturnRows(sqlmock.NewRows([]string{
		"revision_id", "seq", "object_key", "sha256", "md5", "size", "media_type", "is_gif",
	}))
	mock.ExpectCommit()

	err := repository.ApplyPublishedImageKeys(context.Background(), moderation.PublishedImageCommand{
		ItemID: itemID, RevisionID: revisionID, MomentID: momentID, AuthorID: authorID,
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyPublishedImageKeysRejectsRevisionNotMaterialized(t *testing.T) {
	repository, mock := newRepository(t)
	const itemID, revisionID, momentID, authorID = 10, 999, 8, 42

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*WHERE.*id = \\?.*LIMIT.*FOR UPDATE").
		WithArgs(itemID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
			"materialized_revision_id", "approved_revision_id", "pending_revision_id",
			"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
			"lock_version", "created_at", "updated_at",
		}).AddRow(itemID, "moment", momentID, authorID, "active", "visible",
			uint64(101), uint64(101), nil, nil, nil, nil, nil,
			uint64(2), fixedTime, fixedTime))
	mock.ExpectRollback()

	err := repository.ApplyPublishedImageKeys(context.Background(), moderation.PublishedImageCommand{
		ItemID: itemID, RevisionID: revisionID, MomentID: momentID, AuthorID: authorID,
		ImageKeys: []moderation.PublishedImageKey{{Seq: 1, ObjectKey: "moments/42/8/sha.jpg"}},
	})

	assert.ErrorIs(t, err, moderation.ErrRevisionStateConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestApplyPublishedImageKeysRejectsInvalidCommand(t *testing.T) {
	repository, _ := newRepository(t)

	err := repository.ApplyPublishedImageKeys(context.Background(), moderation.PublishedImageCommand{
		ItemID: 0, RevisionID: 101, MomentID: 8, AuthorID: 42,
	})

	assert.ErrorIs(t, err, moderation.ErrInvalidCommand)
}
