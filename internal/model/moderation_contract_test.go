package model_test

import (
	"os"
	"reflect"
	"regexp"
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
		{name: "attempt", got: model.ModerationAttempt{}.TableName(), want: "moderation_attempt"},
		{name: "rule", got: model.ModerationRule{}.TableName(), want: "moderation_rule"},
		{name: "action log", got: model.ModerationActionLog{}.TableName(), want: "moderation_action_log"},
		{name: "visible image", got: model.ModerationVisibleImage{}.TableName(), want: "moderation_visible_image"},
		{name: "user profile", got: model.UserModerationProfile{}.TableName(), want: "user_moderation_profile"},
		{name: "control", got: model.ModerationControl{}.TableName(), want: "moderation_control"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
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
	assertCompositeUniqueIndex(t, reflect.TypeOf(model.ModerationAttempt{}), "uk_moderation_attempt_idempotency", "UserID", "IdempotencyKey")
}

func TestModerationAttemptDoesNotPersistBlockedContentOrDigest(t *testing.T) {
	typ := reflect.TypeOf(model.ModerationAttempt{})
	for _, forbidden := range []string{"Content", "SubmittedContent", "PublishedContent", "ContentDigest", "Digest", "Hash"} {
		_, ok := typ.FieldByName(forbidden)
		assert.Falsef(t, ok, "ModerationAttempt must not contain %s", forbidden)
	}
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
