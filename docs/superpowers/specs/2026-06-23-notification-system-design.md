# 用户消息通知系统设计

## 背景

旧 Java 项目使用 `message` 与 `user_messages` 两张表承载站内通知，并通过多个按类型拆分的接口查询和置已读。当前 Go 项目迁移后仍保留 `message` 与 `user_message` 模型，且文章、碎语、评论等 repository 内直接写通知副作用。

这套实现能覆盖基础场景，但已经不适合后续目标：

- 邮件通知需要先进入持久队列，再按额度、频率、角色、操作人、接收人和聚合规则定时发送。
- 后续要支持 SSE 实时站内通知，但 SSE 只应该是在线通道，不能承担可靠性。
- 服务宕机、重启、重复执行时，通知和邮件任务必须可恢复、可去重、可追溯。
- 邮箱服务有供应商额度和风控风险，不能按标称上限发送。

新版通知系统以 MySQL 作为唯一持久事实源，Redis 只用于短期锁、频率辅助、SSE 多实例广播等非根基能力。

## 目标

- 建立统一通知事件模型，替代业务 repository 分散创建 `message` 的方式。
- 站内通知具备列表、未读数、已读、删除、跳转元数据和 SSE 推送能力。
- 邮件通知先沉淀为持久任务，再由 planner 聚合为邮件批次，sender 按额度和频率发送。
- 支持全站、安全邮件 purpose、角色、操作人、接收人多维度限额。
- 注册验证码、找回密码等关键邮件有固定份额和更高优先级。
- 所有 worker 支持租约恢复，服务宕机后可继续处理。
- 设计可分阶段落地，第一阶段不引入外部 MQ。

## 非目标

- 第一版不实现通用可视化规则引擎。
- 第一版不引入 RabbitMQ、Kafka、Asynq 等额外队列系统。
- 第一版不要求邮件 exactly-once。SMTP 是外部副作用，只能做到持久任务、幂等保护和尽量避免重复。
- 第一版不实现多服务实例下的强实时 SSE 一致性。单实例内存 hub 先落地，多实例后续接 Redis Pub/Sub。

## 核心概念

### 通知事件

`notification_event` 表记录一件已经发生的事实，例如：

- B 评论了 A 的碎语。
- C 回复了 A 的留言。
- D 点赞了 A 的文章。

事件不决定邮件怎么聚合。邮件聚合属于后续 planner 的运行时策略。

### 站内收件箱

`notification_inbox` 表记录某个用户是否收到某个事件，以及是否已读、是否删除。

同一个事件可以投递给多个用户。对个人博客来说通常是一对一，但系统消息或管理员通知可以一对多。

### 邮件任务

`notification_email_task` 表记录一条通知是否需要进入邮件处理。它是邮件队列的最小单位。

task 不是一封邮件。一封邮件由 planner 把多条 task 聚合成 batch。

### 邮件批次

`notification_email_batch` 表记录最终要发送的一封邮件。一个 batch 包含多条 task。

这允许后续实现：

- 同一接收人在一个时间窗口内收到多类互动时合成一封摘要。
- 超过额度后整批延后。
- 发送失败时只重试该批次。

### source 与 root

不新增 `thread_key` 字段。事件使用结构化字段表达对象关系：

- `source_type` / `source_id`：直接发生行为的对象，例如 `comment:99`、`reply:18`、`article:3`。
- `root_type` / `root_id`：最终所属的内容根，例如 `moment:12`、`guestbook:8`、`article:3`。

如果代码需要线程 key，可临时拼接 `root_type + ":" + root_id`。这避免存储冗余字符串，也避免未来命名调整造成迁移。

## 表设计

### notification_event

记录事件事实和轻量展示快照。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned | 主键 |
| `type` | varchar(40) | 事件类型，如 `comment_created`、`reply_created`、`article_liked`、`system_notice` |
| `actor_user_id` | bigint unsigned null | 操作人；系统消息可为空 |
| `source_type` | varchar(30) | 直接对象类型 |
| `source_id` | bigint unsigned | 直接对象 ID |
| `root_type` | varchar(30) | 根对象类型 |
| `root_id` | bigint unsigned | 根对象 ID |
| `title` | varchar(120) | 事件标题快照 |
| `content_excerpt` | varchar(500) | 内容摘要快照 |
| `metadata_json` | json null | 跳转、额外文案、对象快照等扩展信息 |
| `dispatch_status` | varchar(20) | `pending`、`processing`、`done`、`failed` |
| `attempts` | int | 分发尝试次数 |
| `next_process_at` | datetime | 下次可处理时间 |
| `lease_until` | datetime null | worker 租约 |
| `locked_by` | varchar(80) null | worker 标识 |
| `last_error` | varchar(1000) null | 最近错误 |
| `created_at` / `updated_at` / `deleted_at` | datetime | 通用字段 |

建议索引：

- `idx_event_dispatch`: `dispatch_status, next_process_at, lease_until`
- `idx_event_root`: `root_type, root_id, created_at`
- `idx_event_actor`: `actor_user_id, created_at`
- `idx_event_source`: `source_type, source_id`

### notification_inbox

记录站内通知收件状态。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned | 主键 |
| `event_id` | bigint unsigned | 事件 ID |
| `recipient_user_id` | bigint unsigned | 接收人 |
| `is_read` | tinyint | 是否已读 |
| `read_at` | datetime null | 已读时间 |
| `delivered_at` | datetime | 投递时间 |
| `created_at` / `updated_at` / `deleted_at` | datetime | 通用字段 |

约束和索引：

- 唯一约束：`uk_inbox_recipient_event(recipient_user_id, event_id)`
- 查询索引：`idx_inbox_recipient_created(recipient_user_id, created_at)`
- 未读索引：`idx_inbox_recipient_read(recipient_user_id, is_read, created_at)`

### notification_preference

记录用户通知偏好。现有 `user_setting.receive_mail` 继续作为总开关，细粒度偏好放入此表。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned | 主键 |
| `user_id` | bigint unsigned | 用户 |
| `event_type` | varchar(40) | 事件类型；可用 `*` 表示默认 |
| `in_app_enabled` | tinyint | 是否接收站内通知 |
| `email_enabled` | tinyint | 是否接收邮件通知 |
| `email_digest_mode` | varchar(20) | `off`、`digest`、`immediate_digest` |
| `quiet_start` / `quiet_end` | varchar(5) null | 静默时段，格式 `HH:mm` |
| `created_at` / `updated_at` | datetime | 通用字段 |

唯一约束：`uk_preference_user_event(user_id, event_type)`。

### notification_email_task

邮件待处理队列。任务由 dispatcher 创建，由 planner 聚合。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned | 主键 |
| `event_id` | bigint unsigned | 事件 ID |
| `recipient_user_id` | bigint unsigned | 接收人 |
| `actor_user_id` | bigint unsigned null | 操作人，用于 actor 限额 |
| `to_email` | varchar(155) | 发送目标邮箱快照 |
| `event_type` | varchar(40) | 事件类型快照 |
| `purpose` | varchar(40) | 邮件用途，如 `notification` |
| `priority` | int | 优先级，数字越小越优先 |
| `status` | varchar(20) | `pending`、`batched`、`sent`、`deferred`、`failed`、`skipped` |
| `available_at` | datetime | 最早可聚合时间 |
| `next_attempt_at` | datetime | 下次处理时间 |
| `attempts` | int | 尝试次数 |
| `batch_id` | bigint unsigned null | 已归属批次 |
| `lease_until` | datetime null | worker 租约 |
| `locked_by` | varchar(80) null | worker 标识 |
| `idempotency_key` | varchar(120) | 幂等键 |
| `last_error` | varchar(1000) null | 最近错误 |
| `created_at` / `updated_at` / `deleted_at` | datetime | 通用字段 |

约束和索引：

- 唯一约束：`uk_email_task_idempotency(idempotency_key)`
- 待处理索引：`idx_email_task_pick(status, next_attempt_at, available_at, priority)`
- 接收人索引：`idx_email_task_recipient(recipient_user_id, created_at)`
- 操作人索引：`idx_email_task_actor(actor_user_id, created_at)`

### notification_email_batch

最终待发送的一封邮件。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned | 主键 |
| `recipient_user_id` | bigint unsigned | 接收人 |
| `to_email` | varchar(155) | 收件邮箱快照 |
| `purpose` | varchar(40) | 邮件用途 |
| `subject` | varchar(180) | 邮件标题 |
| `status` | varchar(20) | `pending`、`sending`、`sent`、`deferred`、`failed` |
| `item_count` | int | 包含任务数 |
| `scheduled_at` | datetime | 计划发送时间 |
| `sent_at` | datetime null | 实际发送时间 |
| `attempts` | int | 尝试次数 |
| `lease_until` | datetime null | worker 租约 |
| `locked_by` | varchar(80) null | worker 标识 |
| `message_id` | varchar(120) null | 邮件 Message-ID 或内部幂等 ID |
| `last_error` | varchar(1000) null | 最近错误 |
| `created_at` / `updated_at` / `deleted_at` | datetime | 通用字段 |

索引：

- `idx_email_batch_pick(status, scheduled_at, lease_until)`
- `idx_email_batch_recipient(recipient_user_id, created_at)`

### notification_email_batch_item

连接 batch 和 task。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `batch_id` | bigint unsigned | 邮件批次 |
| `task_id` | bigint unsigned | 邮件任务 |

唯一约束：

- `uk_batch_item(batch_id, task_id)`
- `uk_batch_task(task_id)`，确保一个 task 只属于一个 batch。

### email_quota_policy

按 purpose 配置额度和频率。purpose 是可扩展的，不写死为两类。

建议 purpose：

- `register_code`：注册验证码。
- `password_reset`：找回密码。
- `security`：安全提醒。
- `notification`：评论、回复等摘要邮件。
- `admin_notice`：管理员主动通知。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned | 主键 |
| `purpose` | varchar(40) | 邮件用途 |
| `daily_limit` | int | 该 purpose 每日上限 |
| `reserved_min` | int | 该 purpose 每日保底份额 |
| `priority` | int | 全局优先级，数字越小越优先 |
| `max_per_minute` | int | 该 purpose 每分钟上限 |
| `max_per_hour` | int | 该 purpose 每小时上限 |
| `enabled` | tinyint | 是否启用 |
| `created_at` / `updated_at` | datetime | 通用字段 |

唯一约束：`uk_email_quota_policy_purpose(purpose)`。

### email_role_quota_policy

按角色限制操作人和接收人。管理员也必须有限额，只是额度可以更高。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned | 主键 |
| `role` | varchar(30) | `normal`、`vip`、`admin` |
| `scope_type` | varchar(20) | `actor` 或 `recipient` |
| `daily_limit` | int | 每日上限 |
| `max_per_hour` | int | 每小时上限 |
| `enabled` | tinyint | 是否启用 |
| `created_at` / `updated_at` | datetime | 通用字段 |

唯一约束：`uk_role_quota(role, scope_type)`。

### email_quota_usage

记录实际消耗。所有限额判断都以该表为准，服务重启后仍可继续限制。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `quota_date` | date | 日期 |
| `scope_type` | varchar(20) | `site`、`purpose`、`actor`、`recipient` |
| `scope_id` | bigint unsigned | 全站为 0，用户维度为 user_id |
| `purpose` | varchar(40) | 邮件用途 |
| `window_type` | varchar(20) | `day`、`hour`、`minute` |
| `window_start` | datetime | 统计窗口开始 |
| `used_count` | int | 已用数量 |
| `created_at` / `updated_at` | datetime | 通用字段 |

唯一约束：

`uk_quota_usage(scope_type, scope_id, purpose, window_type, window_start)`。

### email_send_log

记录每次真实发送尝试。

建议字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint unsigned | 主键 |
| `batch_id` | bigint unsigned null | 通知邮件批次 |
| `purpose` | varchar(40) | 邮件用途 |
| `to_email` | varchar(155) | 收件邮箱 |
| `status` | varchar(20) | `success` 或 `failed` |
| `provider` | varchar(40) | 邮件供应商 |
| `message_id` | varchar(120) null | 邮件 ID |
| `error` | varchar(1000) null | 错误 |
| `created_at` | datetime | 发送时间 |

## 邮件额度设计

额度分四层判断，任何一层不足都不能发送：

1. 全站安全额度：站点自己设置的每日安全上限，低于供应商标称上限。
2. purpose 额度：注册验证码、找回密码、安全提醒、通知摘要等各自有上限和保底份额。
3. actor 额度：同一个操作用户每天最多触发多少通知邮件。
4. recipient 额度：同一个接收用户每天最多收到多少通知邮件。

阿里云企业邮箱免费版每日标称 2000，不作为实际发送目标。建议默认：

- `provider_daily_hard_limit = 2000`，只作为保护参考。
- `site_daily_safe_limit = 300`，初始真实安全上限。
- `max_per_minute = 5`。
- `max_per_hour = 80`。
- `send_interval_seconds = 12`。
- `notification_daily_limit = 150`，由数据库 `email_quota_policy` 控制。

管理员也有 actor 和 recipient 限额。示例：

| 角色 | actor 日限 | recipient 日限 |
| --- | ---: | ---: |
| normal | 30 | 5 |
| vip | 100 | 20 |
| admin | 300 | 50 |

## 配置归属

### config/env

适合部署级、敏感、基础保护配置：

```yaml
email:
  provider: aliyun_enterprise
  host: smtp.qiye.aliyun.com
  port: 465
  from: noreply@example.com
  password: ""
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

环境变量优先覆盖：

- `BLOG_EMAIL_HOST`
- `BLOG_EMAIL_PORT`
- `BLOG_EMAIL_FROM`
- `BLOG_EMAIL_PASSWORD`
- `BLOG_EMAIL_WORKER_ENABLED`
- `BLOG_EMAIL_PLANNER_ENABLED`

### 数据库策略表

适合运营规则：

- `email_quota_policy`
- `email_role_quota_policy`
- `notification_preference`

### MySQL 运行状态

适合可恢复状态：

- 邮件任务状态。
- 邮件批次状态。
- quota usage。
- send log。

### Redis

适合短期辅助：

- worker 分布式启动锁。
- SSE 多实例 Pub/Sub。
- 短期频率缓存。

Redis 不作为队列可靠性来源。

## Worker 设计

### Event Dispatcher

职责：

- 领取 `notification_event.dispatch_status=pending` 的事件。
- 根据事件类型解析接收人。
- 创建 `notification_inbox`。
- 判断用户偏好和邮箱地址，创建 `notification_email_task`。
- 推送 SSE 在线消息。
- 将 event 标记为 `done`。

幂等点：

- `notification_inbox` 使用 `unique(recipient_user_id, event_id)`。
- `notification_email_task` 使用 `idempotency_key`。

### Email Planner

职责：

- 领取可处理的 `notification_email_task`。
- 按接收人、purpose、时间窗口、优先级聚合为 batch。
- 检查 actor 和 recipient 额度。
- 不足额度时将 task 标记为 `deferred`，设置 `next_attempt_at`。
- 生成 `notification_email_batch` 与 `notification_email_batch_item`。

邮件聚合不依赖 `root_type/root_id`。同一接收人在同一窗口内的碎语评论、留言回复、文章评论可以进入同一封摘要。

### Email Sender

职责：

- 领取 `notification_email_batch.status=pending` 且 `scheduled_at<=now` 的批次。
- 发送前检查全站安全额度、purpose 额度、分钟频率、小时频率。
- 渲染 HTML 邮件。
- 调用 `pkg/email` 的扩展发送接口。
- 成功后标记 batch 和 task 为 sent，写 `email_send_log`。
- 失败后按重试策略更新 `attempts` 和 `next_attempt_at`。

### Lease Recovery

所有 worker 领取任务时写入：

- `status=processing/sending`
- `lease_until=now+lease_seconds`
- `locked_by=hostname/pid/random`

如果服务宕机，其他 worker 在 `lease_until < now` 后可重新领取。

## SSE 设计

SSE 不是可靠队列，只负责在线提示。

接口：

- `GET /notifications/stream`

行为：

- 必须登录。
- 建立连接后按 `user_id` 注册到 hub。
- dispatcher 写 inbox 后推送事件。
- 前端断线重连后用普通列表接口补齐。
- 后续多实例可用 Redis Pub/Sub 广播 `user_id + inbox_id`。

## API 设计

登录用户接口：

- `GET /notifications`：分页列表，支持 `unread_only`。
- `GET /notifications/unread-count`：未读数量。
- `PATCH /notifications/:id/read`：单条已读。
- `PATCH /notifications/read`：批量已读，支持全量或 id 列表。
- `DELETE /notifications/:id`：删除自己的站内通知。
- `GET /notifications/preferences`：查询偏好。
- `PUT /notifications/preferences`：更新偏好。
- `GET /notifications/stream`：SSE。

管理员接口：

- `GET /admin/notifications/email-tasks`：查询邮件任务。
- `GET /admin/notifications/email-batches`：查询邮件批次。
- `GET /admin/notifications/email-quotas`：查询额度策略和用量。
- `PUT /admin/notifications/email-quotas/:id`：调整额度策略。
- `PUT /admin/notifications/role-quotas/:id`：调整角色额度。
- `POST /admin/notifications/email-batches/:id/retry`：重试失败批次。

## 分层边界

- handler：绑定参数、取登录用户、调用 service、统一 response、Swagger。
- service：业务规则、权限、偏好、额度、worker 编排。
- repository：GORM 查询和事务，不返回 DTO。
- dto：所有对外请求和响应。
- model：GORM 表结构，不直接返回给前端。

通知创建应从业务 service 调用 `notification.Publisher`，而不是业务 repository 直接写通知表。

## 建表与迁移策略

当前仓库的 `cmd/migrate` 是“旧 Java 库全量迁移到当前 Go 表结构”的工具，不是线上数据库的通用版本迁移系统。因此通知系统需要同时设计两条路径：

1. 新建或可重建环境：通过 `cmd/migrate` 的 `AutoMigrate` 创建新通知表，并在旧库迁移步骤中直接生成 v2 通知数据。
2. 已运行的 Go 数据库：通过版本化 schema 迁移脚本创建新表，再运行一次性数据回填任务，最后切换业务代码只写新表。

### 新表创建

开发环境和可重建迁移库：

- 在 `internal/model/notification.go` 定义新模型。
- 在 `cmd/migrate/main.go` 的 `autoMigrate` 注册新模型。
- `cmd/migrate` 全量重建时直接创建新表。

线上或不可重建环境：

- 新增版本化迁移目录，例如 `internal/migration/schema` 或 `db/migrations`。
- 每个迁移使用明确版本号和名称，例如 `20260623_001_create_notification_tables.sql`。
- 迁移脚本只做向前变更：建表、加列、加索引、种子策略数据。
- 禁止依赖应用启动时的 `AutoMigrate` 修改线上库。

通知系统第一批 schema 迁移应包含：

- 创建 `notification_event`。
- 创建 `notification_inbox`。
- 创建 `notification_preference`。
- 创建 `notification_email_task`。
- 创建 `notification_email_batch`。
- 创建 `notification_email_batch_item`。
- 创建 `email_quota_policy`。
- 创建 `email_role_quota_policy`。
- 创建 `email_quota_usage`。
- 创建 `email_send_log`。
- 插入默认 `email_quota_policy`，包含 `register_code`、`password_reset`、`security`、`notification`、`admin_notice`。
- 插入默认 `email_role_quota_policy`，包含 normal、vip、admin 的 actor 与 recipient 限额。

### 旧数据处理

旧数据来源有两类：

1. 旧 Java 源库的 `message` / `user_messages`。
2. 当前 Go 目标库已迁移出来的 `message` / `user_message`。

两类都要迁移到 v2 表，但执行入口不同。

从旧 Java 源库全量迁移时：

- 不再把旧 `message` 写入新库 `message` 表作为最终形态。
- 将旧 `message` 直接映射为 `notification_event`。
- 将旧 `user_messages` 通过 JOIN 过滤孤儿记录后映射为 `notification_inbox`。
- 无法准确映射的旧字段写入 `metadata_json`。
- 旧 `from_role` 不再进入主字段；需要追溯时放入 `metadata_json`。

从当前 Go 库增量升级时：

- 新增一次性命令，例如 `cmd/notification-migrate`。
- 从当前库 `message` / `user_message` 读取数据，写入 v2 表。
- 回填过程必须幂等：同一旧消息重复执行不能生成重复事件或重复 inbox。
- 建议在 `notification_event.metadata_json` 中保存 `legacy_message_id`，并对迁移事件建立幂等键。
- 建议在 `notification_inbox.metadata_json` 或单独迁移映射表中保存 `legacy_user_message_id`；如果不加字段，则至少用 `recipient_user_id + event_id` 唯一约束防重。

### 旧类型映射

旧类型需要规范化。第一版建议映射如下：

| 旧类型 | 新事件类型 | 说明 |
| --- | --- | --- |
| `post_like` / `article_like` | `article_liked` | 文章点赞 |
| `say_like` / `moment_like` | `moment_liked` | 碎语点赞 |
| `comment` | `comment_created` | 文章评论 |
| `moment_comment` / `say` | `comment_created` | 碎语评论 |
| `comment_reply` | `reply_created` | 评论回复 |
| `guestBook` | `guestbook_created` | 留言 |
| `guestBook_reply` | `reply_created` | 留言回复 |
| 其他未知类型 | `legacy_notice` | 保留原始 type 到 `metadata_json` |

### 新旧系统切换顺序

推荐执行顺序：

1. 新表上线，但业务仍读旧表、写旧表。
2. 部署支持新表的代码，打开“只写新通知事件”的功能开关，停止新增旧 `message`。
3. 运行旧数据回填，把旧 `message` / `user_message` 写入 v2 表。
4. 对比旧表和新表数量：事件数、收件箱数、未读数、每用户未读数抽样。
5. 前端和后端读接口切到 v2。
6. 观察一段时间后，将旧表改为只读保留。
7. 确认无回滚需求后再删除旧模型、旧 repository 副作用和旧表。

如果实现阶段不想引入功能开关，也可以选择停机维护窗口：先迁移数据，再一次性部署只读写 v2 的代码。个人博客流量较小，这条路径更简单。

### 后续 schema 迁移原则

- 加字段优先使用可空字段或有默认值字段，避免长时间锁表。
- 新增索引要评估表数据量；大表索引尽量在低峰执行。
- 先部署兼容旧字段和新字段的代码，再迁移数据，最后删除旧字段。
- 删除字段和删除表必须延后到至少一个稳定版本之后。
- 每个迁移脚本必须可追溯：版本号、目的、影响表、回滚建议。
- 数据回填任务要可重复执行，并输出处理数、跳过数、失败数。

## 风险与注意事项

- SMTP 成功但服务立即宕机时，可能重复发送同一批次。通过 `message_id`、发送日志、幂等状态减少概率，但不能承诺绝对 exactly-once。
- `FOR UPDATE SKIP LOCKED` 依赖 MySQL 8。若线上 MySQL 不支持，需要改用条件更新抢占任务。
- 邮件摘要内容必须使用创建事件时的快照，避免原评论删除后邮件内容缺失。
- 通知邮件应默认低优先级，不能挤占注册验证码和找回密码份额。
- 管理员也必须受限额保护，避免误操作或账号异常。
- SSE 断线不补发历史，由列表接口补齐。

## 分阶段落地

- 阶段 1：模型、repository、service、站内通知 API。
- 阶段 2：事件发布接入评论、回复、点赞、留言。
- 阶段 3：邮件 task、planner、quota、batch。
- 阶段 4：sender、模板、失败重试、管理查询。
- 阶段 5：SSE。
- 阶段 6：旧表迁移和清理。
