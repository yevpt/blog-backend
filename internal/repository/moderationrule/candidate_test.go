package moderationrule_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/repository/moderationrule"
	"gorm.io/gorm"
)

func TestClaimNextRulesetReturnsNilWithoutRecordNotFoundLog(t *testing.T) {
	observer := &gormErrorObserver{}
	repo, mock := newManagementRepositoryWithLogger(t, observer)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `moderation_ruleset` WHERE status = ? ORDER BY id ASC LIMIT ?")).
		WithArgs("building", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()

	candidate, err := repo.ClaimNextRuleset(context.Background(), moderationrule.StatusBuilding)

	require.NoError(t, err)
	assert.Nil(t, candidate)
	assert.False(t, observer.Contains(gorm.ErrRecordNotFound))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClaimNextRulesetReturnsDatabaseError(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `moderation_ruleset` WHERE status = ? ORDER BY id ASC LIMIT ?")).
		WithArgs("building", 1).
		WillReturnError(errors.New("db down"))
	mock.ExpectRollback()

	candidate, err := repo.ClaimNextRuleset(context.Background(), moderationrule.StatusBuilding)

	assert.Nil(t, candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
	require.NoError(t, mock.ExpectationsWereMet())
}
