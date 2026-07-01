package dbschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaCommentsCoverRegisteredTables(t *testing.T) {
	comments := SchemaComments()
	require.NotEmpty(t, comments.Tables)

	for _, table := range RegisteredTableNames() {
		tc, ok := comments.Tables[table]
		require.Truef(t, ok, "missing table comment for %s", table)
		assert.NotEmptyf(t, tc.Comment, "empty table comment for %s", table)
		assert.NotEmptyf(t, tc.Columns, "missing column comments for %s", table)
	}
}

func TestSchemaCommentsIncludeModerationReviewEmailTables(t *testing.T) {
	comments := SchemaComments()

	require.Contains(t, comments.Tables, "moderation_review_email_batch")
	require.Contains(t, comments.Tables, "moderation_review_email_task")
	assert.Contains(t, comments.Tables["moderation_review_email_batch"].Columns, "recipient_user_id")
	assert.Contains(t, comments.Tables["moderation_review_email_task"].Columns, "revision_id")
}

func TestQuoteSQLCommentEscapesSingleQuote(t *testing.T) {
	assert.Equal(t, "'管理员''备注'", quoteSQLComment("管理员'备注"))
}
