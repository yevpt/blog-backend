# Notification System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a durable user notification system with in-app notifications, recoverable email digest queues, quota controls, and an SSE online channel.

**Architecture:** Use MySQL as the durable source of truth for notification events, inbox rows, email tasks, email batches, quota usage, and send logs. Redis is only an auxiliary tool for short-lived locks, rate helpers, and future multi-instance SSE Pub/Sub; queue recovery must work from MySQL alone.

**Tech Stack:** Go 1.25+, Gin, GORM/MySQL, Redis, zap, gomail.v2, Swagger, gomock/go-sqlmock/httptest/testify.

---

## Reference Documents

- Design spec: `docs/superpowers/specs/2026-06-23-notification-system-design.md`
- Project rules: `AGENTS.md`
- Layering skill: `.agents/skills/go-layering/SKILL.md`
- HTTP API skill: `.agents/skills/http-api/SKILL.md`
- Testing skill: `.agents/skills/go-testing/SKILL.md`

## File Structure

- Create `internal/model/notification.go`: notification event, inbox, preference, email task, email batch, batch item, quota policy, role quota policy, quota usage, send log models.
- Create `internal/dto/notification.go`: public notification list, count, read, preference, stream DTOs.
- Create `internal/dto/admin_notification.go`: admin email task, batch, quota query/update DTOs.
- Create `internal/repository/notification/notification.go`: repository interfaces and shared types.
- Create `internal/repository/notification/event.go`: event creation and worker lease queries.
- Create `internal/repository/notification/inbox.go`: inbox query, read, delete, unread count.
- Create `internal/repository/notification/email_task.go`: email task creation, lease, defer, batch assignment.
- Create `internal/repository/notification/email_batch.go`: batch creation, lease, send result updates.
- Create `internal/repository/notification/quota.go`: quota policy, role quota, usage reservation.
- Create `internal/service/notification/notification.go`: service interfaces, errors, constructors.
- Create `internal/service/notification/publisher.go`: business-facing event publisher.
- Create `internal/service/notification/inbox.go`: list/count/read/delete/preference use cases.
- Create `internal/service/notification/dispatcher.go`: pending event dispatcher.
- Create `internal/service/notification/email_planner.go`: task aggregation into batches.
- Create `internal/service/notification/email_sender.go`: quota checked email sending.
- Create `internal/service/notification/quota.go`: quota evaluation and reservation service.
- Create `internal/service/notification/sse.go`: SSE hub interface and in-memory implementation.
- Create `internal/handler/notification/notification.go`: public notification handlers.
- Create `internal/handler/notification/admin.go`: admin task/batch/quota handlers.
- Modify `pkg/email/email.go`: extend sender to support generic HTML notification email while keeping verification code behavior.
- Modify `pkg/config/config.go`: add email worker and safety limit config fields.
- Modify `config/config.yaml` and `config/config.local.yaml.example`: document safe defaults.
- Modify `internal/router/router.go`: wire notification repo/service/handlers and routes.
- Modify `cmd/migrate/main.go`: include new models in `AutoMigrate`.
- Modify `internal/service/comment`, `internal/service/article`, `internal/service/moment`, `internal/service/guestbook` as needed: publish notification events from service layer.
- Create focused tests under matching packages.
- Run `make swag` after HTTP handlers are implemented.

## Execution Notes

- Do not use old `message` / `user_message` for new writes.
- Do not add `thread_key`; use `source_type/source_id` and `root_type/root_id`.
- Do not send notification emails synchronously from user requests.
- Do not rely on Redis for queue durability.
- Keep every worker operation idempotent and lease based.
- Treat `cmd/migrate` as the old-database rebuild tool. Production schema changes need versioned migrations, not app-start AutoMigrate.
- Commit after each completed task group with a Chinese Conventional Commit message.

## Task 1: Models And Config

**Files:**

- Create: `internal/model/notification.go`
- Modify: `pkg/config/config.go`
- Modify: `config/config.yaml`
- Modify: `config/config.local.yaml.example`
- Modify: `cmd/migrate/main.go`
- Test: `pkg/config/config_test.go`

- [x] **Step 1: Add config tests**

Cover these fields in `pkg/config/config_test.go`:

```go
Email.Provider
Email.ProviderDailyHardLimit
Email.SiteDailySafeLimit
Email.MaxPerMinute
Email.MaxPerHour
Email.SendIntervalSeconds
Email.WorkerEnabled
Email.PlannerEnabled
Email.WorkerBatchSize
Email.LeaseSeconds
```

Run: `go test ./pkg/config -count=1`

Expected: FAIL until config fields are added.

- [x] **Step 2: Add notification models**

Add models matching the design spec:

```text
NotificationEvent
NotificationInbox
NotificationPreference
NotificationEmailTask
NotificationEmailBatch
NotificationEmailBatchItem
EmailQuotaPolicy
EmailRoleQuotaPolicy
EmailQuotaUsage
EmailSendLog
```

Use explicit `TableName()` methods. Keep JSON tags present but remember models must not be returned directly in handlers or Swagger.

- [x] **Step 3: Add config fields and defaults**

Extend `EmailConfig` with worker and safety fields. Add env binding keys for every new field.

Suggested defaults in `config/config.yaml`:

```yaml
email:
  provider: aliyun_enterprise
  provider_daily_hard_limit: 2000
  site_daily_safe_limit: 300
  max_per_minute: 5
  max_per_hour: 80
  send_interval_seconds: 12
  worker_enabled: true
  planner_enabled: true
  worker_batch_size: 20
  lease_seconds: 300
```

- [x] **Step 4: AutoMigrate models**

Register the new models in `cmd/migrate/main.go` `autoMigrate`.

- [x] **Step 5: Verify**

Run:

```bash
go test ./pkg/config -count=1
go test ./cmd/migrate -count=1
go test ./internal/model/... -count=1
```

Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add internal/model/notification.go pkg/config/config.go config/config.yaml config/config.local.yaml.example cmd/migrate/main.go pkg/config/config_test.go
git commit -m "feat(notification): 新增通知模型和邮件配置"
```

## Task 2: Notification Repository

**Files:**

- Create: `internal/repository/notification/notification.go`
- Create: `internal/repository/notification/event.go`
- Create: `internal/repository/notification/inbox.go`
- Create: `internal/repository/notification/email_task.go`
- Create: `internal/repository/notification/email_batch.go`
- Create: `internal/repository/notification/quota.go`
- Test: `internal/repository/notification/*_test.go`

- [ ] **Step 1: Define repository interfaces**

Define methods for:

```text
CreateEvent
LeasePendingEvents
MarkEventDone
MarkEventRetry
CreateInbox
ListInbox
CountUnread
MarkInboxRead
MarkAllInboxRead
DeleteInbox
CreateEmailTask
LeaseEmailTasks
CreateEmailBatchWithItems
LeaseEmailBatches
MarkBatchSent
MarkBatchRetry
ReserveQuota
GetQuotaPolicies
GetRoleQuotaPolicies
```

- [ ] **Step 2: Write repository tests first**

Cover:

- duplicate inbox insert does not create duplicate rows.
- duplicate email task idempotency key is ignored safely.
- lease query skips rows with unexpired lease.
- expired lease rows are claimable.
- quota reservation increments atomically and rejects over-limit.

Run: `go test ./internal/repository/notification -count=1`

Expected: FAIL until repository implementation exists.

- [ ] **Step 3: Implement repository**

Use GORM transactions for multi-row state changes.

For worker leasing, prefer MySQL 8 `FOR UPDATE SKIP LOCKED` through GORM clauses. If the project must support older MySQL, replace with conditional update:

```sql
UPDATE ... SET lease_until=?, locked_by=?
WHERE id=? AND (lease_until IS NULL OR lease_until < NOW())
```

- [ ] **Step 4: Verify**

Run: `go test ./internal/repository/notification -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/notification
git commit -m "feat(notification): 新增通知仓储"
```

## Task 3: Publisher And Inbox Service

**Files:**

- Create: `internal/service/notification/notification.go`
- Create: `internal/service/notification/publisher.go`
- Create: `internal/service/notification/inbox.go`
- Create: `internal/dto/notification.go`
- Test: `internal/service/notification/*_test.go`

- [ ] **Step 1: Define service contracts**

Create:

```go
type Publisher interface {
    Publish(ctx context.Context, event PublishEvent) (*model.NotificationEvent, error)
}

type InboxService interface {
    List(userID uint, req dto.NotificationListReq) (*dto.NotificationPageResp, error)
    UnreadCount(userID uint) (*dto.NotificationUnreadCountResp, error)
    MarkRead(userID uint, id uint) error
    MarkAllRead(userID uint, ids []uint) (*dto.NotificationReadResp, error)
    Delete(userID uint, id uint) error
}
```

- [ ] **Step 2: Write service tests first**

Cover:

- publisher trims and snapshots content excerpt.
- invalid event type is rejected.
- inbox list maps model aggregates to DTO.
- mark read rejects notifications not owned by user.
- mark all read supports all unread when `ids` is empty and request explicitly asks for all.

Run: `go test ./internal/service/notification -count=1`

Expected: FAIL until service implementation exists.

- [ ] **Step 3: Implement publisher and inbox service**

Publisher creates only `notification_event`. It must not create inbox rows directly. Dispatcher owns delivery.

Inbox service returns DTO only and never returns model types.

- [ ] **Step 4: Verify**

Run: `go test ./internal/service/notification -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/notification internal/dto/notification.go
git commit -m "feat(notification): 新增通知发布和收件箱服务"
```

## Task 4: Public Notification APIs

**Files:**

- Create: `internal/handler/notification/notification.go`
- Modify: `internal/router/router.go`
- Test: `internal/handler/notification/*_test.go`

- [ ] **Step 1: API Risk Pass**

Record answers in the handler test or task notes:

- Who can call: logged-in users only.
- Input bounds: `page_size` max 50; batch read ids max 100.
- Resource cost: read/list are DB-only; SSE is long-lived connection.
- Failure convergence: DB errors return server error; no partial external side effects.
- Old state cleanup: delete is soft delete of inbox row only.

- [ ] **Step 2: Write handler tests first**

Cover:

- list binds pagination.
- unread count requires auth.
- mark read rejects invalid id.
- mark all read rejects id list above limit.
- delete only calls service with current user id.

Run: `go test ./internal/handler/notification -count=1`

Expected: FAIL until handler exists.

- [ ] **Step 3: Implement handlers and routes**

Routes:

```go
authed.GET("/notifications", handlers.notification.List)
authed.GET("/notifications/unread-count", handlers.notification.UnreadCount)
authed.PATCH("/notifications/:id/read", handlers.notification.MarkRead)
authed.PATCH("/notifications/read", handlers.notification.MarkAllRead)
authed.DELETE("/notifications/:id", handlers.notification.Delete)
```

Use `jwt.GetClaims(c)` through existing project pattern and `pkg/response`.

- [ ] **Step 4: Verify**

Run:

```bash
go test ./internal/handler/notification -count=1
go test ./internal/router -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/notification internal/router/router.go
git commit -m "feat(notification): 新增站内通知接口"
```

## Task 5: Event Dispatcher

**Files:**

- Create: `internal/service/notification/dispatcher.go`
- Create: `internal/service/notification/preference.go`
- Test: `internal/service/notification/dispatcher_test.go`

- [ ] **Step 1: Write dispatcher tests first**

Cover:

- comment event creates one inbox row for content owner.
- self action does not notify self.
- disabled in-app preference skips inbox.
- enabled email preference creates email task.
- disabled `user_setting.receive_mail` skips email task.
- duplicate execution does not duplicate inbox or email task.
- event is retried with `next_process_at` on repository error.

Run: `go test ./internal/service/notification -run Dispatcher -count=1`

Expected: FAIL until dispatcher exists.

- [ ] **Step 2: Implement recipient resolver**

Resolver maps event type and root/source data to recipients:

```text
article comment -> article owner
moment comment -> moment owner
guestbook message -> guestbook owner
reply -> replied user
like -> owner of liked object
system_notice -> explicit recipients from metadata
```

Use repository helpers instead of querying GORM in service.

- [ ] **Step 3: Implement dispatcher loop method**

Expose a method that can be called by worker bootstrap:

```go
DispatchOnce(ctx context.Context, workerID string, limit int) (int, error)
```

This method leases events, processes each event idempotently, and returns processed count.

- [ ] **Step 4: Verify**

Run: `go test ./internal/service/notification -run Dispatcher -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/notification/dispatcher.go internal/service/notification/preference.go internal/service/notification/dispatcher_test.go
git commit -m "feat(notification): 新增通知事件分发器"
```

## Task 6: Integrate Business Events

**Files:**

- Modify: `internal/service/comment`
- Modify: `internal/service/article`
- Modify: `internal/service/moment`
- Modify: `internal/service/guestbook`
- Modify: `internal/router/router.go`
- Tests: related service tests

- [ ] **Step 1: Remove new notification writes from repositories**

Do not add new notification side effects inside repository methods. Existing old `message` writes should be retired or isolated as part of this task.

- [ ] **Step 2: Inject notification publisher into services**

Service constructors should accept `notification.Publisher` where events are produced. Keep nil-safe behavior only in tests where the event is not relevant.

- [ ] **Step 3: Publish events after successful business mutation**

Examples:

```text
comment created -> comment_created
reply created -> reply_created
article liked -> article_liked
moment liked -> moment_liked
guestbook liked -> guestbook_liked
```

Use content snapshots from service-layer DTO/aggregate data.

- [ ] **Step 4: Write and run tests**

Cover each event-producing path:

- successful mutation publishes one event.
- self actions either publish and get skipped by dispatcher, or are skipped at publisher call by explicit rule.
- business mutation still returns error if publisher fails only when the event is part of the same transaction requirement.

Run related tests:

```bash
go test ./internal/service/comment -count=1
go test ./internal/service/article -count=1
go test ./internal/service/moment -count=1
go test ./internal/service/guestbook -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/service/comment internal/service/article internal/service/moment internal/service/guestbook internal/router/router.go
git commit -m "feat(notification): 接入业务通知事件"
```

## Task 7: Email Quota Service

**Files:**

- Create: `internal/service/notification/quota.go`
- Extend: `internal/repository/notification/quota.go`
- Test: `internal/service/notification/quota_test.go`

- [ ] **Step 1: Write quota tests first**

Cover:

- register code purpose has reserved quota.
- notification cannot consume quota reserved for password reset.
- actor daily limit defers notification task.
- recipient daily limit defers notification task.
- admin role still has limits.
- provider safe daily limit and minute/hour limits are enforced.

Run: `go test ./internal/service/notification -run Quota -count=1`

Expected: FAIL until quota service exists.

- [ ] **Step 2: Implement quota evaluator**

Inputs:

```text
purpose
actor_user_id
recipient_user_id
actor_roles
recipient_roles
now
```

Output:

```text
allowed bool
defer_until time.Time
reason string
```

- [ ] **Step 3: Implement atomic reservation**

Quota reservation must be in the same transaction that changes batch status to sending, so multiple workers cannot over-consume.

- [ ] **Step 4: Verify**

Run: `go test ./internal/service/notification -run Quota -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/notification/quota.go internal/repository/notification/quota.go internal/service/notification/quota_test.go
git commit -m "feat(notification): 新增邮件额度服务"
```

## Task 8: Email Planner

**Files:**

- Create: `internal/service/notification/email_planner.go`
- Test: `internal/service/notification/email_planner_test.go`

- [ ] **Step 1: Write planner tests first**

Cover:

- same recipient receives article comment, moment comment, guestbook reply in one digest batch.
- different recipients are split into different batches.
- task outside digest window remains pending.
- actor over limit is deferred.
- recipient over limit is deferred.
- batch item count is capped and overflow remains pending.

Run: `go test ./internal/service/notification -run EmailPlanner -count=1`

Expected: FAIL until planner exists.

- [ ] **Step 2: Implement planner**

Expose:

```go
PlanOnce(ctx context.Context, workerID string, limit int) (int, error)
```

Planner should group by:

```text
recipient_user_id
to_email
purpose
digest window
priority bucket
```

Do not group by root object; cross-object digest is required.

- [ ] **Step 3: Verify**

Run: `go test ./internal/service/notification -run EmailPlanner -count=1`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/service/notification/email_planner.go internal/service/notification/email_planner_test.go
git commit -m "feat(notification): 新增邮件摘要规划器"
```

## Task 9: Email Sender And Templates

**Files:**

- Modify: `pkg/email/email.go`
- Create: `internal/service/notification/email_sender.go`
- Create: `internal/service/notification/email_template.go`
- Test: `pkg/email/*_test.go`
- Test: `internal/service/notification/email_sender_test.go`

- [ ] **Step 1: Extend mail sender interface**

Keep verification code support and add generic HTML sending:

```go
type MailSender interface {
    SendVerificationCode(to, code string) error
    SendHTML(to string, subject string, htmlBody string, messageID string) error
}
```

Update auth tests and mocks accordingly.

- [ ] **Step 2: Write sender tests first**

Cover:

- sender checks quota before SMTP call.
- successful send marks batch and tasks sent.
- SMTP failure records send log and schedules retry.
- exceeded minute limit defers batch without SMTP call.
- rendered digest includes multiple event types.

Run:

```bash
go test ./pkg/email -count=1
go test ./internal/service/notification -run EmailSender -count=1
```

Expected: FAIL until sender exists.

- [ ] **Step 3: Implement sender**

Expose:

```go
SendOnce(ctx context.Context, workerID string, limit int) (int, error)
```

Send batches with low concurrency first. Respect `send_interval_seconds`.

- [ ] **Step 4: Verify**

Run:

```bash
go test ./pkg/email -count=1
go test ./internal/service/notification -run EmailSender -count=1
go test ./internal/service/auth -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/email internal/service/notification/email_sender.go internal/service/notification/email_template.go
git commit -m "feat(notification): 新增邮件摘要发送器"
```

## Task 10: Worker Bootstrap

**Files:**

- Create: `internal/worker/notification/worker.go`
- Modify: `internal/bootstrap/bootstrap.go`
- Modify: `cmd/server/main.go`
- Test: `internal/worker/notification/*_test.go`

- [ ] **Step 1: Write worker tests**

Cover:

- worker disabled by config does not start loops.
- worker stops on context cancellation.
- dispatcher, planner, and sender continue after one iteration error.

Run: `go test ./internal/worker/notification -count=1`

Expected: FAIL until worker exists.

- [ ] **Step 2: Implement worker runner**

Loops:

```text
dispatcher every 5s
planner every 30s
sender every send_interval_seconds
lease recovery is implicit by lease queries
```

- [ ] **Step 3: Wire server startup**

Start workers after router dependencies are built. Use context cancellation where available.

- [ ] **Step 4: Verify**

Run:

```bash
go test ./internal/worker/notification -count=1
go test ./cmd/server -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/notification internal/bootstrap/bootstrap.go cmd/server/main.go
git commit -m "feat(notification): 启动通知后台任务"
```

## Task 11: SSE Stream

**Files:**

- Create: `internal/service/notification/sse.go`
- Create: `internal/handler/notification/stream.go`
- Modify: `internal/router/router.go`
- Test: `internal/service/notification/sse_test.go`
- Test: `internal/handler/notification/stream_test.go`

- [ ] **Step 1: API Risk Pass**

- Who can call: logged-in users only.
- Input bounds: optional `Last-Event-ID`; no body.
- Resource cost: long-lived connection; enforce heartbeat and disconnect on context done.
- Failure convergence: SSE loss is repaired by list API.
- Old state cleanup: hub removes connection on disconnect.

- [ ] **Step 2: Write SSE tests first**

Cover:

- hub registers and unregisters user connections.
- publishing to one user does not publish to another user.
- handler sets correct SSE headers.
- handler exits when request context is canceled.

Run:

```bash
go test ./internal/service/notification -run SSE -count=1
go test ./internal/handler/notification -run Stream -count=1
```

Expected: FAIL until SSE exists.

- [ ] **Step 3: Implement SSE hub and handler**

Route:

```go
authed.GET("/notifications/stream", handlers.notification.Stream)
```

Dispatcher should call hub after inbox creation.

- [ ] **Step 4: Verify**

Run:

```bash
go test ./internal/service/notification -run SSE -count=1
go test ./internal/handler/notification -run Stream -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/notification/sse.go internal/handler/notification/stream.go internal/router/router.go
git commit -m "feat(notification): 新增通知 SSE 推送"
```

## Task 12: Admin APIs And Swagger

**Files:**

- Create: `internal/dto/admin_notification.go`
- Create: `internal/handler/notification/admin.go`
- Modify: `internal/router/router.go`
- Generated: Swagger docs
- Test: `internal/handler/notification/admin_test.go`

- [ ] **Step 1: API Risk Pass**

- Who can call: admin only.
- Input bounds: pagination max 50; quota values must be non-negative and bounded.
- Resource cost: DB-only; retry endpoint only changes status.
- Failure convergence: retry does not send immediately, only returns batch to pending.
- Old state cleanup: no deletion endpoint in first version.

- [ ] **Step 2: Write admin handler tests**

Cover:

- list email tasks binds status and pagination.
- list batches binds status and pagination.
- quota update rejects negative or excessive values.
- retry failed batch changes status through service.

Run: `go test ./internal/handler/notification -run Admin -count=1`

Expected: FAIL until admin handler exists.

- [ ] **Step 3: Implement admin handlers and routes**

Routes:

```go
admin.GET("/notifications/email-tasks", handlers.notificationAdmin.ListEmailTasks)
admin.GET("/notifications/email-batches", handlers.notificationAdmin.ListEmailBatches)
admin.GET("/notifications/email-quotas", handlers.notificationAdmin.ListQuotas)
admin.PUT("/notifications/email-quotas/:id", handlers.notificationAdmin.UpdateQuota)
admin.PUT("/notifications/role-quotas/:id", handlers.notificationAdmin.UpdateRoleQuota)
admin.POST("/notifications/email-batches/:id/retry", handlers.notificationAdmin.RetryBatch)
```

- [ ] **Step 4: Generate Swagger**

Run: `make swag`

Expected: generated docs contain `/notifications` and `/admin/notifications` paths.

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/handler/notification -run Admin -count=1
go test ./internal/router -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/dto/admin_notification.go internal/handler/notification/admin.go internal/router/router.go docs
git commit -m "feat(notification): 新增通知管理接口"
```

## Task 13: Legacy Message Migration And Cleanup

**Files:**

- Modify: `cmd/migrate/main.go`
- Optional Create: `cmd/notification-migrate/main.go`
- Optional Create: `internal/migration/schema/20260623_001_create_notification_tables.sql`
- Optional Create: `internal/migration/schema/20260623_002_seed_notification_quota.sql`
- Modify after migration: old `internal/model/message.go` and old message writes
- Test: `cmd/migrate/*_test.go`

- [ ] **Step 1: Decide schema migration mode**

Pick one path for the target environment:

- Rebuild/dev path: add new models to `cmd/migrate/main.go` `AutoMigrate`.
- Existing production path: create versioned SQL or Go schema migrations for new notification tables and seed policies.

Do not use app startup to AutoMigrate production databases.

- [ ] **Step 2: Create new table migration**

Create all notification tables from the spec:

```text
notification_event
notification_inbox
notification_preference
notification_email_task
notification_email_batch
notification_email_batch_item
email_quota_policy
email_role_quota_policy
email_quota_usage
email_send_log
```

Include unique keys:

```text
notification_inbox(recipient_user_id, event_id)
notification_email_task(idempotency_key)
notification_email_batch_item(task_id)
email_quota_policy(purpose)
email_role_quota_policy(role, scope_type)
email_quota_usage(scope_type, scope_id, purpose, window_type, window_start)
```

- [ ] **Step 3: Create seed migration**

Seed default `email_quota_policy`:

```text
register_code
password_reset
security
notification
admin_notice
```

Seed default `email_role_quota_policy` for normal, vip, and admin actor/recipient limits.

- [ ] **Step 4: Decide legacy data migration mode**

Use one of:

- migrate during full old database migration in `cmd/migrate/main.go`.
- create one-time internal migration command for already migrated databases.

Do not delete old tables until production data has been verified.

- [ ] **Step 5: Write migration tests**

Cover:

- old `message` becomes `notification_event`.
- old `user_message` becomes `notification_inbox`.
- orphan old rows are skipped.
- old content that does not map cleanly is preserved in `metadata_json`.
- repeated migration is idempotent and does not duplicate events or inbox rows.
- per-user unread counts match between old and new tables for migrated rows.

Run: `go test ./cmd/migrate -count=1`

Expected: FAIL until migration exists.

- [ ] **Step 6: Implement legacy type mapping**

Map known old types:

```text
post_like/article_like -> article_liked
say_like/moment_like -> moment_liked
comment -> comment_created
moment_comment/say -> comment_created
comment_reply -> reply_created
guestBook -> guestbook_created
guestBook_reply -> reply_created
unknown -> legacy_notice
```

- [ ] **Step 7: Implement legacy migration**

For full old Java source migration:

- Change `migrateMessage` to create `notification_event`, or add a new step immediately after old message import to convert old rows.
- Change `migrateUserMessage` to create `notification_inbox`, or add a new conversion step.
- Preserve `legacy_message_id`, `legacy_user_message_id`, old type, and old relation IDs in `metadata_json`.

For already migrated Go databases:

- Implement `cmd/notification-migrate`.
- Read current `message` and `user_message`.
- Write v2 rows with deterministic idempotency keys such as `legacy:message:{id}`.
- Skip rows whose `message_id` has no parent message.

- [ ] **Step 8: Add verification command output**

Migration command must print:

```text
events_created
events_skipped
inbox_created
inbox_skipped
orphans_skipped
failed_rows
old_unread_count
new_unread_count
```

- [ ] **Step 9: Verify**

Run: `go test ./cmd/migrate -count=1`

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add cmd/migrate cmd/notification-migrate internal/migration/schema
git commit -m "feat(notification): 迁移旧消息数据"
```

## Task 14: Full Verification

**Files:**

- All touched files

- [ ] **Step 1: Run focused package tests**

Run:

```bash
go test ./internal/repository/notification -count=1
go test ./internal/service/notification -count=1
go test ./internal/handler/notification -count=1
go test ./internal/worker/notification -count=1
go test ./pkg/email ./pkg/config -count=1
```

Expected: PASS.

- [ ] **Step 2: Run related business tests**

Run:

```bash
go test ./internal/service/comment ./internal/service/article ./internal/service/moment ./internal/service/guestbook -count=1
go test ./internal/handler/comment ./internal/handler/article ./internal/handler/moment ./internal/handler/guestbook -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Swagger verification**

Run:

```bash
make swag
```

Expected: generated Swagger contains notification public/admin paths and no `model.*` response types.

- [ ] **Step 5: Manual smoke test**

Run server locally and verify:

```text
1. Login user A and user B.
2. B comments on A's article.
3. Dispatcher creates inbox row for A.
4. A sees unread count increment.
5. Email task is pending, not sent synchronously.
6. Planner creates a batch after digest window.
7. Sender respects quota and marks batch sent or deferred.
8. SSE sends event when A is online.
```

- [ ] **Step 6: Final commit**

```bash
git add .
git commit -m "feat(notification): 完成用户通知系统"
```

## Open Decisions Before Implementation

- Confirm production MySQL version supports `FOR UPDATE SKIP LOCKED`; otherwise use conditional update leases.
- Confirm first release default notification email policy: comments and replies only, likes in-app only.
- Confirm digest window default: 15 minutes or 30 minutes.
- Confirm initial `site_daily_safe_limit`: recommended 300, below Aliyun 2000 quota.
- Confirm whether notification workers run in the API process first or as a separate command later.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-23-notification-system.md`.

Two execution options:

1. Subagent-Driven (recommended): dispatch a fresh subagent per task, review between tasks, fast iteration.
2. Inline Execution: execute tasks in this session using executing-plans, batch execution with checkpoints.
