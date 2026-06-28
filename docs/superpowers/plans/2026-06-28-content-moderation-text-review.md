# Content Moderation Text Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为现有纯文本审核核心补齐管理员待审队列、通过、修正后通过、驳回、版本回退和站内通知闭环。

**Architecture:** 保持 `handler -> service -> repository` 分层。管理员提交待审版本 ID 与审核项锁版本，service 通过纯状态机生成计划，repository 在同一 MySQL 事务中更新审核版本、业务正文、审核指针、用户计数、操作日志和 `notification_event`。图片仍由后续 Media 计划处理，本计划只持久化碎语业务开关，确保中风险碎语审核后状态不丢失。

**Tech Stack:** Go 1.25+、Gin、GORM/MySQL、gomock、go-sqlmock、httptest/testify、swaggo。

## Global Constraints

- 仅管理员可调用审核管理接口；请求体中的用户 ID 不可信。
- 已删除内容不能通过、修正或驳回；待审版本或锁版本不一致返回 HTTP 409。
- 修正保留 `submitted_content`，只更新 `published_content`、理由、管理员 ID 和时间。
- 驳回首次发布后隐藏；驳回编辑后原子恢复最后通过正文。
- 审核通知只进入站内通知，事件类型使用 `system_notice`，不创建邮件任务。
- 举报、申诉、图片审核、治理和全站控制不进入本计划。
- 禁止直接返回 `model.*`，禁止全局基础设施，生产代码使用注入的 `zap.Logger`。
- 每项生产代码必须先有会按预期失败的测试。

---

### Task 1: Review Persistence Contracts and Moment Options

**Files:**
- Modify: `internal/model/moderation_content.go`
- Modify: `internal/repository/moderation/types.go`
- Modify: `internal/repository/moderation/repository.go`
- Modify: `internal/repository/moderation/query.go`
- Create: `internal/repository/moderation/review_query.go`
- Test: `internal/model/moderation_contract_test.go`
- Test: `internal/repository/moderation/review_query_test.go`
- Modify: `internal/service/moderation/service_mapping.go`

**Interfaces:**
- Produces `ListReviewRecords(ctx, ReviewFilter) (ReviewPage, error)` and `LoadReviewRecord(ctx, itemID, revisionID uint64) (ReviewRecord, error)`.
- `ReviewRecord` contains the exact pending revision, canonical `SubjectRef`, item state, author, submitted/published content, risk/action/status, and nullable `MomentOptions`.

- [ ] **Step 1: Write failing model and repository contract tests**

```go
func TestModerationRevisionPersistsMomentOptions(t *testing.T) {
    typ := reflect.TypeOf(model.ModerationRevision{})
    _, hasStatus := typ.FieldByName("MomentStatus")
    _, hasCommentStatus := typ.FieldByName("MomentCommentStatus")
    require.True(t, hasStatus)
    require.True(t, hasCommentStatus)
}

func TestListReviewRecordsReturnsPendingRevisionAndCanonicalSubject(t *testing.T) {
    repo, mock := newRepository(t)
    mock.ExpectQuery("SELECT .*moderation_item.*moderation_revision").
        WillReturnRows(sqlmock.NewRows([]string{
            "item_id", "content_type", "content_id", "author_id", "lock_version",
            "lifecycle_state", "public_state", "approved_revision_id", "pending_revision_id",
            "revision_id", "version", "submitted_content", "published_content", "risk_level",
            "policy_action", "review_status", "moment_status", "moment_comment_status", "created_at",
        }).AddRow(10, "moment", 7, 42, 3, "active", "placeholder", nil, 20,
            20, 1, "待审原文", "待审正文", "medium", "pre_review", "pending", 1, 0, fixedTime))

    page, err := repo.ListReviewRecords(context.Background(), moderation.ReviewFilter{
        Page: 1, PageSize: 20, ReviewStatus: moderation.ReviewPending,
    })

    require.NoError(t, err)
    require.Len(t, page.Items, 1)
    assert.Equal(t, uint64(20), page.Items[0].RevisionID)
    assert.Equal(t, &moderation.MomentOptions{Status: 1, CommentStatus: 0}, page.Items[0].MomentOptions)
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/model ./internal/repository/moderation -run 'MomentOptions|ReviewRecords' -count=1`

Expected: FAIL because the new revision fields and review query methods do not exist.

- [ ] **Step 3: Add persistence and repository contracts**

```go
type ModerationRevision struct {
    // existing fields...
    MomentStatus        *uint8 `gorm:"type:tinyint;comment:碎语提交时公开开关"`
    MomentCommentStatus *uint8 `gorm:"type:tinyint;comment:碎语提交时评论开关"`
}

type ReviewFilter struct {
    Page         int
    PageSize     int
    ContentType  *SubjectType
    RiskLevel    *RiskLevel
    ReviewStatus ReviewStatus
}

type ReviewRecord struct {
    ItemID          uint64
    Subject         SubjectRef
    AuthorID        uint64
    LockVersion     uint64
    State           ItemState
    RevisionID      uint64
    RevisionVersion uint64
    SubmittedContent string
    PublishedContent string
    RiskLevel       RiskLevel
    PolicyAction    PolicyAction
    ReviewStatus    ReviewStatus
    MomentOptions   *MomentOptions
    CreatedAt       time.Time
}

type ReviewPage struct {
    Total int64
    Items []ReviewRecord
}
```

Extend `RevisionDraft` with `MomentOptions *MomentOptions`, copy submit/edit options into it, and write nullable option columns from `createRevision`. Implement deterministic review queries ordered by `moderation_revision.created_at DESC, moderation_revision.id DESC`; `LoadReviewRecord` must verify the revision belongs to the requested item and return `ErrItemNotFound` otherwise.

- [ ] **Step 4: Run focused and schema tests**

Run: `go test ./internal/model ./internal/dbschema ./internal/repository/moderation ./internal/service/moderation -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/moderation_content.go internal/model/moderation_contract_test.go internal/repository/moderation internal/service/moderation/service_mapping.go
git commit -m "feat(moderation): 持久化审核队列与碎语选项"
```

### Task 2: Atomic Review Transition, Correction, Rollback, and Notification Intent

**Files:**
- Modify: `internal/repository/moderation/types.go`
- Modify: `internal/repository/moderation/transition.go`
- Create: `internal/repository/moderation/review_transition_test.go`
- Modify: `internal/repository/moderation/mock/mock_repository.go`

**Interfaces:**
- Consumes `ReviewRecord` and persisted `MomentOptions` from Task 1.
- Extends `RevisionReview` with nullable corrected content.
- Extends `ApplyTransitionCommand` with one transaction-local `NotificationIntent`.

- [ ] **Step 1: Write failing transaction tests**

```go
func TestCorrectAndApproveUpdatesPublishedContentBeforeMaterialize(t *testing.T) {
    command := reviewCommand(EventCorrectAndApprove)
    corrected := "管理员修正正文"
    command.Review.PublishedContent = &corrected
    expectReviewTransaction(mock, "管理员修正正文", false)

    _, err := repository.ApplyTransition(context.Background(), command)

    require.NoError(t, err)
    require.NoError(t, mock.ExpectationsWereMet())
}

func TestRejectPostReviewEditRestoresApprovedRevisionInSameTransaction(t *testing.T) {
    command := reviewCommand(EventReject)
    command.Next.Materialized = ExistingRevision(11)
    command.Materialize = ExistingRevision(11)
    expectReviewTransaction(mock, "最后通过正文", false)

    _, err := repository.ApplyTransition(context.Background(), command)

    require.NoError(t, err)
}

func TestReviewCreatesInAppNotificationEventAtomically(t *testing.T) {
    command := reviewCommand(EventApprove)
    command.Notification = &NotificationIntent{
        RecipientUserID: 42, Type: "system_notice", SourceType: "system", RootType: "system",
        Title: "内容审核通过", ContentExcerpt: "你的内容已通过审核",
    }
    expectReviewTransaction(mock, "待审正文", true)

    _, err := repository.ApplyTransition(context.Background(), command)

    require.NoError(t, err)
}

func reviewCommand(event Event) ApplyTransitionCommand {
    reason := "审核原因"
    reviewerID := uint64(1)
    status := ReviewApproved
    decision := "approved"
    if event == EventCorrectAndApprove {
        decision = "corrected"
    }
    if event == EventReject {
        status = ReviewRejected
        decision = "rejected"
    }
    return ApplyTransitionCommand{
        Subject: SubjectRef{Type: SubjectArticleComment, ID: 7, RootID: 3},
        AuthorID: 42, ExpectedLockVersion: 4, ExpectedPendingID: uint64Pointer(20),
        Next: ItemState{
            LifecycleState: LifecycleActive, PublicState: PublicVisible,
            Materialized: ExistingRevision(20), Approved: ExistingRevision(20),
        },
        Review: &RevisionReview{
            RevisionID: 20, Status: status, Decision: decision,
            Reason: &reason, ReviewerID: &reviewerID, ReviewedAt: fixedTime,
        },
        Materialize: ExistingRevision(20),
        Log: &ActionLog{
            Revision: ExistingRevision(20), ActorUserID: &reviewerID,
            SubjectUserID: uint64Pointer(42), Action: event, Reason: &reason, CreatedAt: fixedTime,
        },
    }
}

func expectReviewTransaction(mock sqlmock.Sqlmock, materializedContent string, withNotification bool) {
    mock.ExpectBegin()
    mock.ExpectQuery("SELECT .* FROM `moderation_item`.*FOR UPDATE").WillReturnRows(activeItemRows(4, uint64Pointer(20)))
    expectLockedArticleSubject(mock, "当前业务正文")
    mock.ExpectQuery("SELECT .* FROM `moderation_revision`.*FOR UPDATE").
        WillReturnRows(sqlmock.NewRows([]string{"id", "item_id"}).AddRow(20, 10))
    mock.ExpectExec("UPDATE `moderation_revision` SET").WillReturnResult(sqlmock.NewResult(0, 1))
    mock.ExpectExec("UPDATE `moderation_item` SET").WillReturnResult(sqlmock.NewResult(0, 1))
    mock.ExpectQuery("SELECT .* FROM `moderation_revision`").
        WillReturnRows(sqlmock.NewRows([]string{"published_content"}).AddRow(materializedContent))
    mock.ExpectExec("UPDATE `article_comment` SET").WillReturnResult(sqlmock.NewResult(0, 1))
    mock.ExpectExec("INSERT INTO `moderation_action_log`").WillReturnResult(sqlmock.NewResult(1, 1))
    if withNotification {
        mock.ExpectExec("INSERT INTO `notification_event`").WillReturnResult(sqlmock.NewResult(99, 1))
    }
    mock.ExpectCommit()
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/repository/moderation -run 'CorrectAndApprove|RejectPostReview|ReviewCreates' -count=1`

Expected: FAIL because corrected content and notification intent are not transaction inputs.

- [ ] **Step 3: Implement the minimal transactional mutation**

```go
type RevisionReview struct {
    RevisionID       uint64
    Status           ReviewStatus
    Decision         string
    Reason           *string
    ReviewerID       *uint64
    ReviewedAt       time.Time
    PublishedContent *string
}

type NotificationIntent struct {
    RecipientUserID uint64
    Type            string
    SourceType      string
    SourceID        uint
    RootType        string
    RootID          uint
    Title           string
    ContentExcerpt  string
    MetadataJSON    string
}
```

`reviewRevision` updates `published_content` only when `PublishedContent != nil`. `materialize` reads the already-updated revision and applies its persisted `MomentOptions`. `ApplyTransition` inserts `notification_event` after item/materialization/log/profile writes but before commit; metadata must contain `recipient_user_ids:[authorID]`, dispatch status `pending`, and no email-specific side effect.

- [ ] **Step 4: Verify conflict and deletion paths**

Add tests proving stale revision, stale lock, superseded revision, and `lifecycle_state=deleted` return conflict errors without `UPDATE moment/comment/guestbook` or notification inserts.

Run: `go test ./internal/repository/moderation -count=1`

Expected: PASS.

- [ ] **Step 5: Regenerate repository mock and commit**

```bash
go run go.uber.org/mock/mockgen -destination=internal/repository/moderation/mock/mock_repository.go -package=mock github.com/vpt/blog-backend/internal/repository/moderation Repository
git add internal/repository/moderation
git commit -m "feat(moderation): 原子执行人工审核决策"
```

### Task 3: Review Application Service

**Files:**
- Create: `internal/service/moderation/review.go`
- Create: `internal/service/moderation/review_mapping.go`
- Create: `internal/service/moderation/review_test.go`
- Modify: `internal/service/moderation/errors.go`
- Modify: `pkg/config/moderation.go`
- Modify: `pkg/config/moderation_validate.go`
- Modify: `pkg/config/config_test.go`
- Modify: `config/config.yaml`
- Modify: `config/config.local.yaml.example`

**Interfaces:**
- Produces `ReviewService.List`, `ReviewService.Get`, `ReviewService.Approve`, `ReviewService.Correct`, and `ReviewService.Reject`.
- Review actions consume item ID, exact pending revision ID, expected lock version, reviewer ID, and reason/corrected content.

- [ ] **Step 1: Write failing service tests**

```go
func TestReviewServiceApproveBuildsApprovedTransition(t *testing.T) {
    record := moderationrepo.ReviewRecord{
        ItemID: 10, Subject: moderationrepo.SubjectRef{Type: moderationrepo.SubjectArticleComment, ID: 7, RootID: 3},
        AuthorID: 42, LockVersion: 3, RevisionID: 20, RevisionVersion: 1,
        SubmittedContent: "原文", PublishedContent: "安全正文",
        RiskLevel: moderationrepo.RiskLow, PolicyAction: moderationrepo.ActionPostReview,
        ReviewStatus: moderationrepo.ReviewPending,
        State: moderationrepo.ItemState{
            LifecycleState: moderationrepo.LifecycleActive, PublicState: moderationrepo.PublicVisible,
            Materialized: moderationrepo.ExistingRevision(20), Pending: moderationrepo.ExistingRevision(20),
        },
    }
    repo.EXPECT().LoadReviewRecord(gomock.Any(), record.ItemID, record.RevisionID).Return(record, nil)
    repo.EXPECT().ApplyTransition(gomock.Any(), gomock.Any()).DoAndReturn(
        func(_ context.Context, cmd moderationrepo.ApplyTransitionCommand) (moderationrepo.AppliedTransition, error) {
            assert.Equal(t, moderationrepo.ReviewApproved, cmd.Review.Status)
            assert.Equal(t, moderationrepo.ExistingRevision(record.RevisionID), cmd.Materialize)
            assert.Equal(t, int64(1), cmd.ProfileChange.CleanApprovalDelta)
            assert.Equal(t, record.AuthorID, cmd.Notification.RecipientUserID)
            return moderationrepo.AppliedTransition{ItemID: record.ItemID}, nil
        })

    got, err := service.Approve(context.Background(), ReviewCommand{
        ItemID: record.ItemID, RevisionID: record.RevisionID,
        ExpectedLockVersion: record.LockVersion, ReviewerID: 1,
    })

    require.NoError(t, err)
    assert.Equal(t, ReviewApproved, got.ReviewStatus)
}

func TestReviewServiceCorrectSanitizesAndPreservesSubmittedContent(t *testing.T) {
    got, err := service.Correct(ctx, CorrectCommand{
        ReviewCommand: ReviewCommand{ItemID: 10, RevisionID: 20, ExpectedLockVersion: 3, ReviewerID: 1, Reason: "移除不当表述"},
        Content: `<p>修正</p><script>alert(1)</script>`,
    })
    require.NoError(t, err)
    assert.Equal(t, "<p>修正</p>", got.PublishedContent)
}

func TestReviewServiceRejectRequiresPublicReason(t *testing.T) {
    _, err := service.Reject(ctx, ReviewCommand{ItemID: 10, RevisionID: 20, ReviewerID: 1})
    require.ErrorIs(t, err, ErrInvalidRequest)
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/service/moderation -run 'ReviewService' -count=1`

Expected: FAIL because `ReviewService` and review commands do not exist.

- [ ] **Step 3: Implement review service and configuration**

```go
type ReviewService interface {
    List(ctx context.Context, cmd ListReviewCommand) (ReviewPage, error)
    Get(ctx context.Context, itemID uint64) (ReviewItem, error)
    Approve(ctx context.Context, cmd ReviewCommand) (ReviewItem, error)
    Correct(ctx context.Context, cmd CorrectCommand) (ReviewItem, error)
    Reject(ctx context.Context, cmd ReviewCommand) (ReviewItem, error)
}

type ReviewCommand struct {
    ItemID             uint64
    RevisionID         uint64
    ExpectedLockVersion uint64
    ReviewerID         uint64
    Reason             string
}

type CorrectCommand struct {
    ReviewCommand
    Content string
}
```

Add strong config values:

```yaml
moderation:
  review:
    queue_default_page_size: 20
    queue_max_page_size: 100
    reason_max_chars: 1000
```

Approve reason is optional. Correct and reject reason is required after trimming. Correct content uses the existing `ContentProcessor` and content-type length limit, but does not re-enter ordinary-user risk classification. All three methods verify reviewer/item/revision/lock are non-zero, call `Transition`, then build one repository command with reviewer ID/time, profile delta, action log and author notification intent.

- [ ] **Step 4: Verify all review service branches**

Cover initial post-review reject (`hidden`, no materialization), initial pre-review approve, approved-content medium-edit reject rollback, correct-and-approve, stale lock/revision, deleted item, empty sanitized correction, and reason length overflow.

Run: `go test ./internal/service/moderation ./pkg/config -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/moderation pkg/config config
git commit -m "feat(moderation): 新增人工审核应用服务"
```

### Task 4: Admin Review HTTP API

**Files:**
- Create: `internal/dto/admin_moderation.go`
- Create: `internal/handler/moderation/moderation.go`
- Create: `internal/handler/moderation/query.go`
- Create: `internal/handler/moderation/review.go`
- Create: `internal/handler/moderation/response.go`
- Create: `internal/handler/moderation/moderation_test.go`
- Modify: `internal/router/router.go`
- Modify: `internal/router/moderation.go`
- Modify: `internal/router/router_test.go`
- Modify: `pkg/response/response.go`
- Regenerate: `docs/docs.go`
- Regenerate: `docs/swagger.json`
- Regenerate: `docs/swagger.yaml`

**API Risk Pass:**
- Caller: admin route group only; reviewer ID comes from JWT claims.
- Bounds: page/page_size normalized by config; enum filters validated; reason and corrected content bounded by service config.
- Cost: one bounded list query or one MySQL transaction; reuse admin normal rate limit on writes.
- Failure closure: all review writes and notification intent are one transaction; no object storage in this phase.
- Cleanup: rejection restores approved business content or clears first-publication content; deleted items remain terminal.

**Interfaces:**
- `GET /admin/moderation/items`
- `GET /admin/moderation/items/:id`
- `POST /admin/moderation/items/:id/approve`
- `POST /admin/moderation/items/:id/correct`
- `POST /admin/moderation/items/:id/reject`

- [ ] **Step 1: Write failing handler and route tests**

```go
func TestAdminApproveUsesJWTReviewerAndReturnsUpdatedReview(t *testing.T) {
    service.EXPECT().Approve(gomock.Any(), moderationservice.ReviewCommand{
        ItemID: 10, RevisionID: 20, ExpectedLockVersion: 3, ReviewerID: 1,
    }).Return(moderationservice.ReviewItem{
        ItemID: 10, RevisionID: 20, ReviewStatus: moderationservice.ReviewApproved,
    }, nil)
    request := adminJSONRequest(http.MethodPost, "/admin/moderation/items/10/approve",
        `{"revision_id":20,"lock_version":3}`)

    recorder := serveAdminModeration(request, service)

    assert.Equal(t, http.StatusOK, recorder.Code)
    assert.Contains(t, recorder.Body.String(), `"review_status":"approved"`)
}

func TestAdminReviewConflictUsesStable409Code(t *testing.T) {
    service.EXPECT().Reject(gomock.Any(), gomock.Any()).Return(moderationservice.ReviewItem{}, moderationservice.ErrReviewConflict)
    recorder := serveAdminModeration(adminJSONRequest(http.MethodPost,
        "/admin/moderation/items/10/reject", `{"revision_id":20,"lock_version":2,"reason":"不通过"}`), service)
    assert.Equal(t, http.StatusConflict, recorder.Code)
    assert.Contains(t, recorder.Body.String(), "MODERATION_REVIEW_CONFLICT")
}

func adminJSONRequest(method, path, body string) *http.Request {
    request := httptest.NewRequest(method, path, strings.NewReader(body))
    request.Header.Set("Content-Type", "application/json")
    return request
}

func serveAdminModeration(request *http.Request, service moderationservice.ReviewService) *httptest.ResponseRecorder {
    gin.SetMode(gin.TestMode)
    recorder := httptest.NewRecorder()
    engine := gin.New()
    handler := moderationhandler.NewAdminHandler(service)
    admin := engine.Group("/admin/moderation", func(c *gin.Context) {
        jwt.SetClaims(c, &jwt.Claims{UserId: 1})
        c.Next()
    })
    admin.POST("/items/:id/approve", handler.Approve)
    admin.POST("/items/:id/reject", handler.Reject)
    engine.ServeHTTP(recorder, request)
    return recorder
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/handler/moderation ./internal/router -run 'Moderation|Review' -count=1`

Expected: FAIL because the handler package and routes do not exist.

- [ ] **Step 3: Add DTO, handlers, response mapping and routes**

```go
type AdminModerationReviewReq struct {
    RevisionID uint64 `json:"revision_id" binding:"required,min=1"`
    LockVersion uint64 `json:"lock_version" binding:"required,min=1"`
    Reason string `json:"reason"`
}

type AdminModerationCorrectReq struct {
    RevisionID uint64 `json:"revision_id" binding:"required,min=1"`
    LockVersion uint64 `json:"lock_version" binding:"required,min=1"`
    Content string `json:"content" binding:"required"`
    Reason string `json:"reason" binding:"required"`
}
```

Add `CodeModerationReviewConflict = "MODERATION_REVIEW_CONFLICT"`. Bind IDs and bodies in handlers, obtain reviewer from `jwt.GetClaims(c)`, call only `ReviewService`, and convert internal values to DTO. Register all endpoints in the existing admin group; apply `middleware.RateLimitNormal(redisClient)` to approve/correct/reject.

- [ ] **Step 4: Add Swagger and verify generated contracts**

Run: `make swag`

Expected: generated docs contain all five `/admin/moderation/items` paths and only `dto.*` schemas.

Run: `go test ./internal/handler/moderation ./internal/router -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/dto/admin_moderation.go internal/handler/moderation internal/router pkg/response/response.go docs
git commit -m "feat(moderation): 新增审核管理接口"
```

### Task 5: Public Placeholder Read Path and End-to-End Lifecycle

**Files:**
- Modify: `internal/repository/moment/query.go`
- Modify: `internal/repository/moment/feed.go`
- Modify: `internal/repository/moment/moment_test.go`
- Create: `internal/handler/moderation/lifecycle_test.go`
- Modify: `internal/service/notification/notification_test.go`

**Interfaces:**
- Public moment reads may include a moderation `placeholder` row even though its business `moment.status=0`.
- User-hidden moments (`status=0`, moderation state other than `placeholder`) remain excluded.

- [ ] **Step 1: Write failing public-read tests**

```go
func TestMomentListIncludesModerationPlaceholderButNotUserHiddenMoment(t *testing.T) {
    mock.ExpectQuery("SELECT .* FROM `moment`.*LEFT JOIN moderation_item").
        WillReturnRows(momentRows(
            momentRow{ID: 7, Status: 0, ModerationPublicState: "placeholder"},
        ))

    page, err := repo.List(momentrepo.ListFilter{Page: 1, PageSize: 10}, nil)

    require.NoError(t, err)
    require.Len(t, page.Moments, 1)
}
```

- [ ] **Step 2: Run test and verify RED**

Run: `go test ./internal/repository/moment -run 'ModerationPlaceholder' -count=1`

Expected: FAIL because public moment queries currently require `moment.status=1`.

- [ ] **Step 3: Implement placeholder-aware query predicate**

Use a left join scoped to moments:

```sql
LEFT JOIN moderation_item mi
  ON mi.content_type = 'moment' AND mi.content_id = moment.id
WHERE moment.status = 1
   OR (moment.status = 0 AND mi.lifecycle_state = 'active' AND mi.public_state = 'placeholder')
```

Apply the same predicate to public list, feed and detail. Do not include `hidden`, `emergency_hidden` or `deleted` moderation states. The moderation projection remains the only source of placeholder text and never returns pending medium-risk body publicly.

- [ ] **Step 4: Add HTTP lifecycle test**

The lifecycle test must submit a medium-risk new item, list it as placeholder, approve it, verify visible approved content and interaction, submit a medium-risk edit, reject it, verify the last approved content is restored, and verify one `system_notice` event is created per review decision.

Run: `go test ./internal/repository/moment ./internal/handler/moderation ./internal/service/notification -count=1`

Expected: PASS.

- [ ] **Step 5: Run final verification**

```bash
go vet ./...
go test ./... -count=1
go test -race ./internal/repository/moderation ./internal/service/moderation ./internal/handler/moderation ./internal/repository/moment -count=1
go build ./...
git diff --check
```

Expected: every command exits 0; generated Swagger contains no `model.*` definitions.

- [ ] **Step 6: Commit**

```bash
git add internal/repository/moment internal/handler/moderation/lifecycle_test.go internal/service/notification/notification_test.go
git commit -m "fix(moderation): 完成待审内容公开读取闭环"
```

## Self-Review

- Spec coverage: approve, correct, reject, rollback, reviewer/reason/time, notification, stale-review conflict, deleted terminal state, medium moment placeholder and persisted moment switches are covered.
- Deferred by scope: image snapshots/previews, preview cleanup, approved-image reuse, reports/appeals, governance and operations.
- Placeholder scan: no unresolved placeholders or unspecified implementation steps remain.
- Type consistency: repository `ReviewRecord` feeds service `ReviewItem`; HTTP DTOs never expose repository/model values directly; every action carries item ID, revision ID and lock version.
