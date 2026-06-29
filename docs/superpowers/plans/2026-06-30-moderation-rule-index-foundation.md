# Moderation Rule Index Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace linear moderation rule loading and matching with a versioned schema, streaming storage/repository boundaries, and a bounded compact Aho–Corasick runtime without changing public HTTP endpoints.

**Architecture:** MySQL remains the rule fact source. Startup streams the current published ruleset into a compact immutable snapshot, then the existing moderation classifier reads it through one atomic pointer. Binary codecs and streaming object I/O are introduced here but publishing/import orchestration is deferred to the next plan.

**Tech Stack:** Go 1.25.5, GORM/MySQL, AWS SDK v2/Garage, zap, go-sqlmock, gomock, testify.

## Global Constraints

- Read `go-layering`, `go-readability`, and `go-testing` before implementation; use TDD for every task.
- Preserve constructor injection; do not add global DB, storage, logger, worker, or classifier state.
- Do not return `model.*` through handlers or Swagger.
- Runtime keyword matching must not scan all rules and must not retain keyword names, categories, sources, or patterns.
- Keyword capacity is 500000; default regex capacity is 200 with a validated hard maximum of 500.
- Persist at most 128 matched IDs while computing risk over every unsuppressed match.
- All new comments that explain non-obvious boundaries use concise Chinese.
- Do not modify unrelated dirty files in `../blog-frontend`.

## File Structure

- `internal/model/moderation_rule.go`: immutable rule/source facts.
- `internal/model/moderation_ruleset.go`: ruleset, removal, and import metadata.
- `migrations/20260630_moderation_rule_management.sql`: deployed schema transition and seed backfill.
- `internal/service/moderation/textnorm/*`: normalization shared by the core classifier and index package.
- `internal/service/moderation/ruleindex/*`: compact builder, matcher, memory accounting, and stream codec.
- `internal/repository/moderationrule/*`: current ruleset lookup and streaming rule reader.
- `pkg/storage/*`: bounded streaming S3 read/write capability.
- `internal/service/moderation/classifier.go`: adapter from processed content to the immutable index.

---

### Task 1: Versioned Rule Schema and Capacity Configuration

**Files:**
- Modify: `internal/model/moderation_rule.go`
- Create: `internal/model/moderation_ruleset.go`
- Modify: `internal/model/moderation_content.go`
- Modify: `internal/dbschema/schema.go`
- Modify: `internal/dbschema/seed.go`
- Modify: `internal/dbschema/seed_test.go`
- Modify: `internal/model/moderation_contract_test.go`
- Modify: `pkg/config/moderation.go`
- Modify: `pkg/config/moderation_validate.go`
- Modify: `pkg/config/config_test.go`
- Modify: `config/config.yaml`
- Modify: `config/config.test.yaml`
- Create: `migrations/20260630_moderation_rule_management.sql`

**Interfaces:**
- Produces: `model.ModerationRuleSource`, `model.ModerationRuleset`, `model.ModerationRulesetRemoval`, `model.ModerationRuleImport`.
- Produces: `config.ModerationRulesConfig` fields used by every later task.

- [ ] **Step 1: Write failing model and configuration contract tests**

```go
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

func TestModerationRulesConfigRejectsUnsafeBounds(t *testing.T) {
    cfg := validModerationConfig()
    cfg.Rules.MaxEnabledRegexRules = 501
    assert.ErrorContains(t, cfg.Validate("test"), "must not exceed 500")
    cfg = validModerationConfig()
    cfg.Rules.MaxBuildPeakMemoryMB = cfg.Rules.MaxIndexMemoryMB
    assert.ErrorContains(t, cfg.Validate("test"), "must exceed")
}
```

- [ ] **Step 2: Run focused tests and verify the new contracts fail**

Run: `go test ./internal/model ./pkg/config ./internal/dbschema -run 'ModerationRule|ModerationRulesConfig|ModerationDefaults' -count=1`

Expected: FAIL because the new fields and config values do not exist.

- [ ] **Step 3: Implement the exact model shapes**

```go
type ModerationRule struct {
    ID                    uint64     `gorm:"primaryKey"`
    Name                  *string    `gorm:"size:100"`
    RuleType              string     `gorm:"size:16;not null"`
    Pattern               string     `gorm:"size:500;not null"`
    DedupeHash            []byte     `gorm:"type:binary(32);not null;index:idx_moderation_rule_dedupe"`
    Category              string     `gorm:"size:24;not null;index:idx_moderation_rule_filter,priority:1"`
    Effect                string     `gorm:"size:16;not null"`
    RiskLevel             string     `gorm:"size:16;not null;index:idx_moderation_rule_filter,priority:2"`
    Priority              int32      `gorm:"not null;default:100"`
    SourceID              uint64     `gorm:"not null;index:idx_moderation_rule_source"`
    ActivatedRulesetID    uint64     `gorm:"not null;index:idx_moderation_rule_interval,priority:1"`
    DeactivatedRulesetID  *uint64    `gorm:"index:idx_moderation_rule_interval,priority:2"`
    ReplacesRuleID        *uint64    `gorm:"index:idx_moderation_rule_replaces"`
    CreatedAt             time.Time  `gorm:"type:datetime(3);not null"`
    UpdatedAt             time.Time  `gorm:"type:datetime(3);not null"`
}

type ModerationRuleSource struct {
    ID        uint64    `gorm:"primaryKey"`
    Name      string    `gorm:"size:100;not null;uniqueIndex:uk_moderation_rule_source_name"`
    CreatedAt time.Time `gorm:"type:datetime(3);not null"`
    UpdatedAt time.Time `gorm:"type:datetime(3);not null"`
}

type ModerationRuleset struct {
    ID                   uint64     `gorm:"primaryKey"`
    BaseRulesetID        *uint64    `gorm:"index:idx_moderation_ruleset_base"`
    Status               string     `gorm:"size:16;not null;index:idx_moderation_ruleset_status"`
    RuleCount            uint64     `gorm:"not null;default:0"`
    KeywordCount         uint64     `gorm:"not null;default:0"`
    RegexpCount          uint64     `gorm:"not null;default:0"`
    CompositeCount       uint64     `gorm:"not null;default:0"`
    IndexBytes           uint64     `gorm:"not null;default:0"`
    BuildPeakBytes       uint64     `gorm:"not null;default:0"`
    BuildDurationMS      uint64     `gorm:"not null;default:0"`
    IndexObjectKey       *string    `gorm:"size:500"`
    IndexFormatVersion   uint32     `gorm:"not null;default:1"`
    IndexSHA256          *string    `gorm:"size:64"`
    OperatorID           *uint64    `gorm:"index:idx_moderation_ruleset_operator"`
    FailureCode          *string    `gorm:"size:64"`
    CreatedAt            time.Time  `gorm:"type:datetime(3);not null"`
    UpdatedAt            time.Time  `gorm:"type:datetime(3);not null"`
}
```

```go
type ModerationRulesetRemoval struct {
    RulesetID uint64    `gorm:"primaryKey;autoIncrement:false"`
    RuleID    uint64    `gorm:"primaryKey;autoIncrement:false;index:idx_moderation_ruleset_removal_rule"`
    CreatedAt time.Time `gorm:"type:datetime(3);not null"`
}

type ModerationRuleImport struct {
    ID                  uint64     `gorm:"primaryKey"`
    FileName            string     `gorm:"size:255;not null"`
    Format              string     `gorm:"size:8;not null"`
    FileSize            uint64     `gorm:"not null"`
    ObjectKey           string     `gorm:"size:500;not null"`
    SourceID            uint64     `gorm:"not null;index:idx_moderation_rule_import_source"`
    DefaultCategory     string     `gorm:"size:24;not null"`
    DefaultEffect       string     `gorm:"size:16;not null"`
    DefaultRiskLevel    string     `gorm:"size:16;not null"`
    DefaultPriority     int32      `gorm:"not null"`
    ValidationStatus    string     `gorm:"size:16;not null;index:idx_moderation_rule_import_status"`
    TotalRows           uint64     `gorm:"not null;default:0"`
    ValidRows           uint64     `gorm:"not null;default:0"`
    DuplicateRows       uint64     `gorm:"not null;default:0"`
    ErrorRows           uint64     `gorm:"not null;default:0"`
    ErrorObjectKey      *string    `gorm:"size:500"`
    RulesetID           *uint64    `gorm:"index:idx_moderation_rule_import_ruleset"`
    OperatorID          uint64     `gorm:"not null;index:idx_moderation_rule_import_operator"`
    CreatedAt           time.Time  `gorm:"type:datetime(3);not null"`
    UpdatedAt           time.Time  `gorm:"type:datetime(3);not null"`
}
```

Add `RuleMatchesTruncated bool` with `gorm:"type:tinyint(1);not null;default:0"` to both `ModerationRevision` and `ModerationAttempt`.

- [ ] **Step 4: Add migration, dependency-ordered registration, seed backfill, and config defaults**

```yaml
rules:
  max_pattern_chars: 500
  max_keyword_rules: 500000
  max_enabled_regex_rules: 200
  max_import_rows: 200000
  max_import_file_mb: 50
  max_rule_matches_per_content: 128
  max_index_memory_mb: 512
  max_build_peak_memory_mb: 1024
  index_build_timeout: 10m
  candidate_cache_ttl: 10m
  import_artifact_retention_days: 7
  ruleset_artifact_retention_days: 7
  import_history_retention_days: 30
```

The migration must create source/ruleset/removal/import tables, seed source ID 1 and published ruleset ID 1, backfill the active baseline rule to source/ruleset 1, add `rule_matches_truncated`, then drop `enabled`, `ruleset_version`, and the obsolete snapshot index. Failed statements must not be hidden behind `IF` branches that silently skip an incompatible schema.

```sql
CREATE TABLE `moderation_rule_source` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_moderation_rule_source_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `moderation_ruleset` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `base_ruleset_id` bigint unsigned NULL,
  `status` varchar(16) NOT NULL,
  `rule_count` bigint unsigned NOT NULL DEFAULT 0,
  `keyword_count` bigint unsigned NOT NULL DEFAULT 0,
  `regexp_count` bigint unsigned NOT NULL DEFAULT 0,
  `composite_count` bigint unsigned NOT NULL DEFAULT 0,
  `index_bytes` bigint unsigned NOT NULL DEFAULT 0,
  `build_peak_bytes` bigint unsigned NOT NULL DEFAULT 0,
  `build_duration_ms` bigint unsigned NOT NULL DEFAULT 0,
  `index_object_key` varchar(500) NULL,
  `index_format_version` int unsigned NOT NULL DEFAULT 1,
  `index_sha256` char(64) NULL,
  `operator_id` bigint unsigned NULL,
  `failure_code` varchar(64) NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`), KEY `idx_moderation_ruleset_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE `moderation_rule`
  ADD COLUMN `dedupe_hash` binary(32) NULL,
  ADD COLUMN `category` varchar(24) NOT NULL DEFAULT 'other',
  ADD COLUMN `effect` varchar(16) NOT NULL DEFAULT 'review',
  ADD COLUMN `source_id` bigint unsigned NULL,
  ADD COLUMN `activated_ruleset_id` bigint unsigned NULL,
  ADD COLUMN `deactivated_ruleset_id` bigint unsigned NULL,
  ADD COLUMN `replaces_rule_id` bigint unsigned NULL;
```

After backfill, make the new required columns `NOT NULL`, make `name` nullable, add foreign keys/checks/indexes, and only then drop the two old columns.

- [ ] **Step 5: Run model/config/schema tests**

Run: `go test ./internal/model ./pkg/config ./internal/dbschema -count=1`

Expected: PASS.

- [ ] **Step 6: Commit the schema foundation**

```bash
git add internal/model internal/dbschema pkg/config config migrations/20260630_moderation_rule_management.sql
git commit -m "feat(moderation): 建立版本化规则数据模型"
```

### Task 2: Bounded Streaming Object Storage

**Files:**
- Modify: `pkg/storage/resolver.go`
- Modify: `pkg/storage/storage.go`
- Modify: `pkg/storage/garage.go`
- Modify: `pkg/storage/garage_test.go`

**Interfaces:**
- Produces: `storage.ObjectStreamStore`.
- Produces: `PutObjectStream(ctx, key, body, size, contentType)` and `OpenObject(ctx, key, maxBytes)`.

- [ ] **Step 1: Write failing stream and close-behavior tests**

```go
func TestPutObjectStreamForwardsReaderWithoutReadAll(t *testing.T) {
    source := &countingReader{Reader: strings.NewReader("rules")}
    err := client.PutObjectStream(context.Background(), "moderation/rules.csv", source, 5, "text/csv")
    require.NoError(t, err)
    assert.Equal(t, 5, source.readBytes)
}

func TestOpenObjectRejectsContentLengthAboveLimitAndClosesBody(t *testing.T) {
    body := &trackedReadCloser{Reader: strings.NewReader("oversized")}
    api.getOutput = &s3.GetObjectOutput{Body: body, ContentLength: aws.Int64(9)}
    _, err := client.OpenObject(context.Background(), "x", 8)
    assert.ErrorIs(t, err, storage.ErrObjectTooLarge)
    assert.True(t, body.closed)
}
```

- [ ] **Step 2: Run the stream tests and verify failure**

Run: `go test ./pkg/storage -run 'ObjectStream|OpenObject' -count=1`

Expected: FAIL because streaming methods do not exist.

- [ ] **Step 3: Add the minimal stream interface and Garage implementation**

```go
type ObjectStreamStore interface {
    PutObjectStream(ctx context.Context, objectName string, body io.Reader, size int64, contentType string) error
    OpenObject(ctx context.Context, objectName string, maxBytes int64) (io.ReadCloser, error)
}

func (c *Client) PutObjectStream(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
    if body == nil || size < 0 { return errors.New("对象流参数无效") }
    _, err := c.impl.objectAPI.PutObject(ctx, &s3.PutObjectInput{
        Bucket: aws.String(c.impl.bucket), Key: aws.String(normalizeObjectName(key)),
        Body: body, ContentLength: aws.Int64(size), ContentType: aws.String(contentType),
    })
    return err
}
```

`OpenObject` must reject a declared `ContentLength` above `maxBytes`, return a wrapper that counts bytes and produces `ErrObjectTooLarge` if the remote server exceeds the limit, and close the S3 body on every rejected path. Add delegating methods to `CachedObjectURLResolver` and invalidate URL cache after streamed writes.

- [ ] **Step 4: Run storage tests**

Run: `go test ./pkg/storage -count=1`

Expected: PASS, including existing byte-oriented callers.

- [ ] **Step 5: Commit streaming storage**

```bash
git add pkg/storage
git commit -m "feat(storage): 新增有界对象流读写能力"
```

### Task 3: Compact Keyword Automaton and Bounded Matcher

**Files:**
- Create: `internal/service/moderation/textnorm/normalize.go`
- Create: `internal/service/moderation/textnorm/regexp.go`
- Modify: `internal/service/moderation/normalize.go`
- Modify: `internal/service/moderation/regexp.go`
- Create: `internal/service/moderation/ruleindex/types.go`
- Create: `internal/service/moderation/ruleindex/builder.go`
- Create: `internal/service/moderation/ruleindex/compact.go`
- Create: `internal/service/moderation/ruleindex/matcher.go`
- Create: `internal/service/moderation/ruleindex/builder_test.go`
- Create: `internal/service/moderation/ruleindex/matcher_test.go`

**Interfaces:**
- Produces: `ruleindex.SourceRule`, `ruleindex.Limits`, `ruleindex.Snapshot`, `ruleindex.Stats`, `ruleindex.MatchResult`.
- Produces: `ruleindex.Build(ctx, version, source, limits)` and `(*Snapshot).Match(normalized)`.

- [ ] **Step 1: Move normalization behind a shared package with compatibility tests**

```go
func TestCompatibilityWrapperMatchesSharedNormalizer(t *testing.T) {
    raw := "違－禁ＡＡ"
    assert.Equal(t, textnorm.Normalize(raw), moderation.NormalizeText(raw))
    assert.Equal(t, "违禁a", textnorm.Normalize(raw))
}
```

Keep `moderation.NormalizeText` as a thin wrapper so existing callers and tests remain stable; move the actual implementation and normalized-regexp rune handling into `textnorm`.

- [ ] **Step 2: Write failing automaton behavior and adversarial-prefix tests**

```go
func TestMatchAppliesAllowAndKeepsHighestRiskAfterIDCap(t *testing.T) {
    rules := []ruleindex.SourceRule{
        {ID: 1, Type: "keyword", Pattern: "敏感", Risk: "high", Effect: "review"},
        {ID: 2, Type: "keyword", Pattern: "非敏感示例", Risk: "low", Effect: "allow"},
        {ID: 3, Type: "keyword", Pattern: "风险", Risk: "medium", Effect: "review"},
    }
    snapshot := buildSnapshot(t, rules, ruleindex.Limits{MaxMatchIDs: 1, MaxPatternRunes: 500})
    got := snapshot.Match(textnorm.Normalize("非敏感示例 风险"))
    assert.Equal(t, ruleindex.RiskMedium, got.Risk)
    assert.Equal(t, []uint64{3}, got.RuleIDs)
    assert.False(t, got.Truncated)
}

func TestBuilderDoesNotExpandFailureOutputs(t *testing.T) {
    rules := nestedRules(500)
    snapshot := buildSnapshot(t, rules, defaultLimits())
    assert.Equal(t, len(rules), snapshot.Stats().DirectOutputCount)
}

func buildSnapshot(t *testing.T, rules []ruleindex.SourceRule, limits ruleindex.Limits) *ruleindex.Snapshot {
    t.Helper()
    source := func(ctx context.Context, visit func(ruleindex.SourceRule) error) error {
        for _, rule := range rules { if err := visit(rule); err != nil { return err } }
        return nil
    }
    snapshot, _, err := ruleindex.Build(context.Background(), 1, source, limits)
    require.NoError(t, err)
    return snapshot
}

func defaultLimits() ruleindex.Limits {
    return ruleindex.Limits{MaxKeywordRules: 500000, MaxRegexpRules: 200, MaxPatternRunes: 500, MaxMatchIDs: 128}
}

func nestedRules(count int) []ruleindex.SourceRule {
    rules := make([]ruleindex.SourceRule, 0, count)
    var pattern strings.Builder
    for i := 0; i < count; i++ {
        pattern.WriteRune(rune(0x4E00 + i))
        rules = append(rules, ruleindex.SourceRule{ID: uint64(i + 1), Type: "keyword", Pattern: pattern.String(), Risk: "medium", Effect: "review"})
    }
    return rules
}
```

- [ ] **Step 3: Run tests and verify they fail**

Run: `go test ./internal/service/moderation/... -run 'CompatibilityWrapper|MatchApplies|DoesNotExpand' -count=1`

Expected: FAIL because `textnorm` and `ruleindex` do not exist.

- [ ] **Step 4: Implement builder and immutable runtime layout**

```go
type Source func(context.Context, func(SourceRule) error) error

type Snapshot struct {
    version uint64
    states  []state
    edges   []edge
    outputs []uint32
    ruleIDs []uint64
    risks   []Risk
    effects []Effect
    lengths []uint16
    regexps []regexpRule
    stats   Stats
}

type state struct {
    edgeStart, edgeCount uint32
    failure, suffix      uint32
    outputStart, outputCount uint32
    longestAllow         uint16
}

func Build(ctx context.Context, version uint64, source Source, limits Limits) (*Snapshot, Stats, error)
```

During construction use one global transition map keyed by `(state,rune)` plus linked edge arrays; do not allocate one map per state. Compact each state's edges into rune-sorted slices for binary-search lookup, compute failure/suffix links by BFS, and keep only direct terminal outputs.

- [ ] **Step 5: Implement two-pass matching with bounded retained IDs**

First pass marks allow coverage using `longestAllow`. Second pass follows direct output and suffix links, evaluates every unsuppressed review output for risk, and stores at most `MaxMatchIDs` unique IDs in deterministic risk/priority/ID order. Set `Truncated` when another unsuppressed ID is observed after the cap; do not stop risk evaluation.

```go
type MatchResult struct {
    Risk          Risk
    RuleIDs       []uint64
    SuppressedIDs []uint64
    Truncated     bool
}
```

- [ ] **Step 6: Run ruleindex and normalization tests**

Run: `go test ./internal/service/moderation/... -count=1`

Expected: PASS.

- [ ] **Step 7: Commit the compact matcher**

```bash
git add internal/service/moderation/textnorm internal/service/moderation/ruleindex internal/service/moderation/normalize.go internal/service/moderation/regexp.go
git commit -m "perf(moderation): 新增紧凑关键词匹配索引"
```

### Task 4: Index Memory Accounting and Stream Codec

**Files:**
- Create: `internal/service/moderation/ruleindex/memory.go`
- Create: `internal/service/moderation/ruleindex/codec.go`
- Create: `internal/service/moderation/ruleindex/codec_test.go`
- Create: `internal/service/moderation/ruleindex/benchmark_test.go`

**Interfaces:**
- Produces: `(*Snapshot).EncodedSize()`, `(*Snapshot).WriteTo(io.Writer)`, and `ruleindex.ReadFrom(io.Reader, Limits)`.
- Produces: `Stats.IndexBytes` and `Stats.BuildPeakBytes` used by publishing.

- [ ] **Step 1: Write failing round-trip, corrupt-header, and budget tests**

```go
func TestCodecRoundTripPreservesMatchResult(t *testing.T) {
    before := buildSnapshot(t, []ruleindex.SourceRule{{ID: 1, Type: "keyword", Pattern: "风险", Risk: "medium", Effect: "review"}}, defaultLimits())
    var buf bytes.Buffer
    _, err := before.WriteTo(&buf)
    require.NoError(t, err)
    after, _, err := ruleindex.ReadFrom(&buf, defaultLimits())
    require.NoError(t, err)
    assert.Equal(t, before.Match("风险文本"), after.Match("风险文本"))
}

func TestReadRejectsDeclaredArrayBeforeAllocating(t *testing.T) {
    raw := encodedHeaderWithStateCount(math.MaxUint32)
    _, _, err := ruleindex.ReadFrom(bytes.NewReader(raw), defaultLimits())
    assert.ErrorIs(t, err, ruleindex.ErrIndexLimit)
}
```

- [ ] **Step 2: Run codec tests and verify failure**

Run: `go test ./internal/service/moderation/ruleindex -run 'Codec|ReadRejects|Budget' -count=1`

Expected: FAIL because codec and accounting functions do not exist.

- [ ] **Step 3: Implement fixed-format streaming codec**

Use little-endian fixed-width fields, magic `BMR1`, format version `1`, ruleset version, counts, then contiguous arrays. Hash bytes while writing and reading. Validate every count against configured rule count and memory limits before allocating. Serialize regexp/composite source patterns because they must be recompiled after load; never serialize Go pointers.

```go
const magic = "BMR1"
const FormatVersion uint32 = 1

func (s *Snapshot) WriteTo(w io.Writer) (string, error)
func (s *Snapshot) EncodedSize() int64
func ReadFrom(r io.Reader, limits Limits) (*Snapshot, string, error)
```

- [ ] **Step 4: Implement exact retained-byte and conservative peak accounting**

Count `cap(slice) * sizeof(element)` for every retained and build-only slice/map estimate, then apply a documented 20% safety margin to peak estimates. Return `ErrIndexLimit` before growing beyond either configured threshold.

- [ ] **Step 5: Run codec tests and deterministic benchmarks**

Run: `go test ./internal/service/moderation/ruleindex -count=1`

Run: `go test ./internal/service/moderation/ruleindex -run '^$' -bench 'Benchmark(Build|Match)' -benchmem`

Expected: tests PASS; benchmark prints allocations and bytes without a hard timing assertion.

- [ ] **Step 6: Commit codec and accounting**

```bash
git add internal/service/moderation/ruleindex
git commit -m "feat(moderation): 增加规则索引流式编解码"
```

### Task 5: Streaming Ruleset Repository and Classifier Integration

**Files:**
- Create: `internal/repository/moderationrule/repository.go`
- Create: `internal/repository/moderationrule/types.go`
- Create: `internal/repository/moderationrule/snapshot.go`
- Create: `internal/repository/moderationrule/snapshot_test.go`
- Delete: `internal/repository/moderation/rules.go`
- Modify: `internal/repository/moderation/repository.go`
- Modify: `internal/repository/moderation/mock/mock_repository.go`
- Modify: `internal/service/moderation/types.go`
- Modify: `internal/service/moderation/classifier.go`
- Modify: `internal/service/moderation/classifier_test.go`
- Modify: `internal/service/moderation/service_mapping.go`
- Modify: `internal/service/moderation/service_write.go`
- Modify: `internal/service/moderation/service_test.go`
- Modify: `internal/router/moderation.go`
- Modify: `internal/router/moderation_test.go`

**Interfaces:**
- Produces: `moderationrule.SnapshotRepository.CurrentRuleset` and `StreamRules`.
- Changes: `moderation.Classifier.ReplaceSnapshot(*ruleindex.Snapshot)`.
- Changes: `moderation.Classification.RuleMatchesTruncated`.

- [ ] **Step 1: Write failing repository streaming tests**

```go
func TestStreamRulesExcludesFailedCandidatesAndClosesRows(t *testing.T) {
    mock.ExpectQuery("SELECT .* FROM `moderation_rule`.*JOIN `moderation_ruleset`").
        WithArgs(uint64(7), uint64(7)).
        WillReturnRows(sqlmock.NewRows(ruleColumns()).AddRow(activeRuleValues()...))
    var ids []uint64
    err := repo.StreamRules(context.Background(), 7, func(rule moderationrule.RuleRecord) error {
        ids = append(ids, rule.ID)
        return nil
    })
    require.NoError(t, err)
    assert.Equal(t, []uint64{11}, ids)
    require.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: Run repository/classifier tests and verify failure**

Run: `go test ./internal/repository/moderationrule ./internal/service/moderation ./internal/router -run 'StreamRules|Classifier|ModerationService' -count=1`

Expected: FAIL because the new repository package and signatures do not exist.

- [ ] **Step 3: Implement callback-based streaming repository**

```go
type SnapshotRepository interface {
    CurrentRuleset(ctx context.Context) (RulesetRecord, error)
    StreamRules(ctx context.Context, version uint64, visit func(RuleRecord) error) error
}
```

Use `Rows()` and `defer rows.Close()`; scan one row at a time. Join the activation ruleset and require status `published` or `superseded`; include rules with `activated_ruleset_id <= version` and no deactivation at or before the version. Order by rule ID only.

- [ ] **Step 4: Replace the linear classifier with one immutable snapshot**

`NewClassifierFromRepository` must stream rules directly into `ruleindex.Build`; it must not create `[]RuleRecord` or `[]CompiledRule`. `Classify` normalizes once, calls `Snapshot.Match`, and maps risk, IDs, version, and truncation. Keep the cold-load medium-risk behavior and monotonic version check.

- [ ] **Step 5: Persist truncation through revisions and blocked attempts**

```go
type Classification struct {
    Risk                 RiskLevel
    RuleMatchIDs         []uint64
    RuleMatchesTruncated bool
    RulesetVersion       uint64
}
```

Map this field into `RevisionDraft` and `BlockedAttempt`, marshal only the bounded ID slice, and update sqlmock expectations.

- [ ] **Step 6: Run repository, service, router, and race tests**

Run: `go test ./internal/repository/moderationrule ./internal/repository/moderation ./internal/service/moderation ./internal/router -count=1`

Run: `go test -race ./internal/service/moderation/... -count=1`

Expected: PASS.

- [ ] **Step 7: Commit classifier integration**

```bash
git add internal/repository/moderationrule internal/repository/moderation internal/service/moderation internal/router
git commit -m "perf(moderation): 接入流式规则索引运行时"
```

### Task 6: Foundation Verification and Capacity Report

**Files:**
- Create: `internal/service/moderation/ruleindex/capacity_test.go`
- Create: `docs/moderation-rule-index-benchmark.md`

**Interfaces:**
- Consumes: the complete foundation from Tasks 1–5.
- Produces: reproducible benchmark commands and measured baseline; no production API changes.

- [ ] **Step 1: Add deterministic 100k/500k generators and repeated-build checks**

```go
func BenchmarkBuild100K(b *testing.B) { benchmarkBuild(b, 100_000) }
func BenchmarkBuild500K(b *testing.B) { benchmarkBuild(b, 500_000) }

func TestRepeatedBuildKeepsOnlyReferencedSnapshots(t *testing.T) {
    var current atomic.Pointer[ruleindex.Snapshot]
    for i := 1; i <= 20; i++ {
        current.Store(buildGeneratedSnapshot(t, uint64(i), 10_000))
    }
    assert.Equal(t, uint64(20), current.Load().Version())
}

func buildGeneratedSnapshot(t *testing.T, version uint64, count int) *ruleindex.Snapshot {
    t.Helper()
    rules := make([]ruleindex.SourceRule, 0, count)
    for i := 0; i < count; i++ {
        pattern := fmt.Sprintf("风险词%c%c", rune(0x3400+i/20000), rune(0x4E00+i%20000))
        rules = append(rules, ruleindex.SourceRule{ID: uint64(i + 1), Type: "keyword", Pattern: pattern, Risk: "medium", Effect: "review"})
    }
    source := func(ctx context.Context, visit func(ruleindex.SourceRule) error) error {
        for _, rule := range rules { if err := visit(rule); err != nil { return err } }
        return nil
    }
    snapshot, _, err := ruleindex.Build(context.Background(), version, source, defaultLimits())
    require.NoError(t, err)
    return snapshot
}
```

- [ ] **Step 2: Run full backend verification**

Run: `go test ./... -count=1`

Run: `go test -race ./internal/service/moderation/... ./internal/repository/moderationrule -count=1`

Expected: PASS.

- [ ] **Step 3: Run capacity benchmarks and record actual output**

Run: `go test ./internal/service/moderation/ruleindex -run '^$' -bench 'Benchmark(Build100K|Build500K|Match)' -benchmem -count=1`

Expected: command completes within configured build limits. Record machine, rule count, nodes, edges, direct outputs, artifact bytes, retained bytes, peak estimate, build duration, match ns/op, and allocs/op in `docs/moderation-rule-index-benchmark.md`.

- [ ] **Step 4: Commit foundation verification**

```bash
git add internal/service/moderation/ruleindex/capacity_test.go docs/moderation-rule-index-benchmark.md
git commit -m "test(moderation): 增加大规模规则索引基准"
```
