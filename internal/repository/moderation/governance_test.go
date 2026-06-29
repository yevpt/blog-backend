package moderation_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestEnsureNewProfileIsIdempotent(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectExec("INSERT INTO `user_moderation_profile` .*ON DUPLICATE KEY UPDATE `user_id`=`user_id`").
		WithArgs(uint64(42), fixedTime, fixedTime).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repository.EnsureNewProfile(context.Background(), 42, fixedTime)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadModerationProfileDefaultsMissingUserToNewActive(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `user_moderation_profile` WHERE user_id = \\?.*FOR UPDATE").
		WithArgs(uint64(42), 1).
		WillReturnRows(profileRows())
	mock.ExpectCommit()

	got, err := repository.LoadModerationProfile(context.Background(), 42, fixedTime)

	require.NoError(t, err)
	assert.Equal(t, moderation.TrustNew, got.TrustLevel)
	assert.Equal(t, moderation.SanctionActive, got.SanctionState)
	assert.Equal(t, fixedTime, got.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadModerationProfileLazilyReleasesExpiredRestrictionAndSanction(t *testing.T) {
	repository, mock := newRepository(t)
	expired := fixedTime.Add(-time.Minute)
	reason := "temporary"
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `user_moderation_profile` WHERE user_id = \\?.*FOR UPDATE").
		WithArgs(uint64(42), 1).
		WillReturnRows(profileRows().AddRow(
			42, "restricted", "auto", false, "muted", expired, reason,
			0, 1, 2, 3, 7, expired, expired, fixedTime.AddDate(0, -2, 0), fixedTime,
		))
	mock.ExpectExec("UPDATE `user_moderation_profile` SET ").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	got, err := repository.LoadModerationProfile(context.Background(), 42, fixedTime)

	require.NoError(t, err)
	assert.Equal(t, moderation.TrustNormal, got.TrustLevel)
	assert.Zero(t, got.ViolationScore)
	assert.Nil(t, got.RestrictedUntil)
	assert.Equal(t, moderation.SanctionActive, got.SanctionState)
	assert.Nil(t, got.SanctionUntil)
	assert.Nil(t, got.SanctionReason)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetAutomaticTrustDoesNotOverrideManualLock(t *testing.T) {
	repository, mock := newRepository(t)
	until := fixedTime.Add(7 * 24 * time.Hour)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `user_moderation_profile` SET .*WHERE .*manual_trust_locked = \\?").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(42), false).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	changed, err := repository.SetAutomaticTrust(context.Background(), moderation.AutomaticTrustCommand{
		UserID: 42, TrustLevel: moderation.TrustRestricted, RestrictedUntil: &until, UpdatedAt: fixedTime,
	})

	require.NoError(t, err)
	assert.False(t, changed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func profileRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"user_id", "trust_level", "trust_source", "manual_trust_locked",
		"sanction_state", "sanction_until", "sanction_reason",
		"clean_approval_streak", "corrected_count", "rejected_count", "high_risk_count",
		"violation_score", "last_violation_at", "restricted_until", "created_at", "updated_at",
	})
}
