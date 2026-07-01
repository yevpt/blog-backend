# Schema Comments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给当前所有业务表和字段补齐中文注释，并确保生产迁移与新库初始化使用同一套注释。

**Architecture:** 在 `internal/dbschema` 维护 schema 注释 catalog，`AutoMigrate` 后应用表/列注释；生产和开发已有库通过独立 SQL 迁移补齐同一批注释。测试负责阻止后续新增表或字段时忘记补中文注释。

**Tech Stack:** Go 1.25+、GORM/MySQL、MySQL `ALTER TABLE ... COMMENT`、`information_schema`、Go test。

## Global Constraints

- 不改业务数据、索引、外键、`CHECK` 约束和状态枚举。
- 不使用全局 DB；沿用 `dbschema.AutoMigrate(db *gorm.DB)` 参数注入。
- 迁移文件必须能用于生产库和开发库，同一份 SQL 不做环境分支。
- `moderation_review_email_batch`、`moderation_review_email_task` 必须纳入注释范围。
- DataGrip 2025.1 的 `CHECK` 展示问题不通过改变数据库约束绕开。

---

## File Structure

- Create: `internal/dbschema/comments.go` — 注释 catalog、SQL 生成、AutoMigrate 后应用函数。
- Create: `internal/dbschema/comments_test.go` — catalog 覆盖率、SQL 转义、迁移一致性测试。
- Modify: `internal/dbschema/schema.go` — `AutoMigrate` 成功后调用 `ApplySchemaComments`。
- Create: `migrations/20260702_schema_comments.sql` — 已有库补齐注释的版本化迁移。
- Modify: `internal/model/moderation_contract_test.go` — 保持审核邮件表契约，并确认新增迁移被纳入注释范围。

---

### Task 1: 建立注释 catalog 与失败测试

**Files:**
- Create: `internal/dbschema/comments_test.go`
- Create: `internal/dbschema/comments.go`
- Modify: `internal/dbschema/schema.go`

**Interfaces:**
- Produces: `func SchemaComments() SchemaCommentSet`
- Produces: `type SchemaCommentSet struct { Tables map[string]TableComment }`
- Produces: `type TableComment struct { Comment string; Columns map[string]string }`
- Produces: `func coreModels() []any`

- [ ] **Step 1: 写失败测试**

Create `internal/dbschema/comments_test.go` with tests that require all registered models to have table comments and required columns:

```go
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
```

- [ ] **Step 2: 跑测试确认失败**

Run:

```bash
go test ./internal/dbschema -run 'TestSchemaComments' -count=1
```

Expected: FAIL because `SchemaComments` and `RegisteredTableNames` are not defined.

- [ ] **Step 3: 加最小 catalog 类型**

Create `internal/dbschema/comments.go`:

```go
package dbschema

type SchemaCommentSet struct {
	Tables map[string]TableComment
}

type TableComment struct {
	Comment string
	Columns map[string]string
}

func SchemaComments() SchemaCommentSet {
	return SchemaCommentSet{Tables: schemaComments}
}

func RegisteredTableNames() []string {
	models := append([]any{}, coreModels()...)
	models = append(models, moderationModels()...)
	names := make([]string, 0, len(models))
	for _, m := range models {
		if named, ok := m.(interface{ TableName() string }); ok {
			names = append(names, named.TableName())
		}
	}
	return names
}

func quoteSQLComment(comment string) string {
	escaped := strings.ReplaceAll(comment, "'", "''")
	return "'" + escaped + "'"
}
```

Then add `import "strings"` and an empty package-level `schemaComments` map so the compile failure becomes an assertion failure.

- [ ] **Step 4: 拆出 coreModels**

Modify `internal/dbschema/schema.go` so the existing non-moderation model list moves into:

```go
func coreModels() []any {
	return []any{
		&model.Role{},
		&model.User{},
		// keep every existing non-moderation model here in the same order
		&model.AnalyticsFriendLinkDaily{},
	}
}
```

Then change the first `AutoMigrate` call to `db.AutoMigrate(coreModels()...)`. Keep the existing `moderationModels()` function unchanged.

- [ ] **Step 5: 跑测试确认失败点变为缺注释**

Run:

```bash
go test ./internal/dbschema -run 'TestSchemaComments' -count=1
```

Expected: FAIL with `missing table comment`.

---

### Task 2: 拆出模型注册列表并补全注释 catalog

**Files:**
- Modify: `internal/dbschema/comments.go`

**Interfaces:**
- Produces: complete `schemaComments` for all tables in `coreModels()` and `moderationModels()`

- [ ] **Step 1: 接入 ApplySchemaComments 调用点占位**

Modify `internal/dbschema/schema.go` so `AutoMigrate` keeps the two migration phases and can later end with:

```go
if err := db.AutoMigrate(coreModels()...); err != nil {
	return err
}
if err := db.AutoMigrate(moderationModels()...); err != nil {
	return err
}
return ApplySchemaComments(db)
```

If `ApplySchemaComments` is not implemented yet, add a temporary no-op stub in `internal/dbschema/comments.go`:

```go
func ApplySchemaComments(_ *gorm.DB) error {
	return nil
}
```

and import `gorm.io/gorm`.

- [ ] **Step 2: 补完整中文注释 map**

In `internal/dbschema/comments.go`, populate `schemaComments` with one `TableComment` per registered table.

Rules:

- 表注释写业务用途，例如 `moderation_review_email_batch` = `审核摘要邮件批次表`。
- 字段注释写“这个字段在本表里的含义”，例如 `moderation_review_email_task.revision_id` = `待审核内容修订ID`。
- 通用字段统一：`id` = `主键ID`，`created_at` = `创建时间`，`updated_at` = `更新时间`，`deleted_at` = `软删除时间`。
- 新增审核邮件两表必须至少包含迁移中所有字段：
  - `moderation_review_email_batch`: `id`, `recipient_user_id`, `to_email`, `subject`, `status`, `item_count`, `scheduled_at`, `sent_at`, `attempts`, `next_attempt_at`, `lease_until`, `locked_by`, `message_id`, `last_error`, `created_at`, `updated_at`
  - `moderation_review_email_task`: `id`, `revision_id`, `item_id`, `status`, `available_at`, `next_attempt_at`, `batch_id`, `created_at`, `updated_at`

- [ ] **Step 3: 跑覆盖率测试**

Run:

```bash
go test ./internal/dbschema -run 'TestSchemaComments' -count=1
```

Expected: PASS.

---

### Task 3: 应用注释到新建库

**Files:**
- Modify: `internal/dbschema/comments.go`
- Modify: `internal/dbschema/comments_test.go`
- Modify: `internal/dbschema/schema.go`

**Interfaces:**
- Produces: `func ApplySchemaComments(db *gorm.DB) error`
- Produces: `func BuildSchemaCommentSQL(tableDefinitions map[string]map[string]string) ([]string, error)`
- Produces: `func buildSchemaCommentSQL(comments SchemaCommentSet, tableDefinitions map[string]map[string]string) ([]string, error)`

- [ ] **Step 1: 写 SQL 生成测试**

Append tests to `internal/dbschema/comments_test.go`:

```go
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

	sql, err := buildSchemaCommentSQL(comments, defs)

	require.NoError(t, err)
	assert.Contains(t, sql, "ALTER TABLE `moderation_review_email_task` COMMENT = '审核邮件任务表'")
	assert.Contains(t, sql, "ALTER TABLE `moderation_review_email_task` MODIFY COLUMN `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID'")
}
```

- [ ] **Step 2: 实现 SQL 生成与应用**

In `internal/dbschema/comments.go`, add:

```go
func BuildSchemaCommentSQL(tableDefinitions map[string]map[string]string) ([]string, error) {
	return buildSchemaCommentSQL(SchemaComments(), tableDefinitions)
}

func buildSchemaCommentSQL(comments SchemaCommentSet, tableDefinitions map[string]map[string]string) ([]string, error) {
	statements := make([]string, 0)

	for _, table := range sortedTableNames(comments.Tables) {
		tc := comments.Tables[table]
		statements = append(statements, "ALTER TABLE `"+table+"` COMMENT = "+quoteSQLComment(tc.Comment))
		for _, column := range sortedColumnNames(tc.Columns) {
			definition, ok := tableDefinitions[table][column]
			if !ok {
				return nil, fmt.Errorf("missing column definition for %s.%s", table, column)
			}
			statements = append(statements, "ALTER TABLE `"+table+"` MODIFY COLUMN "+definition+" COMMENT "+quoteSQLComment(tc.Columns[column]))
		}
	}

	return statements, nil
}

func ApplySchemaComments(db *gorm.DB) error {
	definitions, err := loadColumnDefinitions(db)
	if err != nil {
		return err
	}
	statements, err := BuildSchemaCommentSQL(definitions)
	if err != nil {
		return err
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
```

Implement `loadColumnDefinitions` from `information_schema.COLUMNS`, reconstructing exact column definition fields required by MySQL: column name, type, nullability, default, extra, generated expression when present, and existing charset/collation only when MySQL requires it.

- [ ] **Step 3: 接入 AutoMigrate**

Modify `internal/dbschema/schema.go` so successful migration ends with `return ApplySchemaComments(db)`.

- [ ] **Step 4: 跑 dbschema 测试**

Run:

```bash
go test ./internal/dbschema -count=1
```

Expected: PASS.

---

### Task 4: 生成生产共用注释迁移

**Files:**
- Create: `migrations/20260702_schema_comments.sql`
- Modify: `internal/dbschema/comments_test.go`

**Interfaces:**
- Consumes: `SchemaComments()`

- [ ] **Step 1: 从开发库读取当前 column definitions**

Use the configured local MySQL development database and query `information_schema.COLUMNS` for the current schema. Do not print credentials.

The generated migration must contain:

```sql
ALTER TABLE `<table>` COMMENT = '<中文表注释>';
ALTER TABLE `<table>` MODIFY COLUMN `<column>` <current full definition> COMMENT '<中文列注释>';
```

for every table and column in `SchemaComments()`.

- [ ] **Step 2: 写迁移一致性测试**

Append to `internal/dbschema/comments_test.go`:

```go
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
```

- [ ] **Step 3: 跑迁移覆盖测试**

Run:

```bash
go test ./internal/dbschema -run 'TestSchemaCommentsMigrationCoversCatalog' -count=1
```

Expected: PASS.

---

### Task 5: 最终验证

**Files:**
- Modify only if verification exposes a defect.

- [ ] **Step 1: 跑相关测试**

Run:

```bash
go test ./internal/dbschema ./internal/model -count=1
```

Expected: PASS.

- [ ] **Step 2: 跑全量测试**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 3: 开发库应用迁移并抽样确认**

Apply `migrations/20260702_schema_comments.sql` to the local development database, then query:

```sql
SELECT TABLE_NAME, TABLE_COMMENT
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME IN ('moderation_image','moderation_review_email_batch','moderation_review_email_task');

SELECT TABLE_NAME, COLUMN_NAME, COLUMN_COMMENT
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME IN ('moderation_image','moderation_review_email_batch','moderation_review_email_task')
  AND COLUMN_NAME IN ('status','recipient_user_id','revision_id');
```

Expected: all returned comments are non-empty Chinese comments.

- [ ] **Step 4: 复查 DataGrip 相关约束未被改写**

Run:

```sql
SHOW CREATE TABLE moderation_image;
```

Expected: `chk_moderation_image_status` still exists and still uses `CHECK`.
