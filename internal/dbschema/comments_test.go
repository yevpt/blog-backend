package dbschema

import (
	"os"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
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

func TestBuildSchemaCommentSQL(t *testing.T) {
	comments := SchemaCommentSet{Tables: map[string]TableComment{
		"moderation_review_email_task": {
			Comment: "审核邮件任务表",
			Columns: map[string]string{"id": "主键ID"},
		},
	}}
	defs := map[string]map[string]string{
		"moderation_review_email_task": {
			"id": "`id` bigint unsigned NOT NULL AUTO_INCREMENT",
		},
	}

	statements, err := buildSchemaCommentSQL(comments, defs)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"ALTER TABLE `moderation_review_email_task` COMMENT = '审核邮件任务表'",
		"ALTER TABLE `moderation_review_email_task` MODIFY COLUMN `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID'",
	}, statements)
}

func TestBuildSchemaCommentSQLSortsAndQuotesIdentifiers(t *testing.T) {
	comments := SchemaCommentSet{Tables: map[string]TableComment{
		"b`table": {
			Comment: "表'注释",
			Columns: map[string]string{
				"z": "末列",
				"a": "首列",
			},
		},
		"a_table": {
			Comment: "首表",
			Columns: map[string]string{},
		},
	}}
	defs := map[string]map[string]string{
		"b`table": {
			"a": "`a` varchar(20) NOT NULL",
			"z": "`z` bigint NOT NULL",
		},
	}

	statements, err := buildSchemaCommentSQL(comments, defs)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"ALTER TABLE `a_table` COMMENT = '首表'",
		"ALTER TABLE `b``table` COMMENT = '表''注释'",
		"ALTER TABLE `b``table` MODIFY COLUMN `a` varchar(20) NOT NULL COMMENT '首列'",
		"ALTER TABLE `b``table` MODIFY COLUMN `z` bigint NOT NULL COMMENT '末列'",
	}, statements)
}

func TestBuildSchemaCommentSQLRejectsMissingColumnDefinition(t *testing.T) {
	comments := SchemaCommentSet{Tables: map[string]TableComment{
		"article": {
			Comment: "文章表",
			Columns: map[string]string{"id": "主键ID"},
		},
	}}

	statements, err := buildSchemaCommentSQL(comments, nil)

	require.EqualError(t, err, "missing column definition for article.id")
	assert.Nil(t, statements)
}

func TestLoadColumnDefinitions(t *testing.T) {
	db, mock, cleanup := newCommentTestDB(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{
		"TABLE_NAME",
		"COLUMN_NAME",
		"COLUMN_TYPE",
		"IS_NULLABLE",
		"COLUMN_DEFAULT",
		"EXTRA",
		"GENERATION_EXPRESSION",
		"CHARACTER_SET_NAME",
		"COLLATION_NAME",
	}).
		AddRow("article", "id", "bigint unsigned", "NO", nil, "auto_increment", "", nil, nil).
		AddRow("article", "title", "varchar(255)", "NO", "管理员's article", "", "", "utf8mb4", "utf8mb4_0900_ai_ci").
		AddRow("article", "published_at", "datetime(3)", "YES", nil, "", "", nil, nil).
		AddRow("article", "slug", "varchar(255)", "YES", nil, "STORED GENERATED", "lower(`title`)", "utf8mb4", "utf8mb4_0900_ai_ci").
		AddRow("article", "read_count", "int unsigned", "NO", "0", "", "", nil, nil)
	mock.ExpectQuery(regexp.QuoteMeta(columnDefinitionsQuery)).WillReturnRows(rows)

	definitions, err := loadColumnDefinitions(db)

	require.NoError(t, err)
	assert.Equal(t, "`id` bigint unsigned NOT NULL AUTO_INCREMENT", definitions["article"]["id"])
	assert.Equal(t, "`title` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT '管理员''s article'", definitions["article"]["title"])
	assert.Equal(t, "`published_at` datetime(3) NULL DEFAULT NULL", definitions["article"]["published_at"])
	assert.Equal(t, "`slug` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci GENERATED ALWAYS AS (lower(`title`)) STORED", definitions["article"]["slug"])
	// unsigned 数值类型的默认值必须是不加引号的 DEFAULT 0，而非字符串 DEFAULT '0'
	assert.Equal(t, "`read_count` int unsigned NOT NULL DEFAULT 0", definitions["article"]["read_count"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// 仅校验每个目录表/列在迁移文件中存在对应语句，不校验语句（如 DEFAULT 子句）本身是否正确。
func TestSchemaCommentsMigrationCoversCatalog(t *testing.T) {
	sqlBytes, err := os.ReadFile("../../migrations/20260702_schema_comments.sql")
	require.NoError(t, err)
	sql := string(sqlBytes)

	for table, tc := range SchemaComments().Tables {
		assert.Contains(t, sql, "ALTER TABLE `"+table+"` COMMENT")
		for column := range tc.Columns {
			assert.Contains(t, sql, "ALTER TABLE `"+table+"` MODIFY COLUMN `"+column+"`")
		}
	}
}

func newCommentTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	return db, mock, func() {
		mock.ExpectClose()
		require.NoError(t, sqlDB.Close())
	}
}
