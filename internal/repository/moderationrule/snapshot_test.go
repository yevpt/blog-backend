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
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCurrentRulesetReturnsPublishedVersion(t *testing.T) {
	repo, mock := newSnapshotRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`status`,`index_object_key`,`index_format_version`,`index_sha256`,`index_bytes` FROM `moderation_ruleset` WHERE status = ? ORDER BY id DESC LIMIT ?")).
		WithArgs("published", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "index_object_key", "index_format_version", "index_sha256", "index_bytes"}).
			AddRow(7, "published", nil, 1, nil, 4096))

	got, err := repo.CurrentRuleset(context.Background())

	require.NoError(t, err)
	assert.Equal(t, uint64(7), got.ID)
	assert.Equal(t, uint64(4096), got.IndexBytes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStreamRulesExcludesFailedCandidatesAndClosesRows(t *testing.T) {
	repo, mock := newSnapshotRepository(t)
	mock.ExpectQuery("SELECT .* FROM moderation_rule AS rule JOIN moderation_ruleset AS activation.*activation.status IN \\(\\?,\\?\\).*").
		WithArgs("published", "superseded", uint64(7), uint64(7)).
		WillReturnRows(ruleRows().AddRow(11, "keyword", "风险", "medium", "review", 10)).
		RowsWillBeClosed()
	var ids []uint64

	err := repo.StreamRules(context.Background(), 7, func(rule moderationrule.RuleRecord) error {
		ids = append(ids, rule.ID)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []uint64{11}, ids)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStreamRulesClosesRowsWhenVisitorFails(t *testing.T) {
	repo, mock := newSnapshotRepository(t)
	mock.ExpectQuery("SELECT .* FROM moderation_rule AS rule JOIN moderation_ruleset AS activation").
		WithArgs("published", "superseded", uint64(7), uint64(7)).
		WillReturnRows(ruleRows().AddRow(11, "keyword", "风险", "medium", "review", 10)).
		RowsWillBeClosed()
	wantErr := errors.New("stop")

	err := repo.StreamRules(context.Background(), 7, func(moderationrule.RuleRecord) error {
		return wantErr
	})

	assert.ErrorIs(t, err, wantErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newSnapshotRepository(t *testing.T) (moderationrule.SnapshotRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
	})
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	return moderationrule.NewRepository(gdb), mock
}

func ruleRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "rule_type", "pattern", "risk_level", "effect", "priority"})
}
