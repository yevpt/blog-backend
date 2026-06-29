# 用户内容审核系统设计（个人博客实用版）

## 1. 目标与边界

本站是个人生活与技术分享博客。本设计只为登录用户发布的评论、回复、留言和碎语增加敏感内容过滤，不建设通用内容治理平台。

核心目标：

- 使用零外部 API 成本的本地文本规则完成低、中、高风险分级。
- 低风险先展示后审核，中风险先审后发，高风险直接拒绝。
- 支持管理员通过、修正后通过、驳回，并保存审核记录。
- 编辑待审内容时能够回退最后通过版本，包括正文、图片和顺序。
- 未审核图片只展示低清预览，GIF 展示固定审核占位图。
- 复用全站已通过图片，减少重复审核。
- 用少量可配置规则自动调整用户信任等级，管理员可手工禁言或封禁。
- 保持状态、事务和配置清楚，便于后续 AI 阅读与维护。

当前不实现：

- 举报和申诉。
- 手机号、身份证等真实身份认证。
- 付费审核 API、本地大模型或图片识别。
- IP 网段批处理、设备画像和封禁账号关联识别。
- 自动禁言、自动封禁和递进式封禁。
- 通用批量任务平台。
- 线上旧版本兼容和灰度双写；上线时直接部署完整新版。
- 已签发 CDN 授权 URL 的即时撤销。
- 用户昵称、头像、简介和站点链接审核。

本方案不是法律意见，也不宣称实现完整监管合规。

## 2. 适用内容与角色

纳入审核：

- `moment`
- `article_comment`
- `moment_comment`
- `guestbook`
- `article_comment_reply`
- `moment_comment_reply`
- `guestbook_reply`

角色规则：

- 所有普通注册用户均可发布，不设白名单，但必须进入统一审核流程。
- 管理员发布内容及图片直接通过；人工审核仍记录管理员 ID 和时间。
- 历史用户迁移为 `trusted`。
- 历史公开内容及其当前图片迁移为已通过。

## 3. 总体架构

新增独立 `moderation` 模块，原评论、留言和碎语模块继续维护业务关系。

- `internal/service/moderation`
  - `Classifier`：文本归一化、规则匹配和风险分级。
  - `PolicyDecider`：根据风险、用户等级和全站控制计算动作。
  - `TransitionEngine`：纯函数状态机，输入快照和事件，输出 `TransitionPlan`。
  - `ReviewService`：管理员通过、修正、驳回。
  - `GovernanceService`：轻量用户等级、禁言和封禁。
  - `ControlService`：全站开关和用户内容紧急隐藏。
- `internal/repository/moderation`
  - 持久化审核项、版本、规则、图片、日志和用户档案。
  - `ApplyTransition(ctx, plan)` 在单个 MySQL 事务中更新业务快照和审核状态。
  - 内部按 `comment`、`guestbook`、`moment` 拆分 subject adapter。
- `internal/handler/moderation`
  - 管理审核、规则、用户治理和全站控制接口。
- `internal/worker/moderation`
  - 仅负责临时对象、孤儿对象、模糊预览和过期图片审核记录清理。

原业务 service 通过构造注入的审核门面提交操作，不直接读审核表或拼接状态。

### 3.1 状态转换计划

`TransitionEngine` 不访问数据库、Redis、对象存储或通知服务。`TransitionPlan` 描述：

- 新的生命周期、公开状态和版本指针。
- 要物化到业务表的正文与图片版本。
- 新增或更新的审核版本。
- 用户档案计数和等级变化。
- 操作日志与站内通知意图。
- 事务提交后需要清理的对象。

封闭事件枚举：

- `submit`、`resubmit`
- `approve`、`correct_and_approve`、`reject`
- `delete`、`admin_delete`
- `emergency_hide`、`restore`

新增事件必须同时补充状态矩阵、非法转换和表驱动测试。业务 service 不允许绕过状态机更新版本指针。

### 3.2 代码可理解性

- 状态、动作、事件和内容类型使用具名常量，禁止魔法字符串。
- moderation 各包使用 `doc.go` 说明职责和禁止事项。
- 状态机、规则、图片、治理和控制分别放在小文件中。
- 中文注释只解释事务、回退、清理和安全边界。
- 状态矩阵和策略矩阵以表驱动测试作为可执行规格。

## 4. 数据模型

### 4.1 `moderation_item`

- `content_type`、`content_id`：联合唯一业务标识。
- `author_id`
- `lifecycle_state`：`active`、`deleted`
- `public_state`：`visible`、`placeholder`、`hidden`、`emergency_hidden`
- `materialized_revision_id`：当前物化到业务表的版本，可为空。
- `approved_revision_id`：最后正式通过版本，可为空。
- `pending_revision_id`：当前唯一待审版本，可为空。
- `state_before_emergency`：紧急隐藏前状态；其他时间为空。
- `emergency_hidden_reason`、`emergency_hidden_at`
- `deleted_at`：删除终止态时间；不使用 GORM `DeletedAt` 隐藏审核墓碑。
- `lock_version` 与标准创建、更新时间。

派生字段不落库：

- `has_pending_revision = pending_revision_id != NULL`
- `display_version`：`pending`、`last_approved`、`none`
- `can_interact`：仅未删除、公开可见、没有待审版本且存在通过版本时为真。

### 4.2 `moderation_revision`

用户每次首次发布或编辑产生一个版本：

- `item_id`、`version`；`(item_id, version)` 唯一。
- `submitter_id`、`idempotency_key`；`(submitter_id, idempotency_key)` 唯一。
- `submitted_content`：用户原文，创建后不修改。
- `published_content`：经过安全清洗的实际发布文本；未修正时等于用户原文的安全清洗结果。
- `risk_level`：`low`、`medium`、`high`
- `policy_action`：`auto_approve`、`post_review`、`pre_review`、`block`
- `review_status`：`pending`、`approved`、`rejected`、`superseded`
- `ruleset_version`、`rule_match_ids`
- `decision_type`：待审时为空，处理后为 `approved`、`corrected`、`rejected` 或 `legacy_migration`。
- `decision_reason`、`reviewer_id`、`reviewed_at`

管理员修正不覆盖 `submitted_content`，只写 `published_content` 和决策字段，因此同一版本同时保存原文、修正文、理由、管理员和时间。

高风险首次发布不创建业务内容、审核项或版本；高风险编辑不改变旧版本。只保存最小 `moderation_attempt`：

- 用户、内容类型、可选审核项。
- `idempotency_key`。
- 规则集版本、命中规则和时间；不保存完整高风险正文或可用于猜测正文的无密钥摘要。

`(user_id, idempotency_key)` 唯一，网络重试只返回首次阻断结果，不重复计分。

### 4.3 图片表

`moderation_revision_image` 保存版本完整图片快照：

- `revision_id`、`seq`
- `object_key`
- `sha256`、`md5`、`size`
- `media_type`、`is_gif`

`moderation_image` 保存全站图片审核状态：

- `sha256 + size` 联合唯一，作为最终可信指纹。
- `md5` 只兼容现有 32 位 MD5 文件名并查找候选。
- `status`：`pending`、`approved`；新图保存预览后为待审，任一引用版本通过后全站转为已通过。
- `preview_object_key`
- `approved_at`、`approved_by`、`last_used_at`

每次命中已通过图片时更新 `last_used_at`。正文回退必须同时恢复图片集合和顺序。

### 4.4 其他表

- `moderation_rule`：规则、风险、优先级、启用状态和规则集版本。
- `moderation_attempt`：高风险阻断最小记录。
- `moderation_action_log`：提交、通过、修正、驳回、删除、隐藏、恢复和用户等级调整。
- `moderation_visible_image`：评论、回复和留言当前物化图片；碎语继续使用 `moment_media`。
- `user_moderation_profile`：用户当前信任、处罚和简单计数。
- `moderation_control`：全站注册和发布模式。

不创建举报、申诉、用户事件、批量任务和来源画像表。

### 4.5 `user_moderation_profile`

- `user_id`：唯一。
- `trust_level`：`new`、`normal`、`trusted`、`restricted`
- `trust_source`：`auto`、`manual`
- `manual_trust_locked`
- `sanction_state`：`active`、`muted`、`banned`
- `sanction_until`、`sanction_reason`
- `clean_approval_streak`、`corrected_count`、`rejected_count`、`high_risk_count`
- `violation_score`、`last_violation_at`、`restricted_until`
- 标准时间字段。

操作日志是审核事实，档案计数是查询投影。审核事务同时追加日志和更新计数；漂移时可从日志重建。

注册完成后尽力执行幂等 `EnsureNewProfile`。首次审核交互再次补建；档案缺失按 `new + active` 处理。

## 5. 文本规则引擎

处理顺序：

1. 校验正文长度、链接和图片数量。
2. 使用安全 Markdown/HTML 解析器清洗展示内容，移除脚本、事件属性、危险协议和非允许标签。
3. 从清洗后内容提取纯文本、链接和广告信号。
4. 执行 Unicode、繁简、全半角和大小写归一。
5. 去除零宽字符，折叠异常分隔符和重复字符。
6. 执行关键词、Go RE2 正则和组合规则。
7. 取所有命中规则的最高风险。

管理员修正文也执行相同的长度、图片归属和安全清洗，但不进入普通用户风险决策。

- `low`：未命中风险规则。
- `medium`：广告、辱骂、上下文不明确或需要人工判断。
- `high`：明确需要阻断的敏感规则。

规则集要求：

- 规则变更先校验、编译并生成单调递增 `ruleset_version`。
- 成功后原子替换内存不可变快照。
- 规则内容不可原地覆盖；修改时创建新规则记录并停用旧记录，删除也只停用，确保历史版本能按 ID 追溯。
- `enforce` 模式下有效规则集不能为空。
- 冷启动加载失败或规则集为空时，普通用户发布降级为中风险。
- 运行期数据库失败时继续使用最后成功快照并告警。

## 6. 风险与用户策略

动作：

- `auto_approve`：直接正式通过。
- `post_review`：先展示待审内容。
- `pre_review`：保存待审版本，不向公众返回正文。
- `block`：拒绝操作。

| 用户等级 | 干净低风险 | 含未审核图片 | 外链或广告 | 中风险 | 高风险 |
| --- | --- | --- | --- | --- | --- |
| `new` | `post_review` | `pre_review` | `pre_review` | `pre_review` | `block` |
| `normal` | `post_review` | `post_review` | `post_review` | `pre_review` | `block` |
| `trusted` | `auto_approve` | `post_review` | `pre_review` | `pre_review` | `block` |
| `restricted` | `pre_review` | `pre_review` | `pre_review` | `pre_review` | `block` |

决策优先级：

1. 管理员发布直接通过。
2. `muted` 或 `banned` 阻断发布、编辑和临时图片上传。
3. 全站 `closed` 阻断普通用户发布和编辑。
4. `pre_review_all` 把自动通过、先发后审降为先审后发。
5. 用户等级策略。
6. 文本风险、图片、链接和广告信号。

处罚用户仍可登录、阅读、查看通知和删除自己内容。

### 6.1 轻量自动等级

只在通过、修正、驳回或高风险阻断后同步评估，不运行复杂评分 Worker：

- `new -> normal`：达到配置的账号年龄和干净通过次数，当前违规分为零。
- `normal -> trusted`：达到更高账号年龄和干净通过次数，当前违规分为零。
- 达到限制分数：进入 `restricted` 并设置 `restricted_until`。
- 修正、驳回和高风险阻断会把连续干净次数清零；受限期内再次违规会把期限顺延为“本次违规时间 + 配置时长”。
- 干净通过按 `clean_approval_score_decay` 逐步抵消违规分，避免一次轻微修正永久阻断晋升。
- 限制到期后，在下一次发布或读取本人档案时懒恢复为 `normal`，同时重置违规分。
- 管理员锁定的信任等级不被自动覆盖。
- `muted`、`banned` 仅由管理员设置和释放。
- 手工处罚可设置截止时间；到期后在下一次权限检查时懒恢复为 `active`，截止时间为空表示必须由管理员释放。

默认计分：

- 管理员修正：`+1`
- 人工驳回：`+3`
- 高风险阻断：`+5`

同一审核版本或同一高风险请求幂等键只能计分一次。

## 7. 发布、编辑与互动

### 7.1 首次发布

| 动作 | 公众正文 | 图片 | 提示 |
| --- | --- | --- | --- |
| `auto_approve` | 直接展示 | 原图 | 发布成功 |
| `post_review` | 展示全文并标记待审核 | 已通过图用原图；新静态图用预览；GIF 用占位 | 发布成功，内容会被审核 |
| `pre_review` | 不返回正文，只显示审核占位 | 静态图用预览；GIF 用占位 | 内容已提交，等待人工审核 |
| `block` | 不创建内容 | 清理临时对象 | 内容存在较高风险，未能发布，请修改后重试 |

### 7.2 编辑

- 低风险先发后审：立即展示新正文并标记待审核。
- 中风险先审后发：继续展示最后通过正文和图片。
- 高风险：拒绝编辑，最后通过版本不变。
- 再次编辑待审内容创建新版本，旧待审版本标记 `superseded`。
- 同一审核项只能有一个 `pending_revision_id`。

### 7.3 互动限制

存在待审版本的内容可以按规则展示，但不可点赞、回复或作为新评论目标。公开 DTO 返回 `can_interact=false`，后端必须重复校验。

只有同时满足以下条件才允许互动：

- 内容未删除且 `public_state=visible`。
- `pending_revision_id=NULL`。
- `approved_revision_id` 非空。
- 原业务内容本身允许评论或点赞。

这样首次先发后审内容被驳回时不会留下新回复、点赞和通知。

## 8. 状态不变量

| 场景 | 生命周期 | 公开状态 | 物化版本 | 通过版本 | 待审版本 |
| --- | --- | --- | --- | --- | --- |
| 首次先发后审 | `active` | `visible` | 新待审 | `NULL` | 新待审 |
| 首次先审后发 | `active` | `placeholder` | `NULL` | `NULL` | 新待审 |
| 已正式通过 | `active` | `visible` | 通过版本 | 同一版本 | `NULL` |
| 通过后先发后审编辑 | `active` | `visible` | 新待审 | 旧通过 | 新待审 |
| 通过后先审后发编辑 | `active` | `visible` | 旧通过 | 旧通过 | 新待审 |
| 首次发布被驳回 | `active` | `hidden` | `NULL` | `NULL` | `NULL` |
| 紧急隐藏 | `active` | `emergency_hidden` | 保留 | 保留 | `NULL` |
| 已删除 | `deleted` | `hidden` | `NULL` | 保留供审计 | `NULL` |

约束：

- 活动态 `deleted_at` 为空；删除态非空。
- `emergency_hidden` 只用于无待审版本的已通过公开内容。
- 紧急隐藏时 `state_before_emergency=visible`，其他状态为空。
- `display_version` 和 `can_interact` 只由状态字段派生。
- 非法组合由状态机拒绝并记录。

### 8.1 删除

- `delete` 与 `admin_delete` 从任意活动状态进入不可逆 `deleted`。
- 删除时清空物化、待审和紧急隐藏字段；待审版本标记 `superseded`。
- 业务删除和审核墓碑在同一 MySQL 事务提交。
- 重复删除幂等，不重复通知或清理。
- 已删除内容禁止提交、重投、通过、修正、驳回、隐藏和恢复。
- 管理审核命中删除态返回 `409 / CONTENT_ALREADY_DELETED`。

删除父内容时，同一事务把直接和间接子评论、回复审核项一并转为 `deleted`，再复用业务模块的关联清理，不能只硬删除业务行。

### 8.2 紧急隐藏

- 仅管理员可隐藏已通过、无待审版本且当前公开的内容。
- 首次隐藏保存 `visible`，重复隐藏不覆盖。
- `restore` 只能恢复该快照，成功后清空原因、时间和快照。
- 删除状态不能恢复。
- 一键隐藏或恢复某用户内容在请求内按固定批次同步执行，每批幂等并返回处理数量；不保存后台批任务。个人博客超过单次总量上限时停止后续批次并返回已处理游标，管理员携带游标继续执行。

## 9. 人工审核

审核请求携带当前待审版本 ID。版本被再次编辑替换时返回 `409 Conflict`。

### 9.1 通过

- 待审版本成为通过和物化版本，公开状态变为 `visible`。
- 图片登记为全站已通过。
- 事务提交后删除模糊预览；失败由定期清理补偿。
- 写操作日志、站内通知并增加干净通过计数。

### 9.2 修正后通过

- 保留 `submitted_content` 原文。
- `published_content` 保存管理员修正文。
- 保存修正理由、管理员 ID 和时间。
- 管理员可移除版本图片，不能引用无关用户对象。
- 修正文和最终图片成为通过快照。
- 通知发布者内容经修正后发布，并包含修正理由。
- 增加修正次数和违规分。

### 9.3 驳回

- 首次发布：改为 `hidden`，清空业务正文和可见图片。
- 先发后审编辑：正文和图片原子回退最后通过版本。
- 先审后发编辑：业务快照保持最后通过版本。
- 清空待审指针，版本标记 `rejected`。
- 通知发布者公开驳回理由，增加驳回次数和违规分。

发布者不能恢复被驳回版本，只能重新编辑并提交新版本。

## 10. 图片处理

### 10.1 展示与复用

- 图片不参与文本风险判定。
- 未审核静态图生成一次最长边默认 48 像素的低清预览。
- 前端可叠加模糊样式，但未通过响应不返回原图地址。
- 未审核 GIF 返回固定 GIF 审核占位图。
- 预览失败返回固定静态占位图。
- 先用 MD5 文件名查候选，再以 SHA-256 和大小确认已通过图片。
- 命中后使用原图并更新 `last_used_at`。
- 图片通过后删除模糊预览。

### 10.2 版本和对象生命周期

- 编辑只新增版本引用，不覆盖旧对象。
- 新版本通过前不得删除最后通过版本图片。
- 驳回按最后通过版本恢复图片和顺序。
- 新版本真正通过后才清理已移除图片。
- 仍被公开、待审或审计保留期内版本引用的对象不得删除。
- 清理过期审核记录时跳过仍被有效版本引用的记录。

对象存储与 MySQL 不做伪事务：

1. 先校验图片、准备正式对象和预览。
2. 再执行 MySQL 状态事务。
3. 事务失败时尽力删除本次新对象。
4. 定期扫描过期 `temp/`、无数据库引用的正式对象和孤儿预览。

扫描使用固定前缀、有界批次和最小对象年龄，避免误删请求中的上传。

## 11. 全站控制与用户处置

`moderation_control`：

- `registration_mode`：`open`、`closed`
- `publishing_mode`：`open`、`pre_review_all`、`closed`
- `reason`、`operator_id`、`changed_at`、`lock_version`

数据库是事实源。个人博客写入量较低，控制读取直接访问数据库，不引入 Redis 双写和缓存失效分支。

`registration_mode=closed` 同时阻止邮箱注册和 OAuth 自动建号，不影响已有用户登录。

管理端支持：

- 手工设置和锁定信任等级。
- 设置、解除 `muted` 和 `banned`。
- 隐藏或恢复某用户全部已通过公开内容。
- 关闭注册、关闭发布或强制全部先审后发。

## 12. HTTP 响应与接口

原内容接口增加：

```json
{
  "moderation": {
    "public_state": "visible",
    "display_version": "pending",
    "has_pending_revision": true,
    "pending_risk_level": "low",
    "can_interact": false
  }
}
```

图片增加 `display_mode`：`original`、`blurred`、`gif_placeholder`。

公开接口不得返回先审后发正文、命中规则、内部评分和高风险记录。作者和管理员可查看自己的待审正文、`review_status`、风险级别和公开处理理由，供编辑器显示审核标识。

管理接口：

- `GET /admin/moderation/items`
- `GET /admin/moderation/items/:id`
- `POST /admin/moderation/items/:id/approve`
- `POST /admin/moderation/items/:id/correct`
- `POST /admin/moderation/items/:id/reject`
- `GET|POST|PATCH|DELETE /admin/moderation/rules...`
- `GET /admin/moderation/users/:id`
- `PATCH /admin/moderation/users/:id/profile`
- `POST /admin/moderation/users/:id/mute|ban|release`
- `POST /admin/moderation/users/:id/hide-content|restore-content`
- `GET|PATCH /admin/moderation/control`

高风险统一返回 HTTP `422`、业务码 `CONTENT_RISK_REJECTED` 和提示：

> 内容存在较高风险，未能发布，请修改后重试。

编辑失败时额外说明原版本不受影响。

## 13. 事务、通知与幂等

- 业务正文、可见图片、审核版本、指针、用户计数和日志在同一 MySQL 事务写入。
- 审核项使用行锁或 `lock_version` 串行化并发编辑、审核和删除。
- 发布、编辑和高风险阻断统一接受 `Idempotency-Key`，防止重试重复创建版本或计分。
- 站内通知意图使用现有 `notification_event` 持久化，不创建审核邮件。
- `post_review` 正式通过前只通知作者，不通知被回复者或根内容作者。
- 驳回和删除后，通知读取根据审核状态隐藏不可用正文摘要。
- 对象清理、通知和重复审核请求使用稳定幂等键。
- 高风险审计失败仍拒绝内容并记录结构化错误。

## 14. 配置

```yaml
moderation:
  enabled: true
  # production 必须为 enforce；observe 仅用于本地规则调试。
  mode: enforce

  policy:
    new:
      clean_low: post_review
      unapproved_image: pre_review
      external_link_or_ad: pre_review
      medium: pre_review
      high: block
    normal:
      clean_low: post_review
      unapproved_image: post_review
      external_link_or_ad: post_review
      medium: pre_review
      high: block
    trusted:
      clean_low: auto_approve
      unapproved_image: post_review
      external_link_or_ad: pre_review
      medium: pre_review
      high: block
    restricted:
      clean_low: pre_review
      unapproved_image: pre_review
      external_link_or_ad: pre_review
      medium: pre_review
      high: block

  rules:
    max_pattern_chars: 500
    max_enabled_regex_rules: 200
    require_non_empty_in_enforce: true

  content:
    moment_max_chars: 800
    comment_max_chars: 2000
    guestbook_max_chars: 2000
    reply_max_chars: 2000
    max_images_per_content: 9
    max_links_per_content: 10

  image:
    max_upload_bytes: 1048576
    max_gif_bytes: 307200
    max_stored_bytes: 512000
    max_pixels: 12000000
    processing_concurrency: 2
    preview_max_edge: 48
    static_placeholder_key: "system/moderation/image-review.jpg"
    gif_placeholder_key: "system/moderation/gif-review.jpg"
    approval_retention_days: 180
    temp_retention: 24h
    orphan_min_age: 24h
    cleanup_interval: 24h
    cleanup_batch_size: 500

  governance:
    new_to_normal:
      min_age_days: 7
      clean_approvals: 3
    normal_to_trusted:
      min_age_days: 30
      clean_approvals: 20
    restricted_score_threshold: 6
    restricted_duration: 168h
    clean_approval_score_decay: 1
    violation_weights:
      corrected: 1
      rejected: 3
      high_risk_blocked: 5

  rate_limit:
    publish_per_minute: 10
    edit_per_minute: 10
    temp_upload_per_minute: 10

  control:
    default_registration_mode: open
    default_publishing_mode: open
    cache_ttl: 30s
    user_hide_batch_size: 200
    user_hide_max_items_per_request: 1000

  audit:
    attempt_retention_days: 180
    action_log_retention_days: 365
    # 不清理当前物化、最后通过或待审版本。
    obsolete_revision_retention_days: 365
    cleanup_interval: 24h
    cleanup_batch_size: 500

  notices:
    low_submitted: "发布成功，内容会被审核。"
    review_required: "内容已提交，等待人工审核。"
    high_rejected: "内容存在较高风险，未能发布，请修改后重试。"
```

适合维护的阈值、时长、限频、策略和文案进入强类型 config。以下固定在代码中：

- 状态枚举和不变量。
- `high` 必须 `block`。
- `restricted` 不能自动通过或先发后审。
- 未审核图片不能 `auto_approve`。
- 删除终止态不能恢复。
- 待审内容不能互动。
- 生产环境必须启用审核并使用 `enforce`。

启动校验数量、时长、阈值、动作、非空规则集、占位图存在性、安全图片范围和非空提示文案；正文上限不得超过现有数据库列容量。

config 启动时加载，修改后重启生效。审核规则、手工用户处置和全站控制保存在数据库并即时生效。

## 15. 迁移与实施阶段

迁移：

1. 回填历史内容为 `approved / legacy_migration`。
2. 回填当前正文和图片版本快照。
3. 分批计算历史图片 SHA-256 和大小并登记通过。
4. 历史用户建立 `trusted + active` 档案。
5. 初始化全站控制。
6. 部署完整新版并完成迁移校验后再开放发布。

迁移可重复、可断点续跑，不修改业务 ID 和对象 key，不兼容旧版应用写入。
使用 `cmd/moderation-migrate` 执行；每批提交后输出下一游标，`--verify-only` 只读校验内容指针、图片指纹、用户画像和控制单例。

实施拆为三份计划：

1. **Core**：表、规则、状态机、业务 adapter、发布编辑、互动限制和删除级联。
2. **Media & Review**：图片、版本回退、人工审核、修正、通知和对象清理。
3. **Governance & Operations**：轻量等级、手工处罚、全站控制、迁移和上线验证。

## 16. 测试与验收

### 16.1 规则与状态

- Unicode、繁简、全半角、零宽字符和分隔符归一化。
- 关键词、正则、组合规则和最高风险合并。
- HTML/Markdown 危险标签、属性和协议清理。
- 无效规则不替换快照；空规则集和加载失败降级中风险。
- 首次发布和编辑的低、中、高风险矩阵。
- 生命周期、公开状态和版本指针合法组合。
- 待审再编辑、正文图片回退、过期审核冲突。
- 修正保留原文、修正文、理由、管理员和时间。
- 删除、审核和编辑并发不能恢复删除内容。
- 紧急隐藏只接受无待审已通过内容。

### 16.2 互动、图片和治理

- 所有待审内容后端拒绝点赞和回复。
- `can_interact` 与后端判断一致。
- 父内容删除同步删除子审核项。
- MD5 候选必须经 SHA-256 和大小确认。
- 图片复用更新 `last_used_at`。
- 静态预览、GIF 占位、回退和孤儿清理。
- 缺失档案按 `new + active`。
- 晋级、限制、到期懒恢复、手工锁定和幂等计分。
- 禁言、封禁和全站模式优先于等级策略。

### 16.3 分层验证

- Repository 使用 `go-sqlmock` 验证事务、锁和迁移幂等。
- Service 使用 `gomock` 覆盖状态和策略矩阵。
- Handler 使用 `httptest` 与 `testify` 验证鉴权、绑定、脱敏和错误码。
- 完成后运行相关测试、`go test ./...`、`go vet ./...` 和 `make swag`。

## 17. 监控与已知风险

记录风险动作数量、待审队列、审核结果、图片处理与清理、用户等级、全站控制、孤儿对象和通知重试失败。

日志使用注入的 `zap.Logger`，不记录完整高风险正文、图片二进制、认证信息或具体规则内容。

接受的剩余风险：

- 本地规则无法理解完整上下文，会有误判和漏判。
- 低风险内容在管理员处理前短暂公开。
- 图片不做识别，低清预览仍可能表达轮廓。
- 只有一名管理员，待审积压时处置速度有限。
- 无手机号或实名信息，无法可靠阻止重新注册。
- 举报、申诉、IP 批量止血和自动封禁以后按实际需要另行设计。
