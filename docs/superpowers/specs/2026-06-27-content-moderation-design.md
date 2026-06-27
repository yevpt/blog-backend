# 用户内容审核系统设计

## 1. 背景与目标

当前评论、回复、留言和碎语由登录用户提交后直接进入业务表，评论与留言没有审核状态，碎语的公开/隐藏状态也不能表达待审、驳回、申诉等审核语义。

本设计为非管理角色提交的用户内容增加统一审核能力，目标是：

- 以零外部 API 成本完成文本风险分级。
- 在低风险内容可用性和违法、不良内容处置之间取得平衡。
- 统一支持首次发布、再次编辑、人工审核、管理员修正、举报和申诉。
- 根据用户历史行为自动调整信任等级，并支持禁言、限制发布和封禁。
- 提供全站发布开关、强制先审后发和批量隔离等紧急止血能力。
- 未审核图片不做机器识别，通过低清模糊预览或 GIF 占位图半屏蔽展示。
- 保留完整版本和图片引用，保证驳回时能够真实回退。
- 复用全站已通过图片，减少重复人工审核，并控制审核图片记录增长。

本方案不是法律意见。由于站点不要求手机号、身份证或已验证邮箱，仍存在真实身份认证方面的合规缺口；不能对外宣称已经实现完全合规。

## 2. 适用范围

### 2.1 纳入审核的内容

- 碎语 `moment`
- 文章评论 `article_comment`
- 碎语评论 `moment_comment`
- 留言 `guestbook`
- 文章评论回复 `article_comment_reply`
- 碎语评论回复 `moment_comment_reply`
- 留言回复 `guestbook_reply`

### 2.2 角色范围

- 普通注册用户：必须经过本设计的审核流程。
- 管理员：发布内容及所带图片直接视为通过；人工审核操作仍需留痕。
- 举报与申诉：仅限已登录注册用户。

### 2.3 非目标

- 不接入收费文本或图片审核 API。
- 不部署本地大模型或图片识别模型。
- 不审核文章正文等管理员生产内容。
- 首版不开放匿名举报。
- 不以本系统替代 ICP、实名、隐私保护等其他合规工作。
- 不承诺仅凭 IP、设备或 OAuth 信息可以彻底阻止被封用户重新注册。

## 3. 合规设计依据

设计重点参考以下现行规定：

- [互联网跟帖评论服务管理规定](https://www.cac.gov.cn/2022-11/16/c_1670253725725039.htm)：覆盖评论、回复、留言、点赞等服务，要求建立审核管理、实时巡查、举报申诉、用户分级和处置留痕机制。
- [网络信息内容生态治理规定](https://www.cac.gov.cn/2019-12/20/c_1578375159509309.htm)：要求网络信息内容服务平台履行内容治理主体责任。
- [互联网信息服务管理办法](https://www.samr.gov.cn/zw/zfxxgk/fdzdgknr/bgt/art/2023/art_483f0dd8eb1b4dc5961e4e008bd4a083.html)：发现明显违法内容时应停止传输、保存记录并报告。
- [互联网用户账号信息管理规定](https://www.cac.gov.cn/2022-06/26/c_1657868775042841.htm)：规定信息发布账号的真实身份认证、账号管理和举报处理要求。

产品已明确不增加手机号或邮箱验证。该决定仅是成本和体验取舍，不消除相关合规要求。

## 4. 总体架构

新增统一 `moderation` 模块，原评论、留言和碎语模块继续负责各自业务关系；审核模块统一负责风险判定、版本状态、图片审核缓存、举报、申诉和管理动作。

建议组件边界：

- `internal/service/moderation`
  - 文本归一化与本地规则引擎。
  - 发布、编辑、审核、举报和申诉状态编排。
  - 图片指纹查询、预览图策略和引用生命周期。
  - 站内通知事件发布。
  - 用户信任等级评估、违规计分和处置策略决策。
  - 全站控制策略与批量处置编排。
- `internal/repository/moderation`
  - 审核项、版本、规则、图片、举报和申诉持久化。
  - 提供跨业务内容与审核记录的事务协调能力。
- `internal/handler/moderation`
  - 登录用户举报、申诉接口。
  - 管理端审核队列、审核动作和规则管理接口。
- `internal/worker/moderation`
  - 幂等通知补偿、对象清理重试、过期图片审核记录清理。
  - 用户等级周期评估、临时限制释放和批量处置任务。
- 原有 `comment`、`guestbook`、`moment` service
  - 通过构造注入的审核接口提交内容，不直接实现审核规则。
  - 不直接访问审核表或持有数据库全局变量。

业务可见快照、审核记录和用户处置事件必须在同一数据库事务中创建或切换，避免出现内容已公开但没有审核状态，或违规已成立但用户档案未更新的中间态。

## 5. 数据模型

### 5.1 `moderation_item`

每个已创建的业务内容对应一条审核项：

- `content_type`、`content_id`：业务内容多态标识，联合唯一。
- `author_id`：内容发布者。
- `visibility_state`：`visible`、`placeholder`、`hidden`、`emergency_hidden`，只表达公开展示状态。
- `visible_revision_id`：当前已物化到业务表并向公众展示的版本，可为空。
- `approved_revision_id`：最后一次正式通过版本，可为空。
- `pending_revision_id`：当前唯一待审版本，可为空。
- `emergency_hidden_reason`、`emergency_hidden_at`：紧急隔离原因和时间。
- 乐观锁版本号及标准时间字段。

`moment.status` 继续表达作者设置的公开/隐藏状态，不复用为审核状态。

审核项不保存通用 `status`、`current_revision_id` 或 `has_pending_revision` 字段：

- 审核状态只属于具体版本，由 `moderation_revision.review_status` 表达。
- `has_pending_revision` 由 `pending_revision_id != NULL` 派生。
- `display_version` 由三个版本指针与 `visibility_state` 派生，不落库。
- 公开列表只依据 `visibility_state` 判断能否展示，不能依据待审状态推断可见性。

### 5.2 `moderation_revision`

每次首次发布、再次编辑和管理员修正均产生版本：

- `item_id`：关联审核项，不可为空。
- `version`：同一审核项内单调递增。
- `submitted_content`：用户提交原文。
- `published_content`：实际发布文本；管理员未修正时与原文相同。
- `risk_level`：`low`、`medium`、`high`。
- `review_status`：`pending`、`approved`、`rejected`、`superseded`。
- `rule_match_ids`：命中规则 ID 集合，仅管理端可见。
- `decision_type`：`approved`、`corrected`、`rejected`、`legacy_migration`。
- `decision_reason`：修正、驳回或申诉处理理由。
- `reviewer_id`、`reviewed_at`：处理管理员和时间。
- `appeal_count`：当前被驳回版本的申诉次数，最大为 3。

高风险首次发布不创建业务内容、审核项或版本；高风险编辑也不创建新业务版本。两者均写入独立的受限安全记录，原业务版本不变。安全记录默认保留 180 天，具体期限可配置。

### 5.3 `moderation_revision_image`

保存每个版本完整的图片快照：

- `revision_id`、`seq`
- `object_key`
- `sha256`、`md5`、`size`
- `media_type`
- `is_gif`：是否为 GIF。实际 `display_mode` 根据图片通过状态和版本状态在响应时计算。

正文回退时必须同时恢复该版本的图片集合和顺序。

### 5.4 `moderation_image`

全站图片审核缓存，以 `sha256 + size` 唯一识别图片内容：

- `sha256`、`size`：最终可信指纹。
- `md5`：兼容现有 32 位 MD5 文件名，用作候选快速查找，不能单独作为通过依据。
- `status`：首版主要使用 `approved`。
- `preview_object_key`：未通过前生成的低清预览对象。
- `approved_at`、`approved_by`
- `last_used_at`：每次审核判断命中该记录时更新。

现有对象 key 保持兼容；新上传在已有 MD5 的同时计算 SHA-256 和大小。正式对象禁止覆盖，内容变化必须生成新对象。

### 5.5 其他表

- `moderation_rule`：规则类型、模式、风险级别、优先级、启用状态和说明。
- `moderation_attempt`：高风险阻断尝试，保存作者、内容类型、可选审核项、受限正文快照、命中规则和时间。
- `moderation_report`：举报者、目标、原因、说明、处理状态和处理结果。
- `moderation_appeal`：版本、申诉人、理由、序号和处理结果。
- `moderation_action_log`：通过、修正、驳回、回退、举报处理等操作留痕。
- `moderation_visible_image`：为评论、回复和留言物化当前可展示图片 key、指纹和顺序；碎语继续使用 `moment_media`。
- `user_moderation_profile`：用户当前信任等级、统计计数、限制期限和封禁信息。
- `user_moderation_event`：通过、修正、驳回、高风险阻断、举报成立等不可变事件，作为自动评级依据。
- `moderation_control`：全站注册和发布控制状态，数据库为事实源，Redis 仅作短期缓存。
- `moderation_bulk_action`：批量隔离、恢复和永久删除任务及其确认、进度、操作者和结果。

至少建立以下约束和索引：审核项 `(content_type, content_id)` 联合唯一、图片 `(sha256, size)` 联合唯一、同一举报者和目标只有一条有效举报，并为待审状态、风险级别、创建时间及图片 `last_used_at` 建立查询索引。

### 5.6 `user_moderation_profile`

`user.status` 继续表示整个账号是否允许登录；内容治理使用独立档案，避免“禁止发言”意外变成“无法登录和查看通知”。档案以 `user_id` 唯一，至少包含：

- `trust_level`：`new`、`normal`、`trusted`、`restricted`、`banned`。
- `level_source`：`auto` 或 `manual`。
- `manual_level_locked`：管理员手工等级是否阻止自动评估覆盖。
- `approved_count`、`clean_approved_count`、`corrected_count`、`rejected_count`、`high_risk_count`、`upheld_report_count`。
- `violation_count`、`violation_score`、`consecutive_clean_approvals`。
- `last_violation_at`、`last_evaluated_at`、`level_changed_at`。
- `muted_until`：临时禁止发布、举报和申诉的截止时间。
- `restricted_until`：自动限制等级的释放时间。
- `ban_count`、`banned_at`、`banned_until`、`ban_reason`。
- `manual_release_only`：多次封禁后只能由管理员解除。

注册和 OAuth 自动创建用户时，必须在同一事务中创建 `new` 档案；历史用户迁移为 `trusted`。运行时发现用户缺少档案时按 `new` 处理并异步补建，不能默认可信。

### 5.7 用户处置事件

自动等级不直接依赖可被重复累加的计数字段，而以幂等 `user_moderation_event` 为事实源，档案计数是投影。事件至少包括：

- `clean_approved`
- `corrected`
- `rejected`
- `high_risk_blocked`
- `report_upheld`
- `manual_adjusted`
- `restricted`
- `muted`
- `banned`
- `released`

同一审核版本和处置原因使用唯一幂等键，避免“举报成立后又驳回内容”对同一次违规重复计分。中风险机器判定本身不记违规，只有人工修正、驳回、高风险阻断或举报成立才记入。

### 5.8 请求来源摘要

为紧急批量处置和重复注册风险判断，版本、举报、申诉及高风险尝试保存：

- 精确 IP 的 HMAC-SHA256 摘要。
- IPv4 `/24` 或 IPv6 `/64` 网段的 HMAC-SHA256 摘要。
- 可选的站点设备标识摘要。

摘要使用独立的生产密钥，不保存明文 IP，不复用公开可猜测的无密钥哈希。共享 IP 或网段只能触发限流、批量候选和新账号降为 `restricted`，不能单独作为永久封禁依据。

来源摘要默认保留 180 天，到期后从长期版本记录中清空；IP 网段批量处置仅覆盖该保留窗口内的新数据。历史迁移内容没有可靠来源摘要，不伪造也不反推。

## 6. 本地文本风险引擎

### 6.1 判定步骤

1. 从用户内容提取纯文本，不改变最终展示文本。
2. 执行 Unicode 规范化、繁简兼容、全半角转换、大小写归一。
3. 去除零宽字符，折叠刻意插入的分隔符和异常重复字符。
4. 执行精确关键词、受限正则和组合条件规则。
5. 取所有命中规则的最高风险级别。

判定规则：

- 明确阻断规则：`high`。
- 模糊、上下文不确定、广告、辱骂等需要人工判断的规则：`medium`。
- 未命中：`low`。

管理员保存规则时先校验和编译。无效规则拒绝保存，当前内存规则快照保持不变。规则更新成功后原子替换不可变快照。

### 6.2 降级

- 冷启动无法加载规则：发布统一降级为中风险，读取接口正常提供服务。
- 运行期间数据库暂时不可用：继续使用最后一次成功加载的规则快照。
- 不允许审核组件异常时默认低风险放行。

## 7. 用户等级与决策策略

### 7.1 等级行为

风险判定结果只描述文本风险；最终发布动作由用户等级、内容信号和全站控制共同决定。动作统一为：

- `auto_approve`：直接成为正式通过版本。
- `post_review`：先公开为待审，再由管理员处理。
- `pre_review`：保存但不公开新内容，等待人工审核。
- `block`：拒绝本次操作。

默认策略：

| 用户等级 | 干净低风险 | 含未审核图片 | 外链或低级广告信号 | 中风险 | 高风险 |
| --- | --- | --- | --- | --- | --- |
| `new` | `post_review` | `pre_review` | `pre_review` | `pre_review` | `block` |
| `normal` | `post_review` | `post_review` | `post_review` | `pre_review` | `block` |
| `trusted` | `auto_approve` | `post_review` | `pre_review` | `pre_review` | `block` |
| `restricted` | `pre_review` | `pre_review` | `pre_review` | `pre_review` | `block` |
| `banned` | `block` | `block` | `block` | `block` | `block` |

已通过图片不算“未审核图片”；可信用户仅使用全站已通过图片且文本干净时仍可自动通过。未审核图片无论用户等级如何都不能直接成为图片通过记录，至少进入先发后审或先审后发流程。

`muted_until` 未过期时按 `banned` 的交互限制处理，但不改变永久等级。封禁用户仍可登录、阅读公开内容和查看自己的处置通知，但禁止发布、编辑、举报和申诉；作者删除自己内容不受影响。

用户处置检查必须早于图片复制和预览生成；被禁言、封禁或全站关闭发布时，同时拒绝非管理员临时图片上传，避免在最终提交前消耗存储和 CPU。

### 7.2 自动升级与降级

评级器在产生用户处置事件后即时评估，并由后台任务定期复核：

- `new -> normal`：达到账号年龄和干净通过次数，且观察窗口内无违规。
- `normal -> trusted`：达到更高账号年龄、连续干净通过次数，且观察窗口内无修正、驳回、高风险或举报成立。
- 任意可发布等级达到限制分数：进入 `restricted`。
- 任意可发布等级达到封禁分数：进入临时 `banned`。
- `restricted` 到期时重新计算滚动违规分：低于限制阈值则恢复 `normal`，仍超限则续期。
- 前两次临时封禁到期后释放为 `restricted`，经过限制观察期且分数下降后才可恢复 `normal`。
- 达到配置的累计封禁次数后设置 `manual_release_only=true`，不再自动释放。

默认违规权重：管理员修正 `+1`、举报成立 `+2`、人工驳回 `+3`、高风险阻断 `+5`。干净通过用于升级条件，不直接抵消违规分，避免用户通过大量正常内容“洗掉”严重违规。

管理员可以手工调整等级、禁言、封禁或释放，并选择是否锁定等级。自动评估不得覆盖已锁定的手工决定。

等级变化默认只影响后续提交，不改写已经正式通过或正在审核的版本。管理员封禁时可选择同时隔离全部公开内容；自动封禁是否隔离历史内容由配置控制，默认不自动隔离，降低误判造成的扩大影响。

### 7.3 防止封禁绕过

- 相同 OAuth `source + UUID` 继续绑定原账号，不能创建新账号绕过。
- 已绑定且唯一的验证邮箱继续归属原账号。
- 新身份若命中被封账号的设备摘要或近期 IP 网段风险，只自动降为 `restricted` 并通知管理员，不自动永久封禁。
- 无手机号或实名信息时无法可靠识别所有重新注册行为，必须在管理端明确展示这一限制。

### 7.4 决策优先级

最终动作按以下顺序计算，前项优先于后项：

1. 管理员发布：直接通过。
2. 用户处于封禁或禁言期：阻断。
3. 全站发布模式为 `closed`：阻断非管理员发布和编辑。
4. 全站发布模式为 `pre_review_all`：把 `auto_approve`、`post_review` 强制降为 `pre_review`。
5. 用户等级策略。
6. 文本风险、图片、链接和广告信号。

## 8. 发布与编辑状态流

风险级别、用户等级、审核状态和公开展示状态彼此独立。下表描述 `normal` 用户；`new`、`trusted`、`restricted` 及全站控制按第 7 节覆盖最终动作。

| 场景 | 决策动作 | 公众展示 | 图片展示 | 返回提示 |
| --- | --- | --- | --- | --- |
| 首次发布被判为先发后审 | `post_review` | 新正文，标记待审 | 已通过图返回原图；新静态图返回模糊图；新 GIF 返回占位图 | 发布成功，内容会被审核 |
| 首次发布被判为先审后发 | `pre_review` | 不返回正文，显示风险占位 | 返回新图的模糊图或 GIF 占位图 | 内容已提交，等待人工审核 |
| 首次发布被阻断 | `block` | 无 | 清理本次临时对象 | 内容存在较高风险，未能发布 |
| 已通过内容编辑后先发后审 | `post_review` | 立即展示新正文并标记待审 | 已通过图正常，新图模糊/GIF 占位 | 修改成功，内容会被审核 |
| 已通过内容编辑后先审后发 | `pre_review` | 继续展示最后通过版本 | 继续展示最后通过版本图片 | 修改已提交，等待人工审核 |
| 已通过内容编辑被阻断 | `block` | 原版本不变 | 清理本次新临时对象 | 内容存在较高风险，原版本不受影响 |

每条内容同一时间只有一个当前待审版本。作者再次编辑待审内容时重新判定风险，新版本取代旧版本进入队列，旧版本标记为 `superseded` 并仅保留审计记录。

### 8.1 状态指针不变量

| 业务状态 | `visibility_state` | `visible_revision_id` | `approved_revision_id` | `pending_revision_id` |
| --- | --- | --- | --- | --- |
| 首次先发后审 | `visible` | 新待审版本 | `NULL` | 新待审版本 |
| 首次先审后发 | `placeholder` | `NULL` | `NULL` | 新待审版本 |
| 已正式通过 | `visible` | 通过版本 | 同一通过版本 | `NULL` |
| 已通过后先发后审编辑 | `visible` | 新待审版本 | 旧通过版本 | 新待审版本 |
| 已通过后先审后发编辑 | `visible` | 旧通过版本 | 旧通过版本 | 新待审版本 |
| 首次发布被驳回 | `hidden` | `NULL` | `NULL` | `NULL` |
| 紧急隔离 | `emergency_hidden` | 保留原指针 | 保留原指针 | 保留原指针 |

任何不符合表中不变量的组合都视为数据错误，service 拒绝继续状态转换并记录错误。`review_status` 只能从版本读取；列表查询不得用版本待审状态决定可见性。

### 8.2 业务表是公开事实源

- 原业务表中的 `content` 明确定义为当前可展示正文；待审但不可见的正文只存于审核版本。
- 碎语当前可展示图片由 `moment_media` 物化；评论、回复和留言的嵌入图片由各自可见正文及统一可见图片投影物化。
- 各业务 repository 对外提供统一 `VisibleSnapshot` 语义，包含正文、图片 key、指纹和顺序；物理表结构可按内容类型适配，图片展示模式在 DTO 映射时派生。
- 可见快照保存稳定对象 key、图片指纹和顺序，不保存预签名 URL；DTO 映射使用审核感知的解析器，未通过时只能解析为低清预览或 GIF 占位，通过后才解析原图。
- `moderation_revision` 是提交、审计和回退依据，不是普通公开列表的正文事实源。
- 新的先审后发内容在业务表保存空可见正文，公众只获得占位文案和允许展示的低清图片投影。
- 通过、修正、驳回和先发后审回退必须在同一事务中同步更新业务可见快照、版本指针和 `moderation_item`。

这样普通读取只需读取业务可见快照并检查 `visibility_state`；复杂版本选择只发生在写入和管理审核路径。

## 9. 人工审核

### 9.1 通过

- 待审版本成为最后通过版本和可见版本。
- 版本引用图片全部登记为全站已通过。
- 数据库事务提交图片通过状态后，所有同指纹引用会自动解析为原图；随后删除对应模糊预览图，失败时写入幂等清理任务重试。
- 通过站内通知告知发布者，不发送邮件。
- 未经修正通过时写入幂等 `clean_approved` 用户事件，供等级升级评估。

### 9.2 修正后通过

- 保留用户原文。
- 保存管理员修正文、修正理由、管理员 ID 和修正时间。
- 管理员可以移除风险图片；修正后的图片集合随修正文一起成为版本快照。
- 修正文成为通过版本。
- 站内通知文案说明内容经管理员修正后发布，并包含修正理由。
- 写入幂等 `corrected` 用户事件并按配置增加违规分。

### 9.3 驳回

- 首次发布：内容不再对公众展示。
- 先发后审编辑：正文和图片引用一起原子回退到最后通过版本。
- 先审后发编辑：丢弃当前待审版本，业务可见快照保持最后通过版本。
- 通过站内通知告知发布者原因。
- 写入幂等 `rejected` 用户事件并按配置增加违规分。

发布者不能直接恢复被驳回原文，可以发起申诉或重新编辑后再次提交。

## 10. 图片处理与生命周期

### 10.1 未审核图片展示

- 图片不参与低、中、高文本风险判定。
- 静态图上传时只生成一次最长边约 32–48 像素的低清预览，并存入 Garage。
- 前端可叠加模糊样式；即使移除样式也只能获取不可恢复的低清图。
- 未审核 GIF 不解码生成预览，统一返回固定 GIF 审核占位图。
- 未审核响应永远不返回原图访问地址。
- 预览生成失败时返回统一静态审核占位图，不能回退为原图。

### 10.2 全站复用

- 审核时先从现有 MD5 文件名快速查找候选，再以 SHA-256 和大小确认。
- 命中已通过记录后直接使用原图，不生成模糊图。
- 每次命中均更新 `last_used_at`。
- 同一图片可跨作者、内容类型和版本复用通过结果。

### 10.3 版本保留与删除

- 编辑提交后，最后通过版本的原图不移动、不覆盖、不删除。
- 新版本只新增自己的图片引用；从新版本移除旧图不代表立即删除对象。
- 驳回时按最后通过版本恢复正文、图片和顺序。
- 只有新版本真正通过后，才可清理旧版本中已移除的图片。
- 图片仍被其他公开、待审或可申诉版本引用时只减少引用，不删除对象。
- 数据库状态先提交，再异步删除零引用对象；删除任务必须幂等。

### 10.4 资源限制

- 完整解码前读取图片宽高并限制总像素，防止压缩炸弹。
- 图片处理使用有界并发，不按 HTTP 请求无限创建处理任务。
- 列表和详情请求只返回预生成预览 URL，不进行实时模糊处理。

## 11. 举报与申诉

### 11.1 举报

- 仅登录用户可以举报。
- 请求必须通过验证码，并按账号和 IP 限频。
- 同一用户对同一内容只能存在一条有效举报。
- 举报原因使用 `illegal`、`porn`、`violence`、`fraud`、`privacy`、`abuse`、`spam`、`minor`、`other` 枚举；补充说明最多 500 字符。
- 举报只创建高优先级复核任务并通知管理员，不自动隐藏内容。
- 作者无法看到举报者身份。
- 处理结果通过站内消息通知举报者和发布者。

管理员可以维持原决定、修正后通过或驳回下架。

举报被判定成立时写入幂等 `report_upheld` 用户事件；举报本身、重复举报和不成立举报均不增加被举报用户违规分。

### 11.2 申诉

- 申诉针对具体被驳回版本，而不是整个内容的永久累计次数。
- 每个被驳回版本最多申诉 3 次。
- 申诉期间维持当前展示状态。
- 管理员可以维持驳回、通过原文或修正后通过。
- 待审期间作者正常编辑不会消耗申诉次数。

## 12. 紧急控制与批量处置

### 12.1 全站控制

`moderation_control` 保存以下运行状态：

- `registration_mode`：`open`、`closed`。
- `publishing_mode`：`open`、`pre_review_all`、`closed`。
- `reason`、`operator_id`、`changed_at`、可选 `expires_at`。

数据库是事实源，Redis 只缓存短 TTL 副本；缓存失效或读取失败时回源数据库。到达 `expires_at` 后后台任务恢复上一稳定模式并留操作日志。管理员发布、用户删除自己内容和已有内容读取不受发布总开关影响。

`registration_mode=closed` 同时阻止邮箱注册和 OAuth 回调中的自动建号，但不影响已有用户登录、刷新令牌或绑定新的 OAuth 身份。

### 12.2 用户级止血

管理端支持：

- 临时禁言并指定截止时间和理由。
- 设置 `restricted`、`banned` 或手工恢复等级。
- 一键将某用户全部公开内容切换为 `emergency_hidden`。
- 恢复该批被隔离内容原有的展示状态。
- 封禁用户时可选择是否同时隔离其全部公开内容。

批量隔离只改变公开状态并保存原状态快照，不直接软删除或删除图片，确保误操作可恢复。

### 12.3 IP 网段批量处置

IP 网段处置采用两阶段流程：

1. `preview`：按 IPv4 `/24`、IPv6 `/64` 摘要、时间范围和内容类型返回命中数量及有限样例。
2. `execute`：携带预览生成的一次性确认令牌，将命中内容批量隔离。

默认动作是可恢复的 `quarantine`，不是不可逆“清空”。永久删除必须另行提交 `purge` 任务，要求再次确认、记录理由并经过配置的隔离保留期。所有批任务分批执行、可续跑、可审计，单批失败不影响已完成批次。

`purge` 必须复用各内容类型的永久删除和关联资源清理语义，处理点赞、回复、通知、媒体和对象引用；禁止用跨表原始 SQL 直接清空内容行。

### 12.4 紧急处置优先级

- 全站 `closed` 高于用户信任等级。
- `pre_review_all` 高于 `trusted` 的自动通过。
- 用户禁言或封禁高于单条内容风险。
- `emergency_hidden` 高于普通 `visible`，但不修改版本审核结果和回退指针。

## 13. HTTP 接口

### 13.1 用户接口

- `GET /moderation/me`：返回自己的等级、禁言/封禁期限及当前允许的操作，不返回内部评分阈值。
- `POST /moderation/reports`
- `POST /moderation/appeals`

原有内容接口保持路径不变，响应增加统一审核摘要：

```json
{
  "moderation": {
    "visibility_state": "visible",
    "display_version": "pending",
    "has_pending_revision": true,
    "pending_risk_level": "low"
  }
}
```

图片响应增加 `display_mode`：`original`、`blurred` 或 `gif_placeholder`。前端只按服务端字段渲染，不自行推断审核状态。

公开响应只提供展示状态和派生字段：`visibility_state` 决定是否展示，`display_version` 明确正文来源，`has_pending_revision` 表示是否存在待审版本。先发后审编辑时 `display_version=pending`，先审后发编辑时为 `last_approved`。公开响应不得包含先审后发的待审正文、命中规则或驳回原因。

作者和管理员可额外获得 `pending_revision.review_status`、`pending_revision.risk_level`、自己的待审正文、可见处理理由，以及被驳回版本的 `appeal_count` 和固定上限 3。API 不再返回语义含糊的顶层审核 `status`。

其他用户的信任等级、违规分、来源摘要和处置历史均不进入公开用户 DTO 或 Swagger 示例。

### 13.2 管理接口

- `GET /admin/moderation/items`
- `GET /admin/moderation/items/:id`
- `POST /admin/moderation/items/:id/approve`
- `POST /admin/moderation/items/:id/correct`
- `POST /admin/moderation/items/:id/reject`
- `GET /admin/moderation/reports`
- `POST /admin/moderation/reports/:id/resolve`
- `GET /admin/moderation/appeals`
- `POST /admin/moderation/appeals/:id/approve`
- `POST /admin/moderation/appeals/:id/correct`
- `POST /admin/moderation/appeals/:id/reject`
- `GET /admin/moderation/rules`
- `POST /admin/moderation/rules`
- `PATCH /admin/moderation/rules/:id`
- `DELETE /admin/moderation/rules/:id`
- `GET /admin/moderation/users/:id`
- `PATCH /admin/moderation/users/:id/profile`
- `POST /admin/moderation/users/:id/mute`
- `POST /admin/moderation/users/:id/ban`
- `POST /admin/moderation/users/:id/release`
- `POST /admin/moderation/users/:id/hide-content`
- `POST /admin/moderation/users/:id/restore-content`
- `GET /admin/moderation/control`
- `PATCH /admin/moderation/control`
- `POST /admin/moderation/bulk-actions/preview`
- `POST /admin/moderation/bulk-actions/execute`
- `GET /admin/moderation/bulk-actions/:id`

审核动作必须携带待审版本 ID。版本不再是当前版本时返回 `409 Conflict`，防止管理员审核旧版本覆盖作者的新编辑。

禁言、封禁、全站关闭和强制先审后发均返回稳定业务错误码或能力字段，前端不得仅凭按钮隐藏代替后端权限校验。

### 13.3 高风险错误

高风险统一返回 HTTP `422 Unprocessable Entity` 和业务错误码 `CONTENT_RISK_REJECTED`，提示：

> 内容存在较高风险，未能发布，请修改后重试。

编辑场景额外说明原已通过版本不受影响。响应不返回命中词、规则 ID、匹配位置或具体风险标签。

## 14. 通知、事务与幂等

- 业务可见正文、可见图片投影、审核版本、版本指针和用户处置事件在同一事务内写入。
- 用户事件使用唯一幂等键；事务内锁定对应 `user_moderation_profile` 行并更新投影，防止并发违规丢分或重复计分。
- 用户档案和全站控制缓存只用于加速读取，数据库更新提交后必须主动失效缓存。
- 审核状态先提交，再发布现有通知系统的站内事件。
- `auto_approve` 和 `post_review` 内容可在首次公开时通知目标用户；`pre_review` 只通知作者已提交，待通过后再通知目标用户，驳回时不向目标用户暴露该内容。
- 用户等级变化、禁言、封禁、释放和管理员手工校正均发送站内通知，包含原因和期限。
- 不产生任何审核邮件任务。
- 审核动作、通知、图片预览删除和旧图片清理均使用稳定幂等键。
- 通知或对象删除失败不回滚已提交的审核结果，由后台任务重试。
- 数据库事务失败时不得产生可见内容，并清理本次新增对象。
- 高风险审计写入失败时仍必须拒绝内容，并使用 `zap.Logger` 记录异常。

## 15. 配置默认值

```yaml
moderation:
  # 是否启用用户内容审核。关闭时仅建议用于本地排障，生产环境应保持开启。
  enabled: true
  # observe 仅记录判定；enforce 执行拦截和待审展示。
  mode: observe
  # 高风险阻断尝试和审核操作日志默认保留天数。
  audit_log_retention_days: 180
  # 静态图片完整解码前允许的最大总像素数，防止压缩炸弹。
  image_max_pixels: 12000000
  # 同时执行图片低清预览处理的最大任务数。
  image_processing_concurrency: 2
  # 低清预览最长边像素；前端可在此基础上增加模糊样式。
  image_preview_max_edge: 48
  # 已通过图片连续未参与审核判断后的缓存记录保留天数。
  image_approval_retention_days: 180
  # 图片审核缓存记录清理周期。
  image_approval_cleanup_interval: 24h
  # 单次最多清理的图片审核缓存记录数，避免长事务。
  image_approval_cleanup_batch_size: 500
  # 单个账号每小时最多提交的举报数。
  report_account_hourly_limit: 10
  # 单个 IP 每小时最多提交的举报数。
  report_ip_hourly_limit: 30
  # 请求来源 HMAC 密钥；生产环境必须通过 BLOG_MODERATION_SOURCE_HMAC_SECRET 注入。
  source_hmac_secret: ""
  # 请求来源摘要保留天数，到期只清空摘要，不删除内容和审核记录。
  source_hash_retention_days: 180
  # 用户等级后台复核周期。
  profile_evaluation_interval: 1h
  # 自动评级只统计最近窗口内的违规事件。
  profile_evaluation_window_days: 90
  # 是否允许自动晋级、限制和封禁，可分别关闭以退化为纯人工调整。
  auto_promotion_enabled: true
  auto_restriction_enabled: true
  auto_ban_enabled: true
  # 新用户晋级普通用户的最小账号年龄和干净通过次数。
  new_to_normal_min_age_days: 7
  new_to_normal_clean_approvals: 3
  # 普通用户晋级可信用户的最小账号年龄和连续干净通过次数。
  normal_to_trusted_min_age_days: 30
  normal_to_trusted_clean_approvals: 20
  # 滚动窗口内达到对应违规分时自动限制或封禁。
  restricted_score_threshold: 5
  banned_score_threshold: 10
  # 自动限制默认持续 7 天；到期后重新计算，分数仍超限则续期。
  restricted_duration: 168h
  # 前两次封禁分别持续 7 天和 30 天。
  ban_durations:
    - 168h
    - 720h
  # 达到第 3 次封禁后不再自动释放。
  permanent_after_ban_count: 3
  # 自动封禁时是否同步隔离该用户历史公开内容；默认关闭以降低误判影响。
  hide_content_on_auto_ban: false
  # 违规事件权重；调整后只影响后续评估，不重写历史事件。
  violation_weights:
    corrected: 1
    report_upheld: 2
    rejected: 3
    high_risk_blocked: 5
  # IP 批量隔离后允许永久清理的最短等待天数。
  bulk_quarantine_retention_days: 7
```

生产正式启用前将 `mode` 从 `observe` 切换为 `enforce`。配置结构、默认值和“删除记录不等于删除仍被引用原图”等边界必须写中文注释。

## 16. 历史数据迁移与上线

实现和上线拆为以下顺序，前一阶段验证通过后再进入下一阶段：

1. 建立审核表、无歧义版本指针、业务可见快照事务和规则观察模式。
2. 接入评论、回复、留言、碎语的发布/编辑状态流及图片预览、回退、全站复用。
3. 接入管理审核、站内通知、举报和申诉。
4. 建立用户档案、事件计分、等级策略、自动限制/封禁和注册链路档案创建。
5. 建立全站控制、用户批量隔离及 IP 网段预览/隔离任务。
6. 以可重复执行的批处理回填历史内容、图片和用户档案。
7. 历史内容统一建立 `approved / legacy_migration` 版本。
8. 历史图片分批补算 SHA-256 和大小，登记为全站已通过图片。
9. 全部历史用户建立 `trusted / legacy_migration` 档案，并按已有内容回填通过计数；历史可信等级不会因缺少旧事件而自动降级。
10. 初始化全站控制为 `registration_mode=open`、`publishing_mode=open`。
11. 启用 `observe` 模式运行 3–7 天，统计规则命中、用户评分和策略决策但不执行自动封禁。
12. 切换为 `enforce`，启用三级展示、用户等级、举报、申诉和紧急控制。

迁移必须支持断点续跑和重复执行，不修改历史业务 ID、对象 key 或现有 URL。

## 17. 测试与验证

### 17.1 规则引擎

- Unicode、繁简、全半角、零宽字符、分隔符和重复字符归一化。
- 精确、正则、组合规则的优先级和最高风险合并。
- 无效规则不替换当前快照。
- 冷启动加载失败降级为中风险。

### 17.2 状态机

- 首次发布和已通过内容编辑的低、中、高风险矩阵。
- `visibility_state` 与三个版本指针的全部合法组合和非法组合拒绝。
- `auto_approve`、`post_review`、`pre_review`、`block` 四种动作。
- 先发后审编辑驳回后正文与图片完整回退。
- 先审后发编辑始终展示最后通过版本。
- 待审再编辑使旧版本失效。
- 管理员修正保留原文、理由、管理员和时间。
- 过期版本审核返回冲突。

### 17.3 用户等级与处置

- 新建用户档案与邮箱注册、OAuth 注册保持事务一致。
- 历史用户迁移为可信用户且重复执行不重复累计。
- 各等级对纯文本、图片、链接、广告信号及三档风险的策略矩阵。
- 违规事件幂等计分，举报成立与内容驳回不会重复计同一违规。
- 自动晋级、限制、封禁、到期释放和第三次封禁转人工释放。
- 管理员手工等级锁阻止自动任务覆盖。
- 被封用户仍可登录读取通知，但不能发布、举报和申诉。
- 缺失档案时按新用户降级，不按可信用户放行。

### 17.4 图片

- MD5 候选与 SHA-256、大小的最终校验。
- 全站已通过图片复用并更新 `last_used_at`。
- 静态图低清预览、GIF 占位和处理失败占位。
- 图片通过后删除模糊图，失败任务可重试。
- 旧图片在新版本通过前不会删除。
- 引用计数为零后才清理对象。
- 180 天清理跳过仍被有效版本引用的记录。
- 像素上限、并发限制及关键处理基准测试。

### 17.5 举报、申诉与通知

- 举报登录、验证码、账号/IP 限频和唯一有效举报约束。
- 举报不自动隐藏内容。
- 每版本最多三次申诉。
- 通知仅进入站内收件箱，不创建邮件任务。
- 重复审核请求不产生重复通知和重复清理。

### 17.6 紧急控制与批量任务

- 全站开放、强制先审后发和关闭发布的优先级。
- 关闭注册同时覆盖邮箱注册和 OAuth 自动建号。
- 用户全部内容隔离与恢复保持原状态快照。
- IP 网段预览令牌一次性、过期和条件绑定校验。
- 批任务断点续跑、重复执行幂等和部分失败恢复。
- 永久清理必须满足隔离保留期，不允许直接绕过。

### 17.7 分层验证

- Repository：使用 `go-sqlmock` 验证事务、查询和迁移幂等。
- Service：使用 `gomock` 覆盖状态流、权限和异常降级。
- Handler：使用 `httptest` 与 `testify` 验证绑定、鉴权、响应脱敏和错误码。
- 完成后运行相关包测试、`go test ./...`、`go vet ./...` 和 Swagger 更新验证。

## 18. 监控与运维

至少记录以下指标或结构化日志：

- 低、中、高风险判定数量。
- 待审队列长度和最老等待时长。
- 人工通过、修正、驳回和申诉结果数量。
- 举报数量、重复举报和限频拒绝数量。
- 图片预览耗时、处理失败、像素限制拒绝数量。
- 模糊图删除、旧图清理和通知重试失败数量。
- 图片审核缓存命中率及定期清理数量。
- 各用户等级数量、等级变化原因、禁言、限制、封禁和自动释放数量。
- 全站控制模式变化、批量隔离命中数、进度、失败和恢复数量。
- 按来源网段摘要聚合的异常注册与发布趋势，不输出原始 IP。

所有日志使用注入的 `zap.Logger`，不记录完整高风险正文、图片二进制、认证信息或具体敏感规则内容。

## 19. 已知风险

- 本地规则不能完整理解上下文，必然存在误判和漏判，需要持续维护规则与人工队列。
- 低风险正文会在人工通过前公开，存在短时间传播风险。
- 图片没有机器内容识别，只通过低清预览、GIF 占位和人工审核降低公开风险。
- 注册用户不要求手机号、身份证或已验证邮箱，真实身份认证要求尚未满足。
- 举报不自动隐藏内容，管理员需要及时处理高优先级复核任务。
- 自动评级依赖可配置阈值，配置过严会误伤用户，配置过松会失去处置效果；上线观察期必须先记录决策再启用自动封禁。
- IP 网段和设备摘要存在共享、变化和伪造问题，只能用于风险降级与批量候选，不能证明两个账号属于同一自然人。
- 紧急批量处置影响范围大，必须使用预览、一次性确认令牌、可恢复隔离和完整操作日志降低误操作风险。
- 无实名条件下可以通过养号获取可信等级；账号年龄、连续干净审核和可配置阈值只能提高成本，不能彻底阻止协同养号。
