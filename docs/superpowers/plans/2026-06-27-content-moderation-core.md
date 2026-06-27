# Content Moderation Core Implementation Plan

> **Required execution skill:** Use `superpowers:subagent-driven-development` when delegating independent tasks, or `superpowers:executing-plans` when executing this plan in a separate session.

**Goal:** 为评论、回复、留言和碎语建立统一的文本风险判定、版本状态机与业务接入，使低风险内容先发后审、中风险内容先审后发、高风险内容明确拒绝，并保证待审内容不可互动、删除不可恢复。

**Architecture:** 新增独立 `moderation` service/repository。纯函数分类器、策略器和状态机只生成决策；repository 在单个 MySQL 事务中锁定审核项、写审核版本并通过私有 subject adapter 物化业务表。原 comment、guestbook、moment service 只依赖 moderation 门面，不直接操作审核表。

**Tech Stack:** Go 1.25.5、Gin、GORM/MySQL、Viper、zap、bluemonday v1.0.27、go.uber.org/mock v0.6.0、testify、go-sqlmock。

**Global Constraints:**

- 遵守 `AGENTS.md`、`go-layering`、`go-readability`、`go-testing` 和 `http-api`。
- 基础设施必须构造注入；生产代码禁止 `fmt.Println`。
- HTTP DTO 与 Swagger 禁止暴露 `model.*`。
- 所有发布与编辑请求必须携带 `Idempotency-Key`。
- 本阶段不实现图片审核、人工审核、举报、申诉、自动处罚和全站控制管理接口。
- Media 阶段完成前，含图片的普通用户发布或编辑统一返回 `409 / CONTENT_IMAGE_REVIEW_UNAVAILABLE`；三个阶段完整部署前不得开放新版写入。
- 状态枚举和不变量固定在代码；阈值、策略、限制和文案放入强类型 config。

## File Map

**Create:**

- `migrations/20260627_content_moderation_core.sql`
- `pkg/config/moderation.go`
- `internal/model/moderation_content.go`
- `internal/model/moderation_rule.go`
- `internal/model/moderation_user.go`
- `internal/model/moderation_control.go`
- `internal/service/moderation/doc.go`
- `internal/service/moderation/types.go`
- `internal/service/moderation/errors.go`
- `internal/service/moderation/content.go`
- `internal/service/moderation/classifier.go`
- `internal/service/moderation/policy.go`
- `internal/service/moderation/transition.go`
- `internal/service/moderation/service.go`
- `internal/service/moderation/*_test.go`
- `internal/service/moderation/mock/mock_service.go`
- `internal/repository/moderation/doc.go`
- `internal/repository/moderation/types.go`
- `internal/repository/moderation/repository.go`
- `internal/repository/moderation/query.go`
- `internal/repository/moderation/rules.go`
- `internal/repository/moderation/transition.go`
- `internal/repository/moderation/subject_comment.go`
- `internal/repository/moderation/subject_guestbook.go`
- `internal/repository/moderation/subject_moment.go`
- `internal/repository/moderation/*_test.go`
- `internal/repository/moderation/mock/mock_repository.go`
- `internal/dto/moderation.go`

**Modify:**

- `go.mod`、`go.sum`
- `config/config.yaml`、`config/config.test.yaml`、`config/config.prod.yaml`、`config/config.local.yaml.example`
- `pkg/config/config.go`、`pkg/config/config_test.go`
- `internal/dbschema/schema.go`、`internal/dbschema/seed.go`
- `pkg/response/response.go`
- `internal/dto/comment.go`、`internal/dto/guestbook.go`、`internal/dto/moment.go`
- `internal/repository/comment/*`、`internal/repository/guestbook/*`、`internal/repository/moment/*`
- `internal/service/comment/*`、`internal/service/guestbook/*`、`internal/service/moment/*`
- `internal/handler/comment/*`、`internal/handler/guestbook/*`、`internal/handler/moment/*`
- `internal/middleware/ratelimit.go`、`internal/middleware/ratelimit_test.go`
- `internal/router/router.go`

---

### Task 1: Add strongly typed moderation configuration

**Files:**

- Create: `pkg/config/moderation.go`
- Modify: `pkg/config/config.go`
- Modify: `pkg/config/config_test.go`
- Modify: `config/config.yaml`
- Modify: `config/config.test.yaml`
- Modify: `config/config.prod.yaml`
- Modify: `config/config.local.yaml.example`

- [ ] **Step 1: Write failing config decode and validation tests**

Add table-driven tests for valid defaults and these failures: production disabled, production `observe`, empty notices, invalid action, `high != block`, restricted post-review/auto-approve, non-positive bounds.

```go
func TestValidateModeration(t *testing.T) {
    tests := []struct {
        name    string
        mutate  func(*config.ModerationConfig)
        env     string
        wantErr string
    }{
        {"production requires enforce", func(c *config.ModerationConfig) {
            c.Mode = "observe"
        }, "production", "moderation.mode"},
        {"high is always blocked", func(c *config.ModerationConfig) {
            c.Policy.Normal.High = "pre_review"
        }, "test", "high"},
    }
    // 使用默认合法配置逐项变异并断言错误字段。
}
```

- [ ] **Step 2: Run the focused test and confirm failure**

Run: `go test ./pkg/config -run Moderation -count=1`
Expected: FAIL because `ModerationConfig` and validation do not exist.

- [ ] **Step 3: Implement config types and validation**

Use explicit nested structs and constants; do not use `map[string]any`.

```go
type ModerationConfig struct {
    Enabled bool                    `mapstructure:"enabled"`
    Mode    string                  `mapstructure:"mode"`
    Policy  ModerationPolicyConfig  `mapstructure:"policy"`
    Rules   ModerationRulesConfig   `mapstructure:"rules"`
    Content ModerationContentConfig `mapstructure:"content"`
    Notices ModerationNoticesConfig `mapstructure:"notices"`
}

func (c ModerationConfig) Validate(environment string) error {
    if environment == "production" && (!c.Enabled || c.Mode != ModerationModeEnforce) {
        return errors.New("production requires moderation.enabled=true and moderation.mode=enforce")
    }
    return validateModerationPolicy(c.Policy)
}
```

Core 只解析本阶段使用的 `policy`、`rules`、`content`、`rate_limit` 和 `notices`；同时为设计中后续 image/governance/control/audit 段定义强类型字段，避免后续改配置结构。

- [ ] **Step 4: Add documented defaults to every config file**

Keep production at `enabled: true` and `mode: enforce`. Test config must use a nonempty seeded ruleset.

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/config -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/config config
git commit -m "feat(moderation): 新增审核强类型配置"
```

### Task 2: Add core schema, models, and seeds

**Files:**

- Create: `migrations/20260627_content_moderation_core.sql`
- Create: `internal/model/moderation_content.go`
- Create: `internal/model/moderation_rule.go`
- Create: `internal/model/moderation_user.go`
- Create: `internal/model/moderation_control.go`
- Modify: `internal/dbschema/schema.go`
- Modify: `internal/dbschema/seed.go`

- [ ] **Step 1: Write schema contract tests**

Add reflection tests for exact table names, indexes and non-GORM-delete lifecycle semantics.

```go
func TestModerationItemUsesExplicitDeletedAt(t *testing.T) {
    typ := reflect.TypeOf(model.ModerationItem{})
    field, ok := typ.FieldByName("DeletedAt")
    require.True(t, ok)
    assert.Equal(t, reflect.TypeOf((*time.Time)(nil)), field.Type)
}
```

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./internal/model ./internal/dbschema -run Moderation -count=1`
Expected: FAIL because models and migration registration are absent.

- [ ] **Step 3: Define closed enums and persistence models**

Create:

- `moderation_item`
- `moderation_revision`
- `moderation_attempt`
- `moderation_rule`
- `moderation_action_log`
- `moderation_visible_image`
- `user_moderation_profile`
- `moderation_control`

```go
type ModerationItem struct {
    ID                     uint64     `gorm:"primaryKey"`
    ContentType            string     `gorm:"size:40;not null;uniqueIndex:uk_moderation_subject"`
    ContentID              uint64     `gorm:"not null;uniqueIndex:uk_moderation_subject"`
    AuthorID               uint64     `gorm:"not null;index"`
    LifecycleState         string     `gorm:"size:16;not null"`
    PublicState            string     `gorm:"size:24;not null"`
    MaterializedRevisionID *uint64
    ApprovedRevisionID     *uint64
    PendingRevisionID      *uint64
    StateBeforeEmergency   *string    `gorm:"size:24"`
    DeletedAt              *time.Time `gorm:"index"`
    LockVersion            uint64     `gorm:"not null;default:1"`
}
```

`moderation_revision` must preserve immutable `submitted_content` and sanitized `published_content`, with unique `(item_id, version)` and `(submitter_id, idempotency_key)`. `moderation_attempt` must not store full blocked text or its digest.

- [ ] **Step 4: Add SQL migration and idempotent seeds**

Seed at least one harmless low-risk baseline rule and one disabled example rule. Initialize the singleton control row. Add check constraints where MySQL supports them and mirror all constraints in service validation.

- [ ] **Step 5: Register AutoMigrate and seed order**

Ensure referenced tables migrate before foreign-key dependants and seeds use `clause.OnConflict`.

- [ ] **Step 6: Verify**

Run: `go test ./internal/model ./internal/dbschema -count=1`
Expected: PASS.

Run: `rg -n "gorm.DeletedAt|DeletedAt" internal/model/moderation_*.go`
Expected: moderation models contain only explicit `*time.Time` deletion fields.

- [ ] **Step 7: Commit**

```bash
git add migrations/20260627_content_moderation_core.sql internal/model internal/dbschema
git commit -m "feat(moderation): 新增审核核心数据模型"
```

### Task 3: Implement sanitization, normalization, and text classification

**Files:**

- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/service/moderation/doc.go`
- Create: `internal/service/moderation/types.go`
- Create: `internal/service/moderation/content.go`
- Create: `internal/service/moderation/classifier.go`
- Create: `internal/service/moderation/classifier_test.go`

- [ ] **Step 1: Add failing behavior tests**

Cover:

- script/event attribute/javascript protocol removal;
- Unicode NFKC, case, full-width, zero-width and repeated separator normalization;
- keyword, Go RE2 regex and combined signals;
- highest matched risk wins;
- invalid regex snapshot rejected;
- empty/enforce or cold-load failure degrades ordinary submissions to medium;
- last good immutable snapshot survives refresh failure.

```go
func TestClassifierUsesHighestRisk(t *testing.T) {
    classifier := moderation.NewClassifier(testLogger, validSnapshot(
        rule("ad", moderation.RiskMedium, "加微"),
        rule("blocked", moderation.RiskHigh, "违禁"),
    ))
    got := classifier.Classify("加 微，违\u200b禁")
    assert.Equal(t, moderation.RiskHigh, got.Risk)
    assert.ElementsMatch(t, []uint64{1, 2}, got.RuleMatchIDs)
}
```

- [ ] **Step 2: Confirm red**

Run: `go test ./internal/service/moderation -run 'Sanitize|Normalize|Classifier' -count=1`
Expected: FAIL because classifier APIs do not exist.

- [ ] **Step 3: Add sanitizer dependency and content pipeline**

Run: `go get github.com/microcosm-cc/bluemonday@v1.0.27`

```go
type ContentProcessor interface {
    Process(raw string, limit int) (ProcessedContent, error)
}

type ProcessedContent struct {
    Published string
    PlainText string
    Links    []string
}
```

Use an allowlist policy. Classification always receives sanitized plain text; publishing uses sanitized output.

- [ ] **Step 4: Implement immutable rule snapshots**

```go
type RuleSnapshot struct {
    Version uint64
    Rules   []CompiledRule
}

type Classifier interface {
    Classify(processed ProcessedContent) Classification
    ReplaceSnapshot(snapshot RuleSnapshot) error
}
```

Compile all rules before atomic replacement. Never mutate a live snapshot.

- [ ] **Step 5: Verify**

Run: `go test ./internal/service/moderation -run 'Sanitize|Normalize|Classifier' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/service/moderation
git commit -m "feat(moderation): 实现文本风险分类器"
```

### Task 4: Implement policy and pure transition engine

**Files:**

- Create: `internal/service/moderation/errors.go`
- Create: `internal/service/moderation/policy.go`
- Create: `internal/service/moderation/transition.go`
- Create: `internal/service/moderation/policy_test.go`
- Create: `internal/service/moderation/transition_test.go`

- [ ] **Step 1: Write policy matrix tests**

Test every trust/risk combination, admin bypass, muted/banned, publishing closed/pre-review-all, restricted restrictions, and the hard invariant `high => block`.

```go
func TestPolicyMatrix(t *testing.T) {
    tests := []struct {
        trust string
        risk  moderation.RiskLevel
        want  moderation.PolicyAction
    }{
        {moderation.TrustNew, moderation.RiskLow, moderation.ActionPostReview},
        {moderation.TrustNormal, moderation.RiskMedium, moderation.ActionPreReview},
        {moderation.TrustTrusted, moderation.RiskLow, moderation.ActionAutoApprove},
        {moderation.TrustRestricted, moderation.RiskLow, moderation.ActionPreReview},
    }
    // 对每行调用 Decide 并断言动作。
}
```

- [ ] **Step 2: Write transition matrix and invariant tests**

Cover initial submit, low edit, medium edit, high block (no state mutation), resubmit superseding pending, idempotent retry, delete/admin_delete from every active state, repeated delete, and every illegal post-delete event.

```go
func TestDeletedIsTerminal(t *testing.T) {
    events := []moderation.Event{
        moderation.EventSubmit, moderation.EventResubmit,
        moderation.EventApprove, moderation.EventReject,
        moderation.EventCorrectAndApprove,
        moderation.EventEmergencyHide, moderation.EventRestore,
    }
    for _, event := range events {
        _, err := moderation.Transition(deletedSnapshot(), moderation.TransitionInput{
            Event: event,
            Now:   fixedTime,
        })
        assert.ErrorIs(t, err, moderation.ErrAlreadyDeleted)
    }
}
```

- [ ] **Step 3: Confirm red**

Run: `go test ./internal/service/moderation -run 'Policy|Transition|Deleted' -count=1`
Expected: FAIL because policy and transition engine are absent.

- [ ] **Step 4: Implement pure decisions**

```go
type TransitionInput struct {
    Event          Event
    Action         PolicyAction
    NewRevisionID  uint64
    Previous       ItemSnapshot
    Now            time.Time
}

type TransitionPlan struct {
    Item               ItemSnapshot
    MaterializeRevision *uint64
    SupersedeRevision   *uint64
    AppendLog           ActionLogIntent
}
```

The engine must not import GORM, Redis, zap, Gin or repository packages. It must derive `display_version` and `can_interact` from snapshot state rather than persisting them.

- [ ] **Step 5: Verify exhaustive tests**

Run: `go test ./internal/service/moderation -run 'Policy|Transition|Deleted' -count=1`
Expected: PASS.

Run: `go list -deps ./internal/service/moderation | rg 'gorm|gin-gonic|go-redis'`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add internal/service/moderation
git commit -m "feat(moderation): 实现审核策略与状态机"
```

### Task 5: Implement moderation repository and subject adapters

**Files:**

- Create: `internal/repository/moderation/doc.go`
- Create: `internal/repository/moderation/types.go`
- Create: `internal/repository/moderation/repository.go`
- Create: `internal/repository/moderation/query.go`
- Create: `internal/repository/moderation/rules.go`
- Create: `internal/repository/moderation/transition.go`
- Create: `internal/repository/moderation/subject_comment.go`
- Create: `internal/repository/moderation/subject_guestbook.go`
- Create: `internal/repository/moderation/subject_moment.go`
- Create: `internal/repository/moderation/repository_test.go`

- [ ] **Step 1: Write sqlmock transaction tests**

Test row locking, optimistic lock mismatch, version creation, pending supersede, subject materialization, rollback on adapter failure, blocked attempt idempotency, and rule snapshot loading.

```go
func TestApplyTransitionRollsBackWhenMaterializationFails(t *testing.T) {
    db, mock := newSQLMockDB(t)
    mock.ExpectBegin()
    mock.ExpectQuery("SELECT .*moderation_items.*FOR UPDATE").
        WillReturnRows(activeItemRows())
    mock.ExpectExec("UPDATE .*comments").
        WillReturnError(errors.New("write failed"))
    mock.ExpectRollback()

    err := repo.ApplyTransition(ctx, command)
    require.Error(t, err)
    require.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: Confirm red**

Run: `go test ./internal/repository/moderation -count=1`
Expected: FAIL because repository does not exist.

- [ ] **Step 3: Define repository boundary**

```go
type Repository interface {
    LoadSubject(ctx context.Context, ref SubjectRef) (SubjectSnapshot, error)
    LoadPolicyContext(ctx context.Context, userID uint64) (PolicyContext, error)
    FindResultByIdempotencyKey(ctx context.Context, userID uint64, key string) (*StoredResult, error)
    ApplyTransition(ctx context.Context, cmd ApplyTransitionCommand) (AppliedTransition, error)
    RecordBlockedAttempt(ctx context.Context, attempt BlockedAttempt) (StoredResult, error)
    LoadEnabledRules(ctx context.Context) ([]RuleRecord, error)
    LoadModerationView(ctx context.Context, refs []SubjectRef, viewer Viewer) (map[SubjectRef]View, error)
}
```

Service passes data commands only; no `func(*gorm.DB)` callback crosses the boundary.
`LoadPolicyContext` reads the user profile and singleton publishing control consistently. A missing profile is interpreted as `new + active`; Core does not auto-promote or auto-sanction users.

- [ ] **Step 4: Implement private subject adapters**

```go
type subjectAdapter interface {
    Materialize(ctx context.Context, tx *gorm.DB, cmd MaterializeCommand) error
    Delete(ctx context.Context, tx *gorm.DB, ref SubjectRef) error
    Descendants(ctx context.Context, tx *gorm.DB, ref SubjectRef) ([]SubjectRef, error)
}
```

Map both article and moment comments/replies to the existing comment tables; guestbook and moment remain separate adapters. Validate author ownership and parent relation inside the transaction.

- [ ] **Step 5: Verify**

Run: `go test ./internal/repository/moderation -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/repository/moderation
git commit -m "feat(moderation): 实现审核事务仓储"
```

### Task 6: Build the moderation application service and generated mocks

**Files:**

- Create: `internal/service/moderation/service.go`
- Create: `internal/service/moderation/service_test.go`
- Create: `internal/service/moderation/mock/mock_service.go`
- Create: `internal/repository/moderation/mock/mock_repository.go`

- [ ] **Step 1: Write orchestration tests with gomock**

Test:

- low/medium/high first submit;
- low/medium/high edit;
- idempotent retries return the original result;
- image-containing request returns `ErrImageReviewUnavailable` before classification;
- blocked attempt audit failure still rejects and logs structured error;
- medium public result contains no pending body;
- admin submit becomes approved.

- [ ] **Step 2: Confirm red**

Run: `go test ./internal/service/moderation -run Service -count=1`
Expected: FAIL because application service is incomplete.

- [ ] **Step 3: Implement narrow service facade**

```go
type Service interface {
    Submit(ctx context.Context, cmd SubmitCommand) (SubmitResult, error)
    Edit(ctx context.Context, cmd EditCommand) (SubmitResult, error)
    Delete(ctx context.Context, cmd DeleteCommand) error
    AssertCanInteract(ctx context.Context, ref SubjectRef) error
    LoadViews(ctx context.Context, refs []SubjectRef, viewer Viewer) (map[SubjectRef]View, error)
}
```

Flow: validate key and bounds → reject images in Core → idempotency lookup → load actor/control policy context → sanitize → classify → decide policy → build pure transition → persist. Return stable public message and business error without rule IDs.

- [ ] **Step 4: Generate mocks**

```bash
go run go.uber.org/mock/mockgen@v0.6.0 \
  -source=internal/service/moderation/service.go \
  -destination=internal/service/moderation/mock/mock_service.go \
  -package=mock
go run go.uber.org/mock/mockgen@v0.6.0 \
  -source=internal/repository/moderation/repository.go \
  -destination=internal/repository/moderation/mock/mock_repository.go \
  -package=mock
```

- [ ] **Step 5: Verify**

Run: `go test ./internal/service/moderation ./internal/repository/moderation -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/moderation internal/repository/moderation
git commit -m "feat(moderation): 新增审核应用服务"
```

### Task 7: Add moderation DTO projection to content reads

**Files:**

- Create: `internal/dto/moderation.go`
- Modify: `internal/dto/comment.go`
- Modify: `internal/dto/guestbook.go`
- Modify: `internal/dto/moment.go`
- Modify: `internal/repository/comment/*`
- Modify: `internal/repository/guestbook/*`
- Modify: `internal/repository/moment/*`
- Modify: related repository/service/handler tests

- [ ] **Step 1: Write public, author, and admin projection tests**

Assert:

- public low/post-review receives sanitized body plus pending marker;
- public medium receives placeholder and no body;
- author/admin receives `pending_content`, `pending_risk_level` and `review_status`;
- last-approved display never leaks pending medium text;
- list loading batches moderation views and has no N+1 query;
- every pending state returns `can_interact=false`.

```go
type ModerationView struct {
    PublicState        string  `json:"public_state"`
    DisplayVersion     string  `json:"display_version"`
    HasPendingRevision bool    `json:"has_pending_revision"`
    PendingRiskLevel   *string `json:"pending_risk_level,omitempty"`
    ReviewStatus       *string `json:"review_status,omitempty"`
    PendingContent     *string `json:"pending_content,omitempty"`
    CanInteract        bool    `json:"can_interact"`
}
```

- [ ] **Step 2: Confirm red**

Run: `go test ./internal/{repository,service,handler}/{comment,guestbook,moment} -run Moderation -count=1`
Expected: FAIL because DTOs and projections are absent.

- [ ] **Step 3: Implement DTO-only projection**

Business repositories continue returning internal entities. Services load moderation views in one batch, choose the safe visible body, and construct DTOs. Do not add moderation fields to public business models solely for serialization.

- [ ] **Step 4: Verify**

Run: `go test ./internal/{repository,service,handler}/{comment,guestbook,moment} -run Moderation -count=1`
Expected: PASS.

Run: `rg -n "json:|swagger:" internal/model`
Expected: no newly added public serialization tags for moderation.

- [ ] **Step 5: Commit**

```bash
git add internal/dto internal/repository internal/service internal/handler
git commit -m "feat(moderation): 返回内容审核展示状态"
```

### Task 8: Route all publish and edit flows through moderation

**Files:**

- Modify: `internal/service/comment/*`
- Modify: `internal/service/guestbook/*`
- Modify: `internal/service/moment/*`
- Modify: `internal/handler/comment/*`
- Modify: `internal/handler/guestbook/*`
- Modify: `internal/handler/moment/*`
- Modify: `internal/router/router.go`
- Modify: `internal/middleware/ratelimit.go`
- Modify: `internal/middleware/ratelimit_test.go`
- Modify: `pkg/response/response.go`
- Modify: related tests and generated mocks

- [ ] **Step 1: Write service tests for all seven subject types**

Cover moment, article/moment comments, guestbook and all three reply kinds. Assert business services call moderation and do not directly create/edit visible rows.

- [ ] **Step 2: Write HTTP tests**

Assert:

- missing/invalid `Idempotency-Key` → `400`;
- low → success with “发布成功，内容会被审核。”;
- medium → success with “内容已提交，等待人工审核。” and no body;
- high → `422 / CONTENT_RISK_REJECTED` with the configured risk message;
- high edit says the old version remains;
- images → `409 / CONTENT_IMAGE_REVIEW_UNAVAILABLE`;
- edit ownership and deleted checks are preserved.
- authenticated actor is read with the existing JWT claims helper; request body user IDs are ignored.
- configured per-user publish/edit limits return `429` before moderation work.

- [ ] **Step 3: Confirm red**

Run: `go test ./internal/service/comment ./internal/service/guestbook ./internal/service/moment ./internal/handler/comment ./internal/handler/guestbook ./internal/handler/moment -run 'Moderation|Idempotency|Risk' -count=1`
Expected: FAIL because write paths bypass moderation.

- [ ] **Step 4: Add request/response errors and routes**

Add `Conflict` and `UnprocessableEntity` helpers to `pkg/response` without exposing internal matches. Add PATCH edit routes for comments, replies and guestbook entries; retain the existing moment edit route shape.

```go
const (
    CodeContentRiskRejected         = "CONTENT_RISK_REJECTED"
    CodeImageReviewUnavailable      = "CONTENT_IMAGE_REVIEW_UNAVAILABLE"
    CodeContentAlreadyDeleted       = "CONTENT_ALREADY_DELETED"
    CodeContentPendingNoInteraction = "CONTENT_PENDING_NO_INTERACTION"
)
```

- [ ] **Step 5: Inject moderation service**

Update constructors and router wiring. Add a config-driven moderation write limiter that reuses the existing Redis principal limiter and fails open only according to the repository's established middleware policy. Regenerate every mock affected by constructor/interface changes using mockgen v0.6.0.

- [ ] **Step 6: Verify**

Run: `go test ./internal/service/comment ./internal/service/guestbook ./internal/service/moment ./internal/handler/comment ./internal/handler/guestbook ./internal/handler/moment -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal pkg/response
git commit -m "feat(moderation): 接入内容发布与编辑流程"
```

### Task 9: Enforce interaction guards server-side

**Files:**

- Modify: comment/reply create services and tests
- Modify: moment/comment like services and tests
- Modify: relevant handlers if error mapping changes

- [ ] **Step 1: Write failing guard tests**

For each supported like/reply/child-comment path, test pending, placeholder, hidden, emergency-hidden and deleted targets. Also test approved visible content succeeds.

```go
func TestCreateReplyRejectsPendingParent(t *testing.T) {
    moderationSvc.EXPECT().
        AssertCanInteract(gomock.Any(), parentRef).
        Return(moderation.ErrPendingNoInteraction)

    _, err := svc.CreateReply(ctx, input)
    assert.ErrorIs(t, err, moderation.ErrPendingNoInteraction)
    repo.AssertNotCalled(t, "CreateReply")
}
```

- [ ] **Step 2: Confirm red**

Run: `go test ./internal/service/... -run 'Pending.*Interact|Interact.*Pending' -count=1`
Expected: FAIL because writes only check business visibility.

- [ ] **Step 3: Guard before side effects**

Call `AssertCanInteract` before creating replies, likes, notifications or counters. Map failures to `409 / CONTENT_PENDING_NO_INTERACTION`. DTO `can_interact` is advisory; backend validation is authoritative.

- [ ] **Step 4: Verify**

Run: `go test ./internal/service/... ./internal/handler/... -run 'Interact|Pending' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service internal/handler
git commit -m "feat(moderation): 禁止待审内容互动"
```

### Task 10: Make delete terminal and cascade moderation tombstones

**Files:**

- Modify: `internal/service/moderation/transition.go`
- Modify: `internal/repository/moderation/transition.go`
- Modify: `internal/repository/moderation/subject_*.go`
- Modify: comment/guestbook/moment delete services and repositories
- Modify: related tests

- [ ] **Step 1: Write transaction and concurrency tests**

Cover:

- user/admin delete from each active state;
- parent delete discovers all direct and indirect descendants;
- item and descendants become lifecycle `deleted` in the same transaction as business cleanup;
- pending revisions become `superseded`;
- materialized/pending/emergency fields clear, approved pointer remains for audit;
- repeated delete is idempotent;
- edit or review racing after delete receives `CONTENT_ALREADY_DELETED`;
- deleted content cannot be restored by emergency restore.

- [ ] **Step 2: Confirm red**

Run: `go test ./internal/service/moderation ./internal/repository/moderation ./internal/service/comment ./internal/service/guestbook ./internal/service/moment -run 'Delete|Deleted|Cascade' -count=1`
Expected: FAIL because existing deletes bypass moderation tombstones.

- [ ] **Step 3: Implement one transactional delete command**

```go
type DeleteCommand struct {
    Subject  SubjectRef
    ActorID  uint64
    IsAdmin  bool
    Event    Event
    Now      time.Time
}
```

Repository locks parent then descendants in stable `content_type, content_id` order to reduce deadlocks. It applies terminal transitions before reusing existing business relation cleanup. Never hard-delete moderation items or revisions.

- [ ] **Step 4: Verify**

Run: `go test ./internal/service/moderation ./internal/repository/moderation ./internal/service/comment ./internal/service/guestbook ./internal/service/moment -run 'Delete|Deleted|Cascade' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service internal/repository
git commit -m "feat(moderation): 实现删除终态与级联墓碑"
```

### Task 11: Complete dependency wiring, API docs, and Core verification

**Files:**

- Modify: `internal/router/router.go`
- Modify: Swagger annotations in modified handlers
- Modify: `docs/docs.go`
- Modify: `docs/swagger.json`
- Modify: `docs/swagger.yaml`
- Modify: any affected constructors, mocks and tests

- [ ] **Step 1: Add startup integration tests**

Assert configuration validates before serving, rules load before ordinary writes, failed initial enforce load prevents unsafe startup or activates the specified medium-risk fallback, and all handlers receive the same injected moderation service.

- [ ] **Step 2: Add an end-to-end HTTP lifecycle test**

Test: registered user submits low comment → public sees body marked pending → reply/like rejected → user edits to medium → public sees no pending body (or last approved body when present) → user deletes → subsequent edit returns `CONTENT_ALREADY_DELETED`.

- [ ] **Step 3: Wire constructors and rule loading**

Build repository → content processor → classifier → policy → moderation service → business services → handlers. Inject logger, DB and config; do not introduce package globals.

- [ ] **Step 4: Refresh Swagger**

Run: `make swag`
Expected: generated docs include moderation DTOs, edit routes, `409`/`422` responses and no `model.*` schemas.

- [ ] **Step 5: Run focused verification**

```bash
go test ./pkg/config ./internal/service/moderation ./internal/repository/moderation -count=1
go test ./internal/service/comment ./internal/service/guestbook ./internal/service/moment -count=1
go test ./internal/handler/comment ./internal/handler/guestbook ./internal/handler/moment -count=1
```

Expected: all PASS.

- [ ] **Step 6: Run full quality gate**

```bash
gofmt -w pkg/config internal/model internal/dbschema internal/dto internal/repository internal/service internal/handler internal/router pkg/response
go vet ./...
go test ./... -count=1
git diff --check
```

Expected: no vet findings, all tests PASS, no whitespace errors.

- [ ] **Step 7: Verify invariants by search**

```bash
rg -n "CONTENT_RISK_REJECTED|CONTENT_ALREADY_DELETED|CONTENT_PENDING_NO_INTERACTION" internal pkg
rg -n "pending_revision_id|approved_revision_id|materialized_revision_id" internal
rg -n "fmt\.Print|var .*gorm\.DB|var .*redis" internal pkg
```

Expected: error codes and version pointers are wired; final search finds no newly introduced global infrastructure or production `fmt.Print*`.

- [ ] **Step 8: Commit**

```bash
git add .
git commit -m "feat(moderation): 完成审核核心流程"
```

## Core Exit Criteria

- Seven subject types share one moderation path for create/edit/delete.
- Low risk displays sanitized pending text; medium risk hides pending text; high risk creates no business content and returns an explicit risk error.
- Public reads never leak medium/high text, rules, score or match IDs.
- Author/admin reads receive review state and pending editor content where permitted.
- Pending content cannot be liked, replied to or used as a child-comment target.
- Delete/admin_delete are irreversible and cascade moderation tombstones.
- No unreviewed image is exposed before the Media & Review phase.
- Config validation, focused tests, full tests, vet, Swagger generation and diff checks pass.
- The follow-up Media & Review plan must implement image snapshots/rollback and approve/correct/reject before deployment.
