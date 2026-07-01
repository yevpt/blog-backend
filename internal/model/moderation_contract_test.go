package model_test

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
)

func TestModerationTableNames(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "item", got: model.ModerationItem{}.TableName(), want: "moderation_item"},
		{name: "revision", got: model.ModerationRevision{}.TableName(), want: "moderation_revision"},
		{name: "revision image", got: model.ModerationRevisionImage{}.TableName(), want: "moderation_revision_image"},
		{name: "image", got: model.ModerationImage{}.TableName(), want: "moderation_image"},
		{name: "attempt", got: model.ModerationAttempt{}.TableName(), want: "moderation_attempt"},
		{name: "rule source", got: model.ModerationRuleSource{}.TableName(), want: "moderation_rule_source"},
		{name: "ruleset", got: model.ModerationRuleset{}.TableName(), want: "moderation_ruleset"},
		{name: "ruleset removal", got: model.ModerationRulesetRemoval{}.TableName(), want: "moderation_ruleset_removal"},
		{name: "rule import", got: model.ModerationRuleImport{}.TableName(), want: "moderation_rule_import"},
		{name: "rule", got: model.ModerationRule{}.TableName(), want: "moderation_rule"},
		{name: "action log", got: model.ModerationActionLog{}.TableName(), want: "moderation_action_log"},
		{name: "visible image", got: model.ModerationVisibleImage{}.TableName(), want: "moderation_visible_image"},
		{name: "user profile", got: model.UserModerationProfile{}.TableName(), want: "user_moderation_profile"},
		{name: "control", got: model.ModerationControl{}.TableName(), want: "moderation_control"},
		{name: "review email task", got: model.ModerationReviewEmailTask{}.TableName(), want: "moderation_review_email_task"},
		{name: "review email batch", got: model.ModerationReviewEmailBatch{}.TableName(), want: "moderation_review_email_batch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

func TestModerationReviewEmailMigrationDeclaresQueueContracts(t *testing.T) {
	migration, err := os.ReadFile("../../migrations/20260701_moderation_review_email.sql")
	require.NoError(t, err)
	sql := string(migration)

	batchPosition := strings.Index(sql, "CREATE TABLE `moderation_review_email_batch`")
	taskPosition := strings.Index(sql, "CREATE TABLE `moderation_review_email_task`")
	require.GreaterOrEqual(t, batchPosition, 0)
	require.Greater(t, taskPosition, batchPosition)
	assert.NotContains(t, sql, "IF NOT EXISTS")
	assert.Contains(t, sql, "UNIQUE KEY `uk_moderation_review_email_revision` (`revision_id`)")
	assert.Contains(t, sql, "KEY `idx_moderation_review_email_task_pick` (`status`, `next_attempt_at`)")
	assert.Contains(t, sql, "KEY `idx_moderation_review_email_batch_pick` (`status`, `next_attempt_at`, `lease_until`)")
	for _, constraint := range []string{
		"CONSTRAINT `fk_moderation_review_email_task_revision`",
		"CONSTRAINT `fk_moderation_review_email_task_item`",
		"CONSTRAINT `fk_moderation_review_email_task_batch`",
		"CONSTRAINT `fk_moderation_review_email_batch_recipient`",
		"CONSTRAINT `chk_moderation_review_email_task_status`",
		"CONSTRAINT `chk_moderation_review_email_batch_status`",
	} {
		assert.Contains(t, sql, constraint)
	}
	assert.Contains(t, sql, "INSERT INTO `moderation_review_email_task`")
	assert.Contains(t, sql, "WHERE r.`review_status` = 'pending'")
}

func TestModerationReviewEmailModelMatchesMigrationIntColumns(t *testing.T) {
	typ := reflect.TypeOf(model.ModerationReviewEmailBatch{})
	for _, fieldName := range []string{"ItemCount", "Attempts"} {
		field, ok := typ.FieldByName(fieldName)
		require.Truef(t, ok, "%s.%s is missing", typ.Name(), fieldName)
		assert.Equal(t, reflect.TypeOf(int(0)), field.Type)
		assert.Contains(t, field.Tag.Get("gorm"), "type:int")
	}

	migration, err := os.ReadFile("../../migrations/20260701_moderation_review_email.sql")
	require.NoError(t, err)
	assert.Contains(t, string(migration), "`item_count` int NOT NULL")
	assert.Contains(t, string(migration), "`attempts` int NOT NULL")
}

func TestModerationItemUsesExplicitDeletedAt(t *testing.T) {
	typ := reflect.TypeOf(model.ModerationItem{})
	field, ok := typ.FieldByName("DeletedAt")
	require.True(t, ok)
	assert.Equal(t, reflect.TypeOf((*time.Time)(nil)), field.Type)
	assert.NotContains(t, field.Tag.Get("gorm"), "softDelete")
}

func TestModerationControlSingletonIDDoesNotAutoIncrement(t *testing.T) {
	typ := reflect.TypeOf(model.ModerationControl{})
	field, ok := typ.FieldByName("ID")
	require.True(t, ok)
	assert.Contains(t, field.Tag.Get("gorm"), "autoIncrement:false")

	migration, err := os.ReadFile("../../migrations/20260627_content_moderation_core.sql")
	require.NoError(t, err)
	tablePattern := regexp.MustCompile("(?s)CREATE TABLE IF NOT EXISTS `moderation_control` \\((.*?)\\) ENGINE")
	match := tablePattern.FindSubmatch(migration)
	require.Len(t, match, 2)
	assert.Regexp(t, "(?m)^\\s*`id` bigint unsigned NOT NULL,$", string(match[1]))
	assert.NotContains(t, string(match[1]), "AUTO_INCREMENT")
}

func TestModerationModelsDeclareRequiredUniqueIndexes(t *testing.T) {
	assertCompositeUniqueIndex(t, reflect.TypeOf(model.ModerationItem{}), "uk_moderation_subject", "ContentType", "ContentID")
	assertCompositeUniqueIndex(t, reflect.TypeOf(model.ModerationRevision{}), "uk_moderation_revision_version", "ItemID", "Version")
	assertCompositeUniqueIndex(t, reflect.TypeOf(model.ModerationRevision{}), "uk_moderation_revision_idempotency", "SubmitterID", "IdempotencyKey")
	assertCompositeUniqueIndex(t, reflect.TypeOf(model.ModerationRevisionImage{}), "uk_moderation_revision_image_seq", "RevisionID", "Seq")
	assertCompositeUniqueIndex(t, reflect.TypeOf(model.ModerationImage{}), "uk_moderation_image_fingerprint", "SHA256", "Size")
	assertCompositeUniqueIndex(t, reflect.TypeOf(model.ModerationAttempt{}), "uk_moderation_attempt_idempotency", "UserID", "IdempotencyKey")
}

func TestModerationMediaDigestFieldsUseExactLengths(t *testing.T) {
	tests := []reflect.Type{
		reflect.TypeOf(model.ModerationRevisionImage{}),
		reflect.TypeOf(model.ModerationImage{}),
	}
	for _, typ := range tests {
		sha256Field, ok := typ.FieldByName("SHA256")
		require.Truef(t, ok, "%s.SHA256 is missing", typ.Name())
		assert.Contains(t, sha256Field.Tag.Get("gorm"), "size:64")

		md5Field, ok := typ.FieldByName("MD5")
		require.Truef(t, ok, "%s.MD5 is missing", typ.Name())
		assert.Contains(t, md5Field.Tag.Get("gorm"), "size:32")
	}
}

func TestModerationMediaMigrationDeclaresFingerprintConstraints(t *testing.T) {
	migration, err := os.ReadFile("../../migrations/20260629_content_moderation_media.sql")
	require.NoError(t, err)
	sql := string(migration)
	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS `moderation_revision_image`")
	assert.Contains(t, sql, "UNIQUE KEY `uk_moderation_revision_image_seq` (`revision_id`, `seq`)")
	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS `moderation_image`")
	assert.Contains(t, sql, "UNIQUE KEY `uk_moderation_image_fingerprint` (`sha256`, `size`)")
	assert.Contains(t, sql, "CHECK (`status` IN ('pending','approved'))")
}

func TestModerationAttemptDoesNotPersistBlockedContentOrDigest(t *testing.T) {
	typ := reflect.TypeOf(model.ModerationAttempt{})
	for _, forbidden := range []string{"Content", "SubmittedContent", "PublishedContent", "ContentDigest", "Digest", "Hash"} {
		_, ok := typ.FieldByName(forbidden)
		assert.Falsef(t, ok, "ModerationAttempt must not contain %s", forbidden)
	}
}

func TestModerationRuleUsesVersionIntervalsWithoutRedundantState(t *testing.T) {
	typ := reflect.TypeOf(model.ModerationRule{})
	for _, forbidden := range []string{"Enabled", "RulesetVersion", "NormalizedPattern", "RootRuleID", "CreatedBy"} {
		_, ok := typ.FieldByName(forbidden)
		assert.Falsef(t, ok, "%s must not exist", forbidden)
	}
	for _, required := range []string{"DedupeHash", "SourceID", "ActivatedRulesetID", "DeactivatedRulesetID", "ReplacesRuleID"} {
		_, ok := typ.FieldByName(required)
		assert.Truef(t, ok, "%s is required", required)
	}
}

func TestModerationAuditModelsPersistMatchTruncation(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(model.ModerationRevision{}),
		reflect.TypeOf(model.ModerationAttempt{}),
	} {
		field, ok := typ.FieldByName("RuleMatchesTruncated")
		require.Truef(t, ok, "%s.RuleMatchesTruncated is required", typ.Name())
		assert.Equal(t, reflect.TypeOf(false), field.Type)
		assert.Contains(t, field.Tag.Get("gorm"), "default:0")
	}
}

func TestModerationRuleManagementMigrationIsOrderedAndStrict(t *testing.T) {
	migration, err := os.ReadFile("../../migrations/20260630_moderation_rule_management.sql")
	require.NoError(t, err)
	sql := string(migration)

	steps := []string{
		"CREATE TABLE `moderation_rule_source`",
		"CREATE TABLE `moderation_ruleset`",
		"CREATE TABLE `moderation_ruleset_removal`",
		"CREATE TABLE `moderation_rule_import`",
		"INSERT INTO `moderation_rule_source`",
		"INSERT INTO `moderation_ruleset`",
		"UPDATE `moderation_rule`",
		"MODIFY COLUMN `dedupe_hash` binary(32) NOT NULL",
		"ADD COLUMN `rule_matches_truncated` tinyint(1) NOT NULL DEFAULT 0",
		"DROP COLUMN `enabled`",
		"DROP COLUMN `ruleset_version`",
	}
	position := -1
	for _, step := range steps {
		next := strings.Index(sql, step)
		require.Greaterf(t, next, position, "%s must exist in dependency order", step)
		position = next
	}

	assert.NotContains(t, sql, "IF EXISTS")
	assert.NotContains(t, sql, "IF NOT EXISTS")
	assert.Contains(t, sql, "DROP INDEX `idx_moderation_rule_snapshot`")
	assert.Contains(t, sql, "CONSTRAINT `fk_moderation_rule_source`")
	assert.Contains(t, sql, "CONSTRAINT `fk_moderation_rule_activation`")
	assert.Contains(t, sql, "CONSTRAINT `fk_moderation_rule_deactivation`")
	assert.Contains(t, sql, "CONSTRAINT `chk_moderation_rule_effect`")
	assert.Contains(t, sql, "CONSTRAINT `chk_moderation_rule_category`")
	assert.Contains(t, sql, "CONSTRAINT `chk_moderation_rule_import_category`")
}

func TestModerationRevisionPersistsMomentOptions(t *testing.T) {
	typ := reflect.TypeOf(model.ModerationRevision{})
	status, hasStatus := typ.FieldByName("MomentStatus")
	commentStatus, hasCommentStatus := typ.FieldByName("MomentCommentStatus")
	require.True(t, hasStatus)
	require.True(t, hasCommentStatus)
	assert.Equal(t, reflect.TypeOf((*uint8)(nil)), status.Type)
	assert.Equal(t, reflect.TypeOf((*uint8)(nil)), commentStatus.Type)
	migration, err := os.ReadFile("../../migrations/20260627_content_moderation_core.sql")
	require.NoError(t, err)
	assert.Contains(t, string(migration), "`moment_status` tinyint unsigned NULL")
	assert.Contains(t, string(migration), "`moment_comment_status` tinyint unsigned NULL")
}

func TestModerationClosedEnumValues(t *testing.T) {
	assert.ElementsMatch(t, []string{"active", "deleted"}, model.ModerationLifecycleStates())
	assert.ElementsMatch(t, []string{"visible", "placeholder", "hidden", "emergency_hidden"}, model.ModerationPublicStates())
	assert.ElementsMatch(t, []string{"low", "medium", "high"}, model.ModerationRiskLevels())
	assert.ElementsMatch(t, []string{"auto_approve", "post_review", "pre_review", "block"}, model.ModerationPolicyActions())
	assert.ElementsMatch(t, []string{"pending", "approved", "rejected", "superseded"}, model.ModerationReviewStatuses())
	assert.ElementsMatch(t, []string{"approved", "corrected", "rejected", "legacy_migration"}, model.ModerationDecisionTypes())
	assert.ElementsMatch(t, []string{"pending", "approved"}, model.ModerationImageStatuses())
	assert.ElementsMatch(t, []string{"new", "normal", "trusted", "restricted"}, model.ModerationTrustLevels())
	assert.ElementsMatch(t, []string{"auto", "manual"}, model.ModerationTrustSources())
	assert.ElementsMatch(t, []string{"active", "muted", "banned"}, model.ModerationSanctionStates())
	assert.ElementsMatch(t, []string{"open", "closed"}, model.ModerationRegistrationModes())
	assert.ElementsMatch(t, []string{"open", "pre_review_all", "closed"}, model.ModerationPublishingModes())
}

func assertCompositeUniqueIndex(t *testing.T, typ reflect.Type, indexName string, fields ...string) {
	t.Helper()
	for priority, fieldName := range fields {
		field, ok := typ.FieldByName(fieldName)
		require.Truef(t, ok, "%s.%s is missing", typ.Name(), fieldName)
		tag := field.Tag.Get("gorm")
		assert.Contains(t, tag, "uniqueIndex:"+indexName)
		assert.Contains(t, tag, "priority:"+string(rune('1'+priority)))
	}
}
