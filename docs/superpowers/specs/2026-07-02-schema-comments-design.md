# 全库中文注释补齐设计

## 背景

当前数据库大量表与字段缺少中文注释，DataGrip 展开 DDL 时难以理解业务含义。开发库审计显示多数表没有 `TABLE_COMMENT`，大量列没有 `COLUMN_COMMENT`。此外，DataGrip 2025.1 在展示 MySQL `CHECK` 约束时可能把 `CHECK_CONSTRAINTS.CHECK_CLAUSE` 中的字符集字面量转义成 `_utf8mb4\'pending\'`，导致本地 DDL 解析报错；数据库侧 `SHOW CREATE TABLE` 本身是合法的。

新增的 `moderation_review_email_batch`、`moderation_review_email_task` 也属于本次注释补齐范围。

## 目标

- 所有业务表都有中文表注释。
- 所有真实业务列都有中文列注释，包括通用列 `id`、`created_at`、`updated_at`、`deleted_at` 等。
- 已存在的生产库、开发库通过同一份版本化 SQL 迁移补齐注释。
- 新建库通过当前 Go 模型和 dbschema 初始化流程也能得到相同注释。
- 保留现有表结构、索引、外键、`CHECK` 约束和业务行为，不为绕过 DataGrip 展示问题把 `CHECK` 改成 `ENUM` 或应用层约束。

## 方案

采用“版本化 SQL + 模型注释同步 + 完整性校验”。

1. 新增一份注释专用迁移，例如 `migrations/20260702_schema_comments.sql`。
   - 表注释使用 `ALTER TABLE <table> COMMENT = '<中文说明>'`。
   - 列注释使用 `ALTER TABLE <table> MODIFY COLUMN <column> <当前完整定义> COMMENT '<中文说明>'`。
   - `MODIFY COLUMN` 必须按当前 schema 精确保留类型、是否为空、默认值、自增、字符集语义等，仅增加或替换 `COMMENT`。

2. 同步 Go 模型字段注释。
   - 给 GORM 字段补 `comment:` 标签，确保 `dbsetup` / `AutoMigrate` 创建的新库天然带列注释。
   - 对 `BaseModel` 等嵌入字段补通用注释，避免每个模型重复维护。
   - 对审核邮件两张新表一并补齐：任务表说明待审核修订邮件聚合状态，批次表说明一封可租用、可重试的审核摘要邮件。

3. 补表注释来源。
   - GORM 字段标签只能稳定覆盖列注释，表注释单独在 `internal/dbschema` 维护一份表名到中文说明的 catalog。
   - `dbschema.AutoMigrate` 完成后调用表注释应用函数，对新建库执行 `ALTER TABLE ... COMMENT`。
   - 不使用全局 DB；沿用现有 `AutoMigrate(db *gorm.DB)` 注入。

4. 增加校验。
   - 新增或扩展测试，校验注册在 `dbschema` 的所有模型都有表注释。
   - 校验模型字段或注释 catalog 覆盖所有应建列，防止后续新增字段忘记写中文注释。
   - 保留现有 moderation email migration contract 测试，并覆盖两张新增审核邮件表的注释约束。

## DataGrip 处理

DataGrip 报错的核心不是数据库 DDL 损坏，而是 2025.1 展示/解析 `CHECK` 约束时使用了带反斜杠转义的表达式。迁移不改变 `CHECK` 约束，只补注释。落地后建议在 DataGrip 尝试重新同步 schema、提高 MySQL introspection level，或升级到修复版本；这属于客户端展示问题。

## 验证

- 跑相关 Go 测试，至少覆盖 `internal/dbschema`、`internal/model`，必要时跑 `go test ./...`。
- 在开发库执行注释迁移后，通过 `information_schema.TABLES` 与 `information_schema.COLUMNS` 抽样确认表注释、列注释已写入。
- 对 `SHOW CREATE TABLE moderation_image` 复查：确认约束仍保留，注释不会改变 `CHECK` 行为。

## 风险与约束

- MySQL 的 `ALTER TABLE ... MODIFY COLUMN` 即使只改注释，也可能触发表元数据变更；生产执行前需要常规备份和低峰窗口。
- 注释迁移是元数据变更，不迁移业务数据，不改索引、外键或约束。
- 迁移文件要作为生产和开发共用的唯一来源，避免手工在生产库单独补注释造成漂移。
