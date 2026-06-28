package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestUseApprovedImageRequiresFullFingerprintAndTouchesLastUsedAt(t *testing.T) {
	repository, mock := newRepository(t)
	fingerprint := moderation.ImageFingerprint{SHA256: "sha256", MD5: "md5", Size: 123}
	mock.ExpectExec("UPDATE moderation_image SET last_used_at = .*, updated_at = .* WHERE md5 = .*sha256 = .*size = .*status = .*").
		WithArgs(fixedTime, fixedTime, "md5", "sha256", uint64(123), moderation.ImageApproved).
		WillReturnResult(sqlmock.NewResult(0, 1))

	approved, err := repository.UseApprovedImage(context.Background(), fingerprint, fixedTime)

	require.NoError(t, err)
	assert.True(t, approved)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUseApprovedImageReturnsFalseWithoutExactMatch(t *testing.T) {
	repository, mock := newRepository(t)
	fingerprint := moderation.ImageFingerprint{SHA256: "sha256", MD5: "md5", Size: 123}
	mock.ExpectExec("UPDATE moderation_image SET last_used_at = .*").
		WithArgs(fixedTime, fixedTime, "md5", "sha256", uint64(123), moderation.ImageApproved).
		WillReturnResult(sqlmock.NewResult(0, 0))

	approved, err := repository.UseApprovedImage(context.Background(), fingerprint, fixedTime)

	require.NoError(t, err)
	assert.False(t, approved)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertPendingImageDoesNotDowngradeApprovedRecord(t *testing.T) {
	repository, mock := newRepository(t)
	image := moderation.PendingImage{
		Fingerprint:      moderation.ImageFingerprint{SHA256: "sha256", MD5: "md5", Size: 123},
		PreviewObjectKey: "moderation/previews/sha.jpg", LastUsedAt: fixedTime,
	}
	mock.ExpectExec(`INSERT INTO moderation_image .*ON DUPLICATE KEY UPDATE.*status = IF\(status = 'approved', status, VALUES\(status\)\)`).
		WithArgs("sha256", uint64(123), "md5", moderation.ImagePending, "moderation/previews/sha.jpg", fixedTime, fixedTime, fixedTime).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repository.UpsertPendingImage(context.Background(), image)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadRevisionImagesPreservesSequence(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT .* FROM `moderation_revision_image` WHERE revision_id = .*ORDER BY seq ASC,id ASC").
		WithArgs(uint64(20)).
		WillReturnRows(sqlmock.NewRows([]string{
			"revision_id", "seq", "object_key", "sha256", "md5", "size", "media_type", "is_gif",
		}).AddRow(20, 1, "a.jpg", "sha-a", "md5-a", 10, "image/jpeg", false).
			AddRow(20, 2, "b.gif", "sha-b", "md5-b", 11, "image/gif", true))

	images, err := repository.LoadRevisionImages(context.Background(), 20)

	require.NoError(t, err)
	require.Len(t, images, 2)
	assert.Equal(t, uint(1), images[0].Seq)
	assert.Equal(t, "b.gif", images[1].ObjectKey)
	require.NoError(t, mock.ExpectationsWereMet())
}
