# Moderation Rule Management API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build administrator rule CRUD, testing, export/template download, recoverable bulk imports, candidate publishing, cleanup, Swagger, and the single-instance worker on top of the index foundation.

**Architecture:** A dedicated `moderationrule` repository and service manager own candidate state, publishing, and one bounded candidate cache. Handlers remain thin DTO adapters under the existing admin moderation routes. The same manager/classifier instance is shared by UGC writes, admin handlers, and the one background worker.

**Tech Stack:** Go 1.25.5, Gin, GORM/MySQL, Garage streaming object storage, zap, encoding/csv, go-sqlmock, gomock, httptest, swaggo.

## Global Constraints

- Complete `2026-06-30-moderation-rule-index-foundation.md` first.
- Read `go-layering`, `go-readability`, `go-testing`, and `http-api` before implementation.
- Admin identity comes only from JWT claims; no actor ID is accepted from request JSON.
- Import is add-only and atomically published; any invalid row makes the whole import invalid.
- One process may have only one building or ready candidate; other imports remain queued.
- Use cursor pagination and exact/prefix pattern search; never issue an unbounded `%term%` scan.
- Uploads, errors, exports, and index artifacts remain streaming end to end.
- Keep handlers DTO-only and regenerate Swagger after route changes.

## File Structure

- `internal/repository/moderationrule/*`: cursor queries, candidates, publishing, imports, and cleanup.
- `internal/service/moderationrule/*`: manager, worker loop, parser, templates, test/export, cache lifecycle.
- `internal/dto/admin_moderation_rule.go`: complete request/response contract.
- `internal/handler/moderation/rules*.go`: admin HTTP adapters and download responses.
- `internal/router/moderation*.go`: one dependency graph shared by core moderation and management.
- `internal/worker/moderation/cleanup.go`: bounded artifact/candidate cleanup extension.

---

### Task 1: Cursor Queries, Metadata, and Source Repository

**Files:**
- Modify: `internal/repository/moderationrule/repository.go`
- Modify: `internal/repository/moderationrule/types.go`
- Create: `internal/repository/moderationrule/query.go`
- Create: `internal/repository/moderationrule/query_test.go`
- Create: `internal/repository/moderationrule/mock/mock_repository.go`

**Interfaces:**
- Produces: `ListRules`, `ListSources`, `FindDuplicateHashes`, `CurrentStatus`, and source upsert methods.
- Produces: cursor/filter records consumed by service and handler tasks.

- [ ] **Step 1: Write failing cursor and duplicate-batch tests**

```go
func TestListRulesUsesIDCursorAndPrefixSearch(t *testing.T) {
    filter := moderationrule.RuleFilter{AfterID: 100, Limit: 50, PatternPrefix: "风险", Category: "fraud"}
    mock.ExpectQuery("SELECT .* WHERE .*id > .*pattern LIKE.* ORDER BY .*id ASC LIMIT").
        WithArgs(uint64(100), "风险%", "fraud", 51).
        WillReturnRows(ruleListRows())
    page, err := repo.ListRules(context.Background(), filter)
    require.NoError(t, err)
    assert.True(t, page.HasMore)
    assert.NotZero(t, page.NextCursor)
}

func TestFindDuplicateHashesUsesOneINQueryPerChunk(t *testing.T) {
    hashes := []moderationrule.DedupeHash{{1}, {2}}
    mock.ExpectQuery("dedupe_hash IN \\(\\?,\\?\\)").WithArgs(hashes[0][:], hashes[1][:]).WillReturnRows(duplicateRows())
    _, err := repo.FindDuplicateHashes(context.Background(), 7, hashes)
    require.NoError(t, err)
}
```

- [ ] **Step 2: Run repository tests and verify failure**

Run: `go test ./internal/repository/moderationrule -run 'ListRules|DuplicateHashes|Sources|CurrentStatus' -count=1`

Expected: FAIL because management queries do not exist.

- [ ] **Step 3: Define exact query records and repository interface**

```go
type RuleFilter struct {
    AfterID, ExactID uint64
    Limit int
    ExactPattern, PatternPrefix string
    Category, RuleType, RiskLevel, Effect string
    SourceID uint64
    Active *bool
}

type RulePage struct {
    Rules []RuleListRecord
    NextCursor uint64
    HasMore bool
}

type DedupeHash [32]byte

type ManagementRepository interface {
    SnapshotRepository
    ListRules(context.Context, RuleFilter) (RulePage, error)
    ListSources(context.Context) ([]SourceRecord, error)
    EnsureSource(context.Context, string) (SourceRecord, error)
    FindDuplicateHashes(context.Context, uint64, []DedupeHash) (map[DedupeHash]uint64, error)
    CurrentStatus(context.Context) (StatusRecord, error)
}
```

- [ ] **Step 4: Implement indexed cursor queries and regenerate the package mock**

Fetch `limit+1`, remove the sentinel row, and set `NextCursor` to the last returned ID. Exact pattern and prefix are mutually exclusive. `Active` is evaluated relative to the current published ruleset, excluding failed activation rulesets. Chunk duplicate hashes to a bounded parameter count.

- [ ] **Step 5: Run repository tests**

Run: `go test ./internal/repository/moderationrule -count=1`

Expected: PASS.

- [ ] **Step 6: Commit management queries**

```bash
git add internal/repository/moderationrule
git commit -m "feat(moderation): 新增规则游标查询仓储"
```

### Task 2: Candidate Creation, Publishing, and Cache Lifecycle

**Files:**
- Create: `internal/repository/moderationrule/candidate.go`
- Create: `internal/repository/moderationrule/candidate_test.go`
- Create: `internal/service/moderationrule/types.go`
- Create: `internal/service/moderationrule/errors.go`
- Create: `internal/service/moderationrule/manager.go`
- Create: `internal/service/moderationrule/candidate.go`
- Create: `internal/service/moderationrule/candidate_test.go`
- Create: `internal/service/moderationrule/mock/mock_service.go`

**Interfaces:**
- Produces: `moderationrule.Service` and `moderationrule.Worker`.
- Produces: create/replace/batch-status commands and `PublishCandidate`.
- Consumes: `ruleindex`, streaming object store, repository, and the core classifier snapshot replacer.

- [ ] **Step 1: Write failing service tests for immutable edits, conflicts, and eviction**

```go
func TestReplaceRuleCreatesNewFactAndRemovalCandidate(t *testing.T) {
    repo.EXPECT().CurrentStatus(gomock.Any()).Return(moderationrule.StatusRecord{CurrentRulesetID: 7}, nil)
    repo.EXPECT().CreateCandidate(gomock.Any(), gomock.Any()).DoAndReturn(assertReplacementOf(41))
    input := moderationrule.RuleInput{RuleType: "keyword", Pattern: "风险词", Category: "other", Effect: "review", RiskLevel: "medium", Priority: 100, SourceID: 1}
    got, err := manager.ReplaceRule(ctx, moderationrule.ReplaceRuleCommand{RuleID: 41, ExpectedRulesetID: 7, ActorID: 1, Rule: input})
    require.NoError(t, err)
    assert.Equal(t, uint64(7), got.BaseRulesetID)
}

func TestPublishRejectsStaleBaseWithoutReplacingSnapshot(t *testing.T) {
    repo.EXPECT().PublishCandidate(gomock.Any(), candidateID, uint64(7)).Return(ErrRulesetConflict)
    err := manager.PublishCandidate(ctx, candidateID, 7, 1)
    assert.ErrorIs(t, err, moderationrule.ErrRulesetConflict)
    assert.Equal(t, 0, classifier.replaceCalls)
}

func TestCandidateCacheEvictsOnTTLAndCancel(t *testing.T) {
    cache := newCandidateCache(10*time.Minute, clock.Now)
    cache.Store(8, snapshot)
    clock.Advance(11*time.Minute)
    cache.EvictExpired()
    assert.Nil(t, cache.Load(8))
}
```

- [ ] **Step 2: Run service tests and verify failure**

Run: `go test ./internal/service/moderationrule -run 'ReplaceRule|Publish|CandidateCache' -count=1`

Expected: FAIL because the service package does not exist.

- [ ] **Step 3: Implement repository candidate transactions**

```go
type CreateCandidateCommand struct {
    BaseRulesetID uint64
    ActorID uint64
    Additions []RuleDraft
    RemoveRuleIDs []uint64
}

func (r *repository) PublishCandidate(ctx context.Context, id, expectedBase uint64, now time.Time) error
```

`CreateCandidate` inserts a building ruleset, candidate rule facts, and removal rows in bounded transactions. `PublishCandidate` locks the candidate and current published row, verifies base/current equality, fills deactivation IDs, supersedes the old ruleset, publishes the candidate, and commits atomically.

- [ ] **Step 4: Implement manager candidate building and lock-free publish**

```go
type Service interface {
    ListRules(context.Context, ListQuery) (RulePage, error)
    Metadata(context.Context) (Metadata, error)
    Status(context.Context) (Status, error)
    CreateRule(context.Context, CreateRuleCommand) (Job, error)
    ReplaceRule(context.Context, ReplaceRuleCommand) (Job, error)
    BatchStatus(context.Context, BatchStatusCommand) (Job, error)
    TestText(context.Context, TestTextCommand) (TestResult, error)
    PublishCandidate(context.Context, uint64, uint64, uint64) error
    CancelCandidate(context.Context, uint64, uint64) error
}

type Worker interface {
    Run(context.Context)
    ProcessOnce(context.Context) error
}
```

Create/replace/batch methods only persist a building candidate and return its job; they do not rebuild 500000 rules inside the HTTP request. `ProcessOnce` claims one building ruleset, builds it through `ruleindex.Build`, enforces memory limits, writes the codec to an `os.CreateTemp` file, closes and reopens it, then calls `PutObjectStream` with `EncodedSize`; remove the temp file on every path. A ready ruleset with no import row is auto-published; a ruleset referenced by an import remains ready for explicit confirmation. On publish, load or reuse the bounded candidate through `OpenObject`, commit DB first, atomically replace the classifier snapshot, and clear cache. Never hold the publish mutex during index construction or object download.

- [ ] **Step 5: Add create/replace/batch validation and duplicate protection**

Validate type/category/effect/risk, raw and normalized length, regexp compilation, composite signals, maximum 1000 batch IDs, non-empty resulting ruleset, and expected current version. Compute SHA-256 over `effect + NUL + type + NUL + duplicateBasis` and compare original normalized values if a hash already exists.

- [ ] **Step 6: Run service and race tests**

Run: `go test ./internal/repository/moderationrule ./internal/service/moderationrule -count=1`

Run: `go test -race ./internal/service/moderationrule -count=1`

Expected: PASS.

- [ ] **Step 7: Commit candidate management**

```bash
git add internal/repository/moderationrule internal/service/moderationrule
git commit -m "feat(moderation): 实现规则候选发布流程"
```

### Task 3: Streaming Import Validation and Recoverable Worker

**Files:**
- Create: `internal/repository/moderationrule/import.go`
- Create: `internal/repository/moderationrule/import_test.go`
- Create: `internal/service/moderationrule/import_parse.go`
- Create: `internal/service/moderationrule/import_validate.go`
- Create: `internal/service/moderationrule/import_worker.go`
- Create: `internal/service/moderationrule/import_test.go`
- Modify: `internal/service/moderationrule/types.go`
- Modify: `internal/service/moderationrule/manager.go`

**Interfaces:**
- Produces: upload job creation, import history/detail, cancellation, error report, and `Worker.Run(ctx)`.
- Consumes: one `storage.ObjectStreamStore`, repository claiming, and Task 2 candidate builder.

- [ ] **Step 1: Write failing CSV/TXT and all-or-nothing tests**

```go
func TestCSVParserAcceptsQuotedNewlineAndLeadingComments(t *testing.T) {
    input := "# template\npattern,category,risk_level\n\"line1\nline2\",other,medium\n"
    rows, errs := parseCSV(strings.NewReader(input), defaults())
    require.Empty(t, errs)
    assert.Equal(t, "line1\nline2", rows[0].Pattern)
}

func TestInvalidImportCreatesNoCandidateRules(t *testing.T) {
    repo.EXPECT().FindDuplicateHashes(gomock.Any(), gomock.Any(), gomock.Any()).Return(map[string]uint64{"hash": 9}, nil)
    repo.EXPECT().CreateCandidate(gomock.Any(), gomock.Any()).Times(0)
    err := worker.ProcessOnce(ctx)
    require.NoError(t, err)
    assert.Equal(t, "invalid", stored.ValidationStatus)
}
```

- [ ] **Step 2: Run import tests and verify failure**

Run: `go test ./internal/service/moderationrule ./internal/repository/moderationrule -run 'CSVParser|TXTParser|InvalidImport|ImportClaim' -count=1`

Expected: FAIL because parser and worker methods do not exist.

- [ ] **Step 3: Implement standard CSV and TXT streaming parsers**

Use `encoding/csv.Reader` with `FieldsPerRecord = -1`. Permit `#` comment lines only before the CSV header. TXT accepts one non-empty, non-comment keyword per line. Apply row values over task defaults except source, which is task-level only. Stop reading after configured bytes or rows.

Add repository methods with exact responsibilities:

```go
CreateImport(context.Context, CreateImportCommand) (ImportRecord, error)
ClaimNextImport(context.Context, time.Time) (*ImportRecord, error)
UpdateImportValidation(context.Context, UpdateImportValidationCommand) error
ListImports(context.Context, uint64, int) (ImportPage, error)
GetImport(context.Context, uint64) (ImportRecord, error)
CancelImport(context.Context, uint64, uint64, time.Time) error
ResetInterruptedImports(context.Context, time.Time) (int64, error)
```

- [ ] **Step 4: Implement bounded validation and error streaming**

Keep at most `MaxImportRows` SHA-256 keys in the file-local set. Query database duplicates in chunks of 1000. Write `row,field,value,error_code,message` to a temporary error CSV as errors are found, then stream it to Garage. Do not retain all parsed rows or errors in a Go slice.

- [ ] **Step 5: Implement recoverable single-worker claiming**

```go
func (m *manager) Run(ctx context.Context) {
    ticker := time.NewTicker(m.pollInterval)
    defer ticker.Stop()
    for {
        _ = m.ProcessOnce(ctx)
        m.cache.EvictExpired()
        select { case <-ctx.Done(): return; case <-ticker.C: }
    }
}

func (m *manager) ProcessOnce(ctx context.Context) error {
    if err := m.ProcessNextImport(ctx); err != nil { return err }
    return m.ProcessNextRuleset(ctx)
}
```

Claim one queued import transactionally, reset interrupted validation/build jobs on startup, and never create one goroutine or ticker per import. After validation succeeds, re-read the source stream and insert candidate facts in bounded batches, then enqueue the linked building ruleset. Partial candidate rows remain unpublished and are cleanup-safe. `ProcessNextRuleset` is the Task 2 generic builder and is the only path that constructs or auto-publishes a snapshot.

The import-create command accepts a trimmed `SourceName` (1–100 characters); create or reuse the source through `EnsureSource` before storing the import. CSV rows cannot override it. This is the only source-creation path in this phase; metadata exposes the resulting source ID for later manual rules.

- [ ] **Step 6: Run import, recovery, close, and race tests**

Run: `go test ./internal/service/moderationrule ./internal/repository/moderationrule -run 'Import|CSV|TXT|Close|Recover' -count=1`

Run: `go test -race ./internal/service/moderationrule -count=1`

Expected: PASS.

- [ ] **Step 7: Commit import processing**

```bash
git add internal/repository/moderationrule internal/service/moderationrule
git commit -m "feat(moderation): 新增规则批量导入任务"
```

### Task 4: Templates, Exports, Text Testing, and Admin DTOs

**Files:**
- Create: `internal/dto/admin_moderation_rule.go`
- Create: `internal/service/moderationrule/template.go`
- Create: `internal/service/moderationrule/export.go`
- Create: `internal/service/moderationrule/test_text.go`
- Create: `internal/service/moderationrule/template_test.go`
- Create: `internal/service/moderationrule/export_test.go`
- Create: `internal/handler/moderation/rules.go`
- Create: `internal/handler/moderation/rule_download.go`
- Create: `internal/handler/moderation/rules_test.go`
- Modify: `internal/handler/moderation/moderation.go`
- Modify: `internal/handler/moderation/response.go`
- Modify: `pkg/response/response.go`

**Interfaces:**
- Produces: DTO-only JSON endpoints and streaming download handlers.
- Produces: stable errors `MODERATION_RULESET_CONFLICT`, `MODERATION_RULE_LIMIT`, `MODERATION_INDEX_MEMORY_LIMIT`, `MODERATION_IMPORT_INVALID`.

- [ ] **Step 1: Write failing handler tests for auth, cursors, template, and download headers**

```go
func TestRuleTemplateSetsSafeAttachmentHeaders(t *testing.T) {
    recorder := serveRuleAdmin(http.MethodGet, "/admin/moderation/rule-imports/template?format=csv", nil, service)
    assert.Equal(t, http.StatusOK, recorder.Code)
    assert.Equal(t, "text/csv; charset=utf-8", recorder.Header().Get("Content-Type"))
    assert.Contains(t, recorder.Header().Get("Content-Disposition"), "moderation-rules-template.csv")
}

func TestCreateRuleUsesJWTActorAndExpectedVersion(t *testing.T) {
    service.EXPECT().CreateRule(gomock.Any(), gomock.Any()).DoAndReturn(assertActorAndVersion(1, 7))
    recorder := serveRuleAdmin(http.MethodPost, "/admin/moderation/rules", strings.NewReader(validRuleJSON()), service)
    assert.Equal(t, http.StatusAccepted, recorder.Code)
}
```

- [ ] **Step 2: Run handler tests and verify failure**

Run: `go test ./internal/handler/moderation -run 'Rule|Template|Export|Import' -count=1`

Expected: FAIL because DTOs and handlers do not exist.

- [ ] **Step 3: Define bounded DTOs and converters**

Define request bindings for cursor/limit/search mode, create/replace, batch IDs, test text, multipart defaults including `source_name`, and publish expected version. Define responses for rule rows, metadata, status, job, import history/detail, test hits, and cursor pages. Keep model conversion in handler-local functions.

```go
type AdminModerationRulePageResp struct {
    List       []AdminModerationRuleResp `json:"list"`
    NextCursor uint64                    `json:"next_cursor"`
    HasMore    bool                      `json:"has_more"`
}

type AdminModerationRuleListReq struct {
    Cursor        uint64 `form:"cursor"`
    Limit         int    `form:"limit" binding:"omitempty,min=1,max=100"`
    ID            uint64 `form:"id"`
    Pattern       string `form:"pattern" binding:"omitempty,max=500"`
    SearchMode    string `form:"search_mode" binding:"omitempty,oneof=exact prefix"`
    Category      string `form:"category"`
    RuleType      string `form:"rule_type"`
    RiskLevel     string `form:"risk_level"`
    Effect        string `form:"effect"`
    SourceID      uint64 `form:"source_id"`
    Active        *bool  `form:"active"`
}

type AdminModerationRuleSaveReq struct {
    ExpectedRulesetVersion uint64  `json:"expected_ruleset_version" binding:"required"`
    Name                   *string `json:"name" binding:"omitempty,max=100"`
    RuleType               string  `json:"rule_type" binding:"required,oneof=keyword regexp composite"`
    Pattern                string  `json:"pattern" binding:"required,max=500"`
    Category               string  `json:"category" binding:"required"`
    Effect                 string  `json:"effect" binding:"required,oneof=review allow"`
    RiskLevel              string  `json:"risk_level" binding:"required,oneof=low medium high"`
    Priority               int32   `json:"priority"`
    SourceID               uint64  `json:"source_id" binding:"required"`
}

type AdminModerationRuleBatchStatusReq struct {
    ExpectedRulesetVersion uint64   `json:"expected_ruleset_version" binding:"required"`
    RuleIDs                []uint64 `json:"rule_ids" binding:"required,min=1,max=1000,dive,required"`
    Active                 bool     `json:"active"`
}

type AdminModerationRuleTestReq struct {
    Text      string  `json:"text" binding:"required,max=10000"`
    RulesetID *uint64 `json:"ruleset_id"`
}

type AdminModerationRuleImportPublishReq struct {
    ExpectedRulesetVersion uint64 `json:"expected_ruleset_version" binding:"required"`
}
```

- [ ] **Step 4: Implement safe templates and streaming export**

CSV template contains `#` instructions, header, and one disabled synthetic example; TXT contains comments only. Export writes UTF-8 BOM and each row directly to `csv.Writer`. Prefix cells beginning with `=`, `+`, `-`, or `@` with a single quote.

- [ ] **Step 5: Implement current/candidate text testing**

Limit text to 10000 characters. Match against current or one ready candidate, then load metadata for only the bounded hit/suppressed IDs. Return effective risk, ruleset ID, matched excerpts, `truncated`, and suppressed allow hits without exposing data publicly.

- [ ] **Step 6: Implement thin handlers and stable error mapping**

Handlers bind, obtain actor claims, call only the service, select JSON or stream response, and map conflict/limit/import errors. Use `202 Accepted` for asynchronous create/replace/batch/import jobs and `200` for reads/publish completion.

- [ ] **Step 7: Run handler/service tests**

Run: `go test ./internal/dto ./internal/service/moderationrule ./internal/handler/moderation -count=1`

Expected: PASS.

- [ ] **Step 8: Commit API contracts and handlers**

```bash
git add internal/dto/admin_moderation_rule.go internal/service/moderationrule internal/handler/moderation pkg/response/response.go
git commit -m "feat(moderation): 新增审核规则管理接口"
```

### Task 5: Runtime Sharing, Routes, and Cleanup

**Files:**
- Modify: `internal/router/router.go`
- Modify: `internal/router/moderation.go`
- Modify: `internal/router/moderation_admin.go`
- Modify: `internal/router/moderation_test.go`
- Modify: `internal/router/router_test.go`
- Modify: `internal/bootstrap/bootstrap.go`
- Modify: `cmd/server/main.go`
- Modify: `internal/worker/moderation/cleanup.go`
- Modify: `internal/worker/moderation/cleanup_test.go`
- Create: `internal/repository/moderationrule/cleanup.go`
- Create: `internal/repository/moderationrule/cleanup_test.go`

**Interfaces:**
- Produces: `router.Runtime{Analytics, ModerationRules}` returned by `router.Setup`.
- Ensures: one classifier/manager instance shared by core writes, admin handlers, and worker.

- [ ] **Step 1: Write failing runtime identity, route, and cleanup tests**

```go
func TestModerationCoreAdminAndWorkerShareOneRuleManager(t *testing.T) {
    runtime := newModerationRuntimeForTest(t)
    assert.Same(t, runtime.RuleManager, runtime.AdminRuleService)
    assert.Same(t, runtime.Classifier, runtime.RuleManager.Classifier())
}

func TestRegisterAdminRoutesIncludesRuleManagement(t *testing.T) {
    routes := registeredAdminRoutes(t)
    assert.Contains(t, routes, "GET /admin/moderation/rules")
    assert.Contains(t, routes, "POST /admin/moderation/rule-imports")
}
```

- [ ] **Step 2: Run router/worker tests and verify failure**

Run: `go test ./internal/router ./internal/bootstrap ./internal/worker/moderation -run 'Moderation|Rule|Cleanup' -count=1`

Expected: FAIL because runtime and routes are not wired.

- [ ] **Step 3: Return one composite runtime from router setup**

```go
type Runtime struct {
    Analytics AnalyticsRuntime
    ModerationRules moderationrule.Worker
}
```

Construct repository, classifier, and manager once before UGC services. Inject the classifier into core moderation, the manager into `AdminHandler`, store the same manager in `Runtime`, and start `runtime.ModerationRules.Run(ctx)` once from bootstrap/main. Preserve the existing single analytics ingestor invariant.

- [ ] **Step 4: Register every rule/import route with appropriate limits**

Register reads directly in the admin group; protect create/replace/batch/publish/cancel with `RateLimitNormal`; protect multipart import with `RateLimitTempUpload`. Routes must be absent when moderation is disabled.

- [ ] **Step 5: Extend bounded cleanup**

Delete invalid/canceled/failed unpublished rule facts, removal rows, expired import objects/reports, and failed/canceled/superseded index artifacts in configured batches. Never delete current published or ready candidate artifacts. Treat missing objects as success and leave referenced sources intact.

- [ ] **Step 6: Run router, worker, and full backend tests**

Run: `go test ./internal/router ./internal/bootstrap ./internal/worker/moderation ./internal/repository/moderationrule -count=1`

Run: `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 7: Commit runtime wiring and cleanup**

```bash
git add internal/router internal/bootstrap cmd/server internal/worker/moderation internal/repository/moderationrule
git commit -m "feat(moderation): 接入规则任务运行时与清理"
```

### Task 6: Swagger and Backend Acceptance

**Files:**
- Modify (generated): `docs/docs.go`
- Modify (generated): `docs/swagger.json`
- Modify (generated): `docs/swagger.yaml`
- Modify: `docs/moderation-rollout.md`

**Interfaces:**
- Consumes: all management API tasks.
- Produces: documented DTO-only admin API and deployment notes.

- [ ] **Step 1: Generate and inspect Swagger**

Run: `make swag`

Expected: generated docs include `/admin/moderation/rules`, `/rules/test`, `/rules/export`, `/rules/metadata`, `/rule-imports`, template, error, publish, and cancel routes; schemas refer only to `dto.*`.

- [ ] **Step 2: Update rollout documentation with resource prerequisites**

Document the migration order, 1 GB recommendation for about 100k rules, 2–4 GB for approaching 500k, Garage artifact retention, configurable memory thresholds, initial import/publish sequence, rollback by keeping moderation disabled, and benchmark command.

- [ ] **Step 3: Run acceptance verification**

Run: `go test ./... -count=1`

Run: `go test -race ./internal/service/moderation/... ./internal/service/moderationrule ./internal/repository/moderationrule -count=1`

Run: `go build ./cmd/server`

Expected: all commands PASS.

- [ ] **Step 4: Commit docs and generated API**

```bash
git add docs
git commit -m "docs(moderation): 更新规则管理接口与部署说明"
```
