# Moderation Review and Media Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让审核队列只展示当前修订，并补齐可查看图片的 90 天审计历史与碎语图片正式化生命周期。

**Architecture:** repository 提供当前投影、历史读模型和有界图片引用更新；service 编排 Garage 复制、MySQL 状态变更和补偿；handler 只暴露管理员 DTO。首次自动通过图片使用“先创建审核事实、再同步正式化、失败补偿并保持不可公开”的 saga，人工通过复用同一正式化组件。

**Tech Stack:** Go 1.25、Gin、GORM/MySQL、Garage S3、gomock、go-sqlmock、httptest/testify。

## Global Constraints

- 不迁移现有审核内容或对象。
- 正式路径固定为 `moments/{userID}/{momentID}/{hash}.{ext}`。
- 非当前修订、操作日志和私有审计图片保留 90 天。
- handler 不写业务逻辑；service 不使用 GORM；repository 不访问对象存储。
- 所有生产改动必须先看到对应测试按预期失败。

---

### Task 1: 当前审核队列投影

**Files:**
- Modify: `internal/repository/moderation/review_query.go`
- Modify: `internal/repository/moderation/types.go`
- Test: `internal/repository/moderation/review_query_test.go`

**Interfaces:**
- Consumes: `ReviewFilter`。
- Produces: `ListReviewRecords(context.Context, ReviewFilter) (ReviewPage, error)`，每个 item 最多一条当前修订。

- [ ] **Step 1: 写失败测试**

新增 sqlmock 用例，构造同一 item 的 approved v1、superseded v2、pending v3，期望列表只扫描 `pending_revision_id=v3`；无 pending 时扫描最大 version。断言 `Total` 按 item 数而不是 revision 数计算。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/repository/moderation -run 'TestListReviewRecordsReturnsCurrentRevision' -count=1`

Expected: FAIL，现有查询直接 JOIN 全部 `moderation_revision`。

- [ ] **Step 3: 最小实现**

将 `reviewQuery` 的版本连接改为当前修订表达式：

```sql
JOIN moderation_revision
  ON moderation_revision.id = COALESCE(
    moderation_item.pending_revision_id,
    (SELECT latest.id FROM moderation_revision AS latest
     WHERE latest.item_id = moderation_item.id
     ORDER BY latest.version DESC LIMIT 1)
  )
```

筛选、计数和分页复用同一查询；排序继续使用当前修订时间。

- [ ] **Step 4: 验证 GREEN**

Run: `go test ./internal/repository/moderation -run 'TestListReviewRecords|TestLoadCurrentReviewRecord' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/repository/moderation/review_query.go internal/repository/moderation/types.go internal/repository/moderation/review_query_test.go
git commit -m "fix(moderation): 收口审核队列当前修订投影"
```

### Task 2: 审计历史 repository 与 service

**Files:**
- Modify: `internal/repository/moderation/repository.go`
- Modify: `internal/repository/moderation/types.go`
- Create: `internal/repository/moderation/history.go`
- Modify: `internal/service/moderation/review.go`
- Create: `internal/service/moderation/review_history.go`
- Test: `internal/repository/moderation/history_test.go`
- Test: `internal/service/moderation/review_history_test.go`

**Interfaces:**
- Produces repository method `LoadReviewHistory(ctx context.Context, itemID uint64, page, pageSize int) (ReviewHistoryPage, error)`。
- Produces service method `History(ctx context.Context, cmd ReviewHistoryCommand) (ReviewHistoryPage, error)`。

- [ ] **Step 1: 写 repository 失败测试**

覆盖修订倒序分页、对应图片顺序、操作事件、空 item 返回 `ErrItemNotFound`；图片和日志必须只按当前页 revision IDs/itemID 批量查询，禁止 N+1。

- [ ] **Step 2: 验证 repository RED**

Run: `go test ./internal/repository/moderation -run TestLoadReviewHistory -count=1`

Expected: FAIL，接口尚不存在。

- [ ] **Step 3: 定义读模型并实现**

```go
type ReviewHistoryCommand struct { ItemID uint64; Page int; PageSize int }
type ReviewHistoryEvent struct {
    ID uint64; RevisionID *uint64; ActorUserID *uint64
    Action Event; Reason *string; MetadataJSON *string; CreatedAt time.Time
}
type ReviewHistoryPage struct {
    Total int64; Page int; PageSize int
    Revisions []ReviewRecord
    Images map[uint64][]RevisionImageRecord
    Events []ReviewHistoryEvent
}
```

repository 校验 `page>=1`、`1<=pageSize<=100`，一次查询修订、一次查询图片、一次查询事件；service 只做分页归一化和 repository 错误映射。

- [ ] **Step 4: 写并通过 service 测试**

Run: `go test ./internal/service/moderation -run TestReviewHistory -count=1`

Expected: PASS，gomock 验证 item ID 与最大页大小。

- [ ] **Step 5: 提交**

```bash
git add internal/repository/moderation internal/service/moderation
git commit -m "feat(moderation): 新增审核审计历史读模型"
```

### Task 3: 管理端历史与图片 DTO/API

**Files:**
- Modify: `internal/dto/admin_moderation.go`
- Modify: `internal/handler/moderation/query.go`
- Modify: `internal/handler/moderation/response.go`
- Modify: `internal/router/router.go`
- Test: `internal/handler/moderation/moderation_test.go`
- Regenerate: `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`

**Interfaces:**
- Produces: `GET /admin/moderation/items/:id/history?page=&page_size=`。
- Produces DTO `AdminModerationHistoryResp`，图片字段含 `access_url` 与 `display_mode`。

- [ ] **Step 1: 写 handler 失败测试**

覆盖 admin 成功、无身份 401、`page_size=101` 绑定失败、图片和事件映射；成功响应不得出现 object key 以外的私有存储凭据。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/handler/moderation -run TestAdminModerationHistory -count=1`

Expected: FAIL，路由和 handler 尚不存在。

- [ ] **Step 3: 实现 DTO、handler 和路由**

handler 使用 `reqbind.PathUint` 与 `reqbind.Query`，调用 `ReviewService.History`，统一通过 `response.Success` 返回 DTO；Swagger 标注 admin、分页边界和 401/403/404/500。

- [ ] **Step 4: 验证并生成 Swagger**

Run: `go test ./internal/handler/moderation -count=1 && make swag`

Expected: PASS，Swagger 包含 `/admin/moderation/items/{id}/history`。

- [ ] **Step 5: 提交**

```bash
git add internal/dto internal/handler/moderation internal/router docs
git commit -m "feat(moderation): 增加管理端审计历史接口"
```

### Task 4: 图片正式化与补偿边界

**Files:**
- Create: `internal/service/moderationmedia/publish.go`
- Modify: `internal/service/moderationmedia/media.go`
- Modify: `internal/repository/moderation/repository.go`
- Create: `internal/repository/moderation/media_publish.go`
- Modify: `pkg/storage/resolver.go`
- Test: `internal/service/moderationmedia/publish_test.go`
- Test: `internal/repository/moderation/media_publish_test.go`

**Interfaces:**
- Produces `Publisher.Publish(ctx context.Context, cmd PublishCommand) (PublishResult, error)`。
- Produces repository method `ApplyPublishedImageKeys(ctx context.Context, cmd PublishedImageCommand) error`。

- [ ] **Step 1: 写失败测试**

覆盖首次复制、保留相同 hash、不再公开图片转审计、Copy 失败不改 DB、DB 失败删除新正式对象、提交成功删除 staging/旧公开对象、重复执行幂等。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/service/moderationmedia -run TestPublish -count=1`

Expected: FAIL，Publisher 尚不存在。

- [ ] **Step 3: 实现 service 编排**

```go
type PublishCommand struct {
    ItemID, RevisionID, UserID, MomentID uint64
    Current []moderationrepo.RevisionImageRecord
    Previous []moderationrepo.RevisionImageRecord
}
type PublishedImage struct { SourceKey, PublicKey string; Seq uint }
type PublishResult struct { Images []PublishedImage; AuditMoves map[string]string }
```

先计算纯计划，再 `CopyObject`；全部复制成功后调用 repository 更新引用和 `moment_media`。repository 失败删除本轮新对象；repository 成功后删除 staging 与已转存的旧公开对象。正式 key 使用 `path.Join("moments", userID, momentID, basename)`，审计 key 使用 `path.Join("moderation/history/moments", itemID, basename)`。

- [ ] **Step 4: 实现 repository 原子更新并验证**

repository 在单事务内锁定 item，校验 revision 仍是 approved/materialized，批量更新修订图片 key，重建当前 `moment_media`。Run: `go test ./internal/repository/moderation ./internal/service/moderationmedia -count=1`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/service/moderationmedia internal/repository/moderation pkg/storage/resolver.go
git commit -m "feat(moderation): 建立碎语图片正式化生命周期"
```

### Task 5: 三种通过路径统一正式化

**Files:**
- Modify: `internal/service/moderation/service.go`
- Modify: `internal/service/moderation/service_write.go`
- Modify: `internal/service/moderation/review.go`
- Modify: `internal/router/moderation.go`
- Test: `internal/service/moderation/service_test.go`
- Test: `internal/service/moderation/review_test.go`

**Interfaces:**
- Consumes: Task 4 `Publisher`。
- Produces: 自动通过、人工通过、修正通过均在返回成功前完成正式化。

- [ ] **Step 1: 写失败测试**

分别断言三条路径调用 Publisher；Publisher 失败时接口不返回成功，新的公开对象被补偿，item 不得留下可交互但图片未正式化的公开投影。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/service/moderation -run 'Test.*PublishImages' -count=1`

Expected: FAIL，service 尚未注入 Publisher。

- [ ] **Step 3: 最小实现**

为两个 service 构造函数注入窄接口 `ApprovedImagePublisher`。人工审核在审核事务前读取当前/旧图片并生成复制计划，事务提交后应用正式 key；首次自动通过在 `ApplyTransition` 返回 subject ID 后立即执行同一流程，正式化失败通过补偿 repository 将公开投影收回 placeholder，等待幂等重试而不是暴露 staging URL。

- [ ] **Step 4: 验证 GREEN**

Run: `go test ./internal/service/moderation ./internal/router -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/service/moderation internal/router/moderation.go
git commit -m "fix(moderation): 统一审核通过图片正式化流程"
```

### Task 6: 90 天审计与孤儿清理

**Files:**
- Modify: `config/config.yaml`
- Modify: `config/config.test.yaml`
- Modify: `pkg/config/moderation.go`
- Modify: `internal/repository/moderation/cleanup.go`
- Modify: `internal/worker/moderation/cleanup.go`
- Test: `internal/repository/moderation/cleanup_test.go`
- Test: `internal/worker/moderation/cleanup_test.go`

- [ ] **Step 1: 写失败测试**

固定 now，断言 90 天前且未被 materialized/approved/pending 引用的修订、日志、staging、history 对象可清理；当前 revision 及其正式对象始终保留。

- [ ] **Step 2: 验证 RED**

Run: `go test ./internal/repository/moderation ./internal/worker/moderation -run 'Test.*Cleanup' -count=1`

Expected: FAIL，当前配置为 365 天且未覆盖 history prefix。

- [ ] **Step 3: 实现并验证**

将审计 retention 默认值统一为 90，清理目标增加 `moderation/staging/moments/` 与 `moderation/history/moments/`；每次删除前通过 `ReferencedObjectKeys` 二次确认。Run: `go test ./internal/repository/moderation ./internal/worker/moderation ./pkg/config -count=1`，Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add config pkg/config internal/repository/moderation internal/worker/moderation
git commit -m "fix(moderation): 将审计数据保留期收口为九十天"
```

### Task 7: 后端回归验证

- [ ] **Step 1: 格式化与静态检查**

Run: `gofmt -w internal/repository/moderation/*.go internal/service/moderation/*.go internal/service/moderationmedia/*.go internal/handler/moderation/*.go internal/dto/admin_moderation.go internal/router/moderation.go internal/router/router.go pkg/config/*.go pkg/storage/resolver.go internal/worker/moderation/*.go && git diff --check`

Expected: 无输出、退出码 0。

- [ ] **Step 2: 全量测试**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 3: 构建**

Run: `go build ./...`

Expected: PASS。
