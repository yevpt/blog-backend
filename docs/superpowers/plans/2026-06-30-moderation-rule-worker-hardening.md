# Moderation Rule and Worker Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复规则真实启停状态、三类规则导入、错误报告下载和空轮询日志噪声。

**Architecture:** 规则 active 由 repository 按当前 published ruleset 推导；导入文件由 service 持有对象存储编排，handler 仅绑定 multipart；parser 生成统一 RuleInput 并复用单条规则校验。worker 的空队列通过零行查询表达 idle，不生成 GORM 错误。

**Tech Stack:** Go 1.25、Gin multipart、GORM/MySQL、Garage 流式对象、encoding/csv、RE2、go-sqlmock、gomock、httptest。

## Global Constraints

- CSV 支持 `name,rule_type,pattern,category,effect,risk_level,priority`。
- TXT 仅导入关键词。
- `allow` 仅支持 keyword；regexp 必须可编译；composite 至少两个 `&&` 信号。
- 导入任一行失败时整批不可发布。
- 候选 ruleset 发布前不得改变当前 active 投影。

---

### Task 1: 规则 active 真实投影

**Files:**
- Modify: `internal/repository/moderationrule/query.go`
- Test: `internal/repository/moderationrule/query_test.go`
- Modify: `internal/dbschema/seed.go`
- Modify: `migrations/20260627_content_moderation_core.sql`

- [ ] **Step 1: 写失败测试**

覆盖默认无 active 筛选时一条启用、一条已停用；覆盖 `deactivated_ruleset_id > currentVersion` 在候选发布前仍启用。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/repository/moderationrule -run TestListRulesComputesActive -count=1`

Expected: FAIL，当前 `rec.Active` 保持 Go 零值。

- [ ] **Step 3: 实现**

所有列表请求先读取 current published ID，并按下式计算：

```go
rec.Active = rec.ActivatedRulesetID <= currentVersion &&
    (rec.DeactivatedRulesetID == nil || *rec.DeactivatedRulesetID > currentVersion)
```

active=true SQL 使用 `deactivated_ruleset_id IS NULL OR deactivated_ruleset_id > ?`。移除正式 seed 中的“停用规则示例”，保留一条启用安全基线。

- [ ] **Step 4: 验证并提交**

Run: `go test ./internal/repository/moderationrule ./internal/dbschema -count=1`，Expected: PASS。

```bash
git add internal/repository/moderationrule internal/dbschema/seed.go migrations/20260627_content_moderation_core.sql
git commit -m "fix(moderation): 修正规则启停状态投影"
```

### Task 2: CSV 三类规则 parser

**Files:**
- Modify: `internal/service/moderationrule/import_parse.go`
- Modify: `internal/service/moderationrule/import_validate.go`
- Modify: `internal/service/moderationrule/template.go`
- Test: `internal/service/moderationrule/import_parse_test.go`
- Test: `internal/service/moderationrule/template_test.go`

**Interfaces:**
- Produces `ParsedRow{Name *string, RuleType, Pattern, Category, Effect, RiskLevel string, Priority int32}`。

- [ ] **Step 1: 写失败测试**

用一份 CSV 同时包含 keyword、regexp、composite；断言名称和优先级解析、默认 rule_type、非法正则、单信号 composite、regexp allow、非法 priority 均生成精确行号错误。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/service/moderationrule -run 'TestParseCSV.*RuleType|TestCSVTemplate' -count=1`

Expected: FAIL，当前 ParsedRow 没有 rule type/name 且 priority 未解析。

- [ ] **Step 3: 实现统一校验**

`validateParsedRow` 将 ParsedRow 转为 `RuleInput` 并调用现有 `validateRuleInput`；去重摘要使用 `computeDedupeHash(row.Effect, row.RuleType, row.Pattern)`；候选 RuleDraft 保留每行 RuleType/Name/Priority。

- [ ] **Step 4: 修正模板并验证**

CSV 模板表头前写三类注释示例，正式表头后不写数据行。Run: `go test ./internal/service/moderationrule -count=1`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/service/moderationrule
git commit -m "fix(moderation): 补齐三类规则导入校验"
```

### Task 3: multipart 上传与对象补偿

**Files:**
- Modify: `internal/service/moderationrule/types.go`
- Modify: `internal/service/moderationrule/manager.go`
- Modify: `internal/service/moderationrule/import_worker.go`
- Modify: `internal/handler/moderation/rule_download.go`
- Modify: `internal/handler/moderation/moderation.go`
- Modify: `internal/router/moderation.go`
- Test: `internal/service/moderationrule/import_worker_test.go`
- Test: `internal/handler/moderation/rules_test.go`

**Interfaces:**
- Produces `CreateImportInput{FileName, Format string; FileSize uint64; Body io.Reader; ...}`；对象 key 只由 service 生成。

- [ ] **Step 1: 写失败测试**

handler 覆盖真实 multipart、缺文件、空文件、格式/扩展名不一致、超过 `MaxImportFileMB`；service 覆盖 Put 成功/CreateImport 失败时 DeleteObject 补偿。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/handler/moderation ./internal/service/moderationrule -run 'TestCreateImport' -count=1`

Expected: FAIL，当前 handler 用 Query 绑定并未上传文件。

- [ ] **Step 3: 实现**

handler 使用 `c.ShouldBind` 绑定 multipart 字段、`FormFile` 读取文件，并用 `http.MaxBytesReader` 限制请求体；service 用随机 import upload ID 生成 `moderation/imports/{operatorID}/{uploadID}.{format}`，调用 `PutObjectStream` 后创建 DB 任务，DB 失败删除对象。

- [ ] **Step 4: 验证并提交**

Run: `go test ./internal/handler/moderation ./internal/service/moderationrule -count=1`，Expected: PASS。

```bash
git add internal/service/moderationrule internal/handler/moderation internal/router/moderation.go
git commit -m "fix(moderation): 完成规则文件上传与失败补偿"
```

### Task 4: 错误报告真实下载

**Files:**
- Modify: `internal/service/moderationrule/types.go`
- Modify: `internal/service/moderationrule/import_worker.go`
- Modify: `internal/handler/moderation/rule_download.go`
- Test: `internal/handler/moderation/rules_test.go`

- [ ] **Step 1: 写失败测试**

mock service 返回 `io.ReadCloser`，断言下载体等于 CSV；无报告 404；对象读取失败 500。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/handler/moderation -run TestDownloadImportErrors -count=1`

Expected: FAIL，当前响应体为空。

- [ ] **Step 3: 实现并验证**

Service 新增 `OpenImportErrors(ctx, id) (io.ReadCloser, error)`，先校验任务再打开限定 object key；handler 设置 header 后 `io.Copy`。Run: `go test ./internal/handler/moderation ./internal/service/moderationrule -count=1`，Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add internal/service/moderationrule internal/handler/moderation
git commit -m "fix(moderation): 返回真实规则导入错误报告"
```

### Task 5: worker 空轮询降噪

**Files:**
- Modify: `internal/repository/moderationrule/import.go`
- Modify: `internal/repository/moderationrule/candidate.go`
- Test: `internal/repository/moderationrule/import_test.go`
- Test: `internal/repository/moderationrule/candidate_test.go`

- [ ] **Step 1: 写失败测试**

使用带 Observer 的 GORM logger，空队列时断言返回 `(nil, nil)` 且不记录 `gorm.ErrRecordNotFound`；数据库错误仍返回。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/repository/moderationrule -run 'TestClaimNext.*Empty' -count=1`

Expected: FAIL，`Take` 触发 record-not-found 日志。

- [ ] **Step 3: 实现**

锁查询改用 `Find(&row)` 并检查 `RowsAffected==0`，保留事务和 `FOR UPDATE`；不修改全局 GORM logger。

- [ ] **Step 4: 验证并提交**

Run: `go test ./internal/repository/moderationrule -count=1`，Expected: PASS。

```bash
git add internal/repository/moderationrule
git commit -m "fix(moderation): 消除规则 worker 空轮询错误日志"
```

### Task 6: Swagger 与全量验证

- [ ] Run: `make swag && go test ./... && go build ./...`

Expected: 全部 PASS，Swagger multipart 参数与错误报告响应正确。
