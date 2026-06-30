package moderationrule_test

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/repository/moderationrule"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestListRulesUsesIDCursorAndPrefixSearch(t *testing.T) {
	repo, mock := newManagementRepository(t)
	rows := ruleListRows()
	for i := 1; i <= 51; i++ {
		rows.AddRow(ruleListRowValues(uint64(100+i), "keyword", "风险"+string(rune('a'+i))+"_test")...)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT rule.id,rule.name,rule.rule_type,rule.pattern,rule.category,rule.effect,rule.risk_level,rule.priority,rule.source_id,rule.activated_ruleset_id,rule.deactivated_ruleset_id,rule.replaces_rule_id,rule.created_at,rule.updated_at FROM moderation_rule AS rule WHERE id > ? AND pattern LIKE ? AND category = ? ORDER BY id ASC LIMIT ?")).
		WithArgs(uint64(100), "风险%", "fraud", 51).
		WillReturnRows(rows)

	page, err := repo.ListRules(context.Background(), moderationrule.RuleFilter{
		AfterID: 100, Limit: 50, PatternPrefix: "风险", Category: "fraud",
	})

	require.NoError(t, err)
	assert.True(t, page.HasMore)
	assert.Equal(t, uint64(151), page.NextCursor)
	assert.Len(t, page.Rules, 50)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListRulesExactIDOverridesCursor(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT rule.id,rule.name,rule.rule_type,rule.pattern,rule.category,rule.effect,rule.risk_level,rule.priority,rule.source_id,rule.activated_ruleset_id,rule.deactivated_ruleset_id,rule.replaces_rule_id,rule.created_at,rule.updated_at FROM moderation_rule AS rule WHERE id = ? ORDER BY id ASC LIMIT ?")).
		WithArgs(uint64(42), 2).
		WillReturnRows(ruleListRows().AddRow(ruleListRowValues(42, "keyword", "测试")...))

	page, err := repo.ListRules(context.Background(), moderationrule.RuleFilter{ExactID: 42, Limit: 1})

	require.NoError(t, err)
	assert.False(t, page.HasMore)
	assert.Len(t, page.Rules, 1)
	assert.Equal(t, uint64(42), page.Rules[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListRulesExactPatternOverridesPrefix(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT rule.id,rule.name,rule.rule_type,rule.pattern,rule.category,rule.effect,rule.risk_level,rule.priority,rule.source_id,rule.activated_ruleset_id,rule.deactivated_ruleset_id,rule.replaces_rule_id,rule.created_at,rule.updated_at FROM moderation_rule AS rule WHERE pattern = ? ORDER BY id ASC LIMIT ?")).
		WithArgs("精确词", 21).
		WillReturnRows(ruleListRows().AddRow(ruleListRowValues(1, "keyword", "精确词")...))

	page, err := repo.ListRules(context.Background(), moderationrule.RuleFilter{Limit: 20, ExactPattern: "精确词", PatternPrefix: "忽略"})

	require.NoError(t, err)
	assert.False(t, page.HasMore)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListRulesActiveFilterUsesCurrentPublishedVersion(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id` FROM `moderation_ruleset` WHERE status = ? ORDER BY id DESC LIMIT ?")).
		WithArgs("published", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT rule.id,rule.name,rule.rule_type,rule.pattern,rule.category,rule.effect,rule.risk_level,rule.priority,rule.source_id,rule.activated_ruleset_id,rule.deactivated_ruleset_id,rule.replaces_rule_id,rule.created_at,rule.updated_at FROM moderation_rule AS rule WHERE activated_ruleset_id <= ? AND deactivated_ruleset_id IS NULL ORDER BY id ASC LIMIT ?")).
		WithArgs(uint64(7), 11).
		WillReturnRows(ruleListRows().AddRow(ruleListRowValues(1, "keyword", "活动")...))

	active := true
	page, err := repo.ListRules(context.Background(), moderationrule.RuleFilter{Limit: 10, Active: &active})

	require.NoError(t, err)
	assert.True(t, page.Rules[0].Active)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListRulesInactiveFilterExcludesActiveRules(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id` FROM `moderation_ruleset` WHERE status = ? ORDER BY id DESC LIMIT ?")).
		WithArgs("published", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT rule.id,rule.name,rule.rule_type,rule.pattern,rule.category,rule.effect,rule.risk_level,rule.priority,rule.source_id,rule.activated_ruleset_id,rule.deactivated_ruleset_id,rule.replaces_rule_id,rule.created_at,rule.updated_at FROM moderation_rule AS rule WHERE deactivated_ruleset_id <= ? ORDER BY id ASC LIMIT ?")).
		WithArgs(uint64(7), 11).
		WillReturnRows(ruleListRows().AddRow(ruleListRowValues(2, "keyword", "已停用")...))

	inactive := false
	page, err := repo.ListRules(context.Background(), moderationrule.RuleFilter{Limit: 10, Active: &inactive})

	require.NoError(t, err)
	assert.False(t, page.Rules[0].Active)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListSourcesReturnsAllSources(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,name,created_at FROM `moderation_rule_source` ORDER BY id ASC")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at"}).
			AddRow(1, "手工维护", time.Now()).
			AddRow(2, "采购词库", time.Now()))

	sources, err := repo.ListSources(context.Background())

	require.NoError(t, err)
	assert.Len(t, sources, 2)
	assert.Equal(t, "手工维护", sources[0].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureSourceCreatesNewSource(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`name`,`created_at` FROM `moderation_rule_source` WHERE name = ? ORDER BY id ASC LIMIT ?")).
		WithArgs("新来源", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at"}))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `moderation_rule_source`").
		WithArgs("新来源", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(3, 1))
	mock.ExpectCommit()

	source, err := repo.EnsureSource(context.Background(), "新来源")

	require.NoError(t, err)
	assert.Equal(t, uint64(3), source.ID)
	assert.Equal(t, "新来源", source.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureSourceReturnsExistingSource(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`name`,`created_at` FROM `moderation_rule_source` WHERE name = ? ORDER BY id ASC LIMIT ?")).
		WithArgs("已有来源", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at"}).AddRow(5, "已有来源", time.Now()))

	source, err := repo.EnsureSource(context.Background(), "已有来源")

	require.NoError(t, err)
	assert.Equal(t, uint64(5), source.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFindDuplicateHashesReturnsExistingRuleIDs(t *testing.T) {
	repo, mock := newManagementRepository(t)
	hash1 := moderationrule.DedupeHash{0x01}
	hash2 := moderationrule.DedupeHash{0x02}
	mock.ExpectQuery("dedupe_hash IN \\(\\?,\\?\\).*activated_ruleset_id <= \\?").
		WithArgs(hash1[:], hash2[:], uint64(7), uint64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"dedupe_hash", "id"}).
			AddRow(hash1[:], 42))

	dupes, err := repo.FindDuplicateHashes(context.Background(), 7, []moderationrule.DedupeHash{hash1, hash2})

	require.NoError(t, err)
	assert.Len(t, dupes, 1)
	assert.Equal(t, uint64(42), dupes[hash1])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFindDuplicateHashesChunksLargeBatches(t *testing.T) {
	repo, mock := newManagementRepository(t)
	hashes := make([]moderationrule.DedupeHash, 0, 2500)
	for i := 0; i < 2500; i++ {
		hashes = append(hashes, moderationrule.DedupeHash{byte(i % 256), byte(i / 256), byte(i % 7)})
	}
	for chunk := 0; chunk < 3; chunk++ {
		mock.ExpectQuery("dedupe_hash IN").
			WillReturnRows(sqlmock.NewRows([]string{"dedupe_hash", "id"}))
	}

	dupes, err := repo.FindDuplicateHashes(context.Background(), 7, hashes)

	require.NoError(t, err)
	assert.Empty(t, dupes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCurrentStatusReturnsPublishedAndCandidate(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`rule_count`,`keyword_count`,`regexp_count`,`composite_count`,`index_bytes`,`build_peak_bytes`,`build_duration_ms`,`index_object_key`,`index_sha256`,`updated_at` FROM `moderation_ruleset` WHERE status = ? ORDER BY id DESC LIMIT ?")).
		WithArgs("published", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "rule_count", "keyword_count", "regexp_count", "composite_count", "index_bytes", "build_peak_bytes", "build_duration_ms", "index_object_key", "index_sha256", "updated_at"}).
			AddRow(7, 1000, 950, 30, 20, 4096, 8192, 500, "moderation/rulesets/7.bin", "abc123", time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`status`,`base_ruleset_id`,`rule_count`,`keyword_count`,`regexp_count`,`composite_count`,`index_bytes`,`build_peak_bytes`,`failure_code`,`created_at`,`updated_at` FROM `moderation_ruleset` WHERE status IN (?,?) ORDER BY id DESC LIMIT ?")).
		WithArgs("building", "ready", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "base_ruleset_id", "rule_count", "keyword_count", "regexp_count", "composite_count", "index_bytes", "build_peak_bytes", "failure_code", "created_at", "updated_at"}).
			AddRow(8, "ready", 7, 1100, 1040, 35, 25, 4500, 9000, nil, time.Now(), time.Now()))

	status, err := repo.CurrentStatus(context.Background())

	require.NoError(t, err)
	assert.Equal(t, uint64(7), status.CurrentRulesetID)
	assert.Equal(t, uint64(1000), status.RuleCount)
	require.NotNil(t, status.Candidate)
	assert.Equal(t, uint64(8), status.Candidate.RulesetID)
	assert.Equal(t, "ready", status.Candidate.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCurrentStatusNoCandidateReturnsOnlyPublished(t *testing.T) {
	repo, mock := newManagementRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`rule_count`,`keyword_count`,`regexp_count`,`composite_count`,`index_bytes`,`build_peak_bytes`,`build_duration_ms`,`index_object_key`,`index_sha256`,`updated_at` FROM `moderation_ruleset` WHERE status = ? ORDER BY id DESC LIMIT ?")).
		WithArgs("published", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "rule_count", "keyword_count", "regexp_count", "composite_count", "index_bytes", "build_peak_bytes", "build_duration_ms", "index_object_key", "index_sha256", "updated_at"}).
			AddRow(7, 1000, 950, 30, 20, 4096, 8192, 500, nil, nil, time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`status`,`base_ruleset_id`,`rule_count`,`keyword_count`,`regexp_count`,`composite_count`,`index_bytes`,`build_peak_bytes`,`failure_code`,`created_at`,`updated_at` FROM `moderation_ruleset` WHERE status IN (?,?) ORDER BY id DESC LIMIT ?")).
		WithArgs("building", "ready", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "base_ruleset_id", "rule_count", "keyword_count", "regexp_count", "composite_count", "index_bytes", "build_peak_bytes", "failure_code", "created_at", "updated_at"}))

	status, err := repo.CurrentStatus(context.Background())

	require.NoError(t, err)
	assert.Equal(t, uint64(7), status.CurrentRulesetID)
	assert.Nil(t, status.Candidate)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newManagementRepository(t *testing.T) (moderationrule.ManagementRepository, sqlmock.Sqlmock) {
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

func ruleListRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "rule_type", "pattern", "category", "effect", "risk_level", "priority", "source_id", "activated_ruleset_id", "deactivated_ruleset_id", "replaces_rule_id", "created_at", "updated_at"})
}

func ruleListRowValues(id uint64, ruleType, pattern string) []driver.Value {
	return []driver.Value{id, nil, ruleType, pattern, "fraud", "review", "medium", int32(100), uint64(1), uint64(1), nil, nil, time.Now(), time.Now()}
}
