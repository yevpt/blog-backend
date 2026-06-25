# 自建站点分析 — 交接文档（给下一个 agent）

> 本文档让你**冷启动**接手「自建站点分析（类 GA/百度统计）」的实现。读完即可继续，不需要原会话上下文。

## 0. 一分钟速览

- **设计 spec**：[docs/superpowers/specs/2026-06-25-self-hosted-analytics-design.md](specs/2026-06-25-self-hosted-analytics-design.md)（已定稿，含注册/匿名细分、BFF 身份中转、隐私口径）。
- **Phase 1 后端计划**：[docs/superpowers/plans/2026-06-25-analytics-phase1-backend.md](plans/2026-06-25-analytics-phase1-backend.md)（16 个任务）。
- **进度账本**：`.superpowers/sdd/progress.md`（含每个任务的 commit 区间与遗留 Minor）。
- **工作分支**：`feat/analytics-phase1`（从 `dev` 切出，baseline `c5ccddd`）。
- **执行方法**：superpowers `subagent-driven-development`（每任务派 implementer + reviewer，脚本在 `~/.claude/plugins/cache/claude-plugins-official/superpowers/6.0.3/skills/subagent-driven-development/scripts/`：`task-brief`、`review-package`）。
- **当前进度**：Phase 1 后端 **Task 1–15 全部完成并通过评审**，仅剩 **Task 16（配置 + 部署接线）** + 最终整分支评审 + 收尾。

### ⚠️ 环境坑（务必先知道）
派发 subagent 时，`model: sonnet` 和 `model: haiku` 在本环境会报错（解析到一个不可用的 Qwen 模型）。**只有 `model: opus` 和 `fable` 可用**。所有 implementer/reviewer 都用 `opus`。

---

## 1. 立即要做：Task 16（Phase 1 后端最后一个任务）

提取简报：`scripts/task-brief docs/superpowers/plans/2026-06-25-analytics-phase1-backend.md 16`

**目标**：把目前散落在代码里的 4 处临时字面量迁移到 `cfg.Analytics` 配置块，并补全部署接线。

**待迁移点（已用 `TODO(Task 16)` 标注）**：
- `internal/router/router.go:260` 和 `:294`（`newAnalyticsCollectHandler` 内的时区/在线窗口/channel 缓冲/批量/siteHost/ipSalt/geoIPPath 字面量）
- `internal/bootstrap/bootstrap.go:144`（retentionDays/时区/onlineWindow）
- `internal/worker/analytics/scheduler.go:12`

**做法**：
1. 在 `pkg/config/config.go` 的 `Config` 结构体加 `Analytics AnalyticsConfig \`mapstructure:"analytics"\``，定义字段（mapstructure tag）：`timezone`、`retention_days`、`online_window`(Duration)、`session_timeout`、`bounce_duration`、`channel_buffer`、`public_cache_ttl`、`geoip_path`、`site_host`、`ip_salt`。仿现有 `EmailConfig` 等写法。
2. `config/config.yaml`（及 `config.prod.yaml` / `config.local.yaml.example` / `config.test.yaml`）加 `analytics:` 默认块（见 spec 末尾「配置项」表：timezone=Asia/Shanghai、retention_days=90、online_window=90s、session_timeout=30m、bounce_duration=10s、channel_buffer=4096、public_cache_ttl=60s、geoip_path=""、site_host=yevpt.com、ip_salt=change_me）。
3. **把 `cfg` 透传进 `newAnalyticsCollectHandler`**（`router.Setup` 已经有 `cfg` 参数，目前没往下传）。用 `cfg.Analytics.*` 替换 4 处字面量。注意 `time.LoadLocation(cfg.Analytics.Timezone)` 的 error 要处理（失败回退 `FixedZone("CST", 8*3600)`，和 Task 14 `dayRangeUTC` 的兜底一致）。
4. 敏感值仍走 env 覆盖（项目用 `BLOG_` 前缀 viper 自动 env）：`ip_salt`→`BLOG_ANALYTICS_IP_SALT`、`geoip_path`→`BLOG_ANALYTICS_GEOIP_PATH`。`ANALYTICS_ALLOWED_ORIGINS` **保持 `os.Getenv` 直读**（与现有 CORS env 风格一致，不进 viper）。
5. `docker-compose.yml` 的 `blog-server.environment` 增 `ANALYTICS_ALLOWED_ORIGINS: ${ANALYTICS_ALLOWED_ORIGINS:-}`、`BLOG_ANALYTICS_GEOIP_PATH: ${ANALYTICS_GEOIP_PATH:-}`、`BLOG_ANALYTICS_IP_SALT: ${ANALYTICS_IP_SALT:-}`；`volumes` 增 `- ./geoip:/app/geoip:ro`（放 `ip2region.xdb`）。`.env.example` 补这 3 个变量（`ANALYTICS_ALLOWED_ORIGINS=https://www.yevpt.com,https://yevpt.com` 等）。
6. `go build ./... && go test ./...`，commit：`chore(analytics): 新增统计配置项与部署接线`。

---

## 2. Task 16 完成后：最终整分支评审 + 收尾

1. **最终整分支评审**（用最强模型 `opus`）：
   `scripts/review-package $(git merge-base dev HEAD) HEAD` → 把打印的路径喂给 `superpowers:requesting-code-review` 的 `code-reviewer.md` 模板派一个 reviewer。把下面第 3 节的「遗留风险清单」一并给它，让它判定哪些必须合并前修。
2. 评审若有 Critical/Important → 派**一个** fix subagent（带完整 findings 列表，别一个 finding 一个 subagent）。
3. 跑全量：`go build ./... && go test ./... && make swag`。
4. **部署前手动验证**（沙箱里没真 DB，必须在目标环境做）：
   - 跑 `cmd/dbsetup` 的 AutoMigrate，**确认 5 张 analytics 表建表成功**（特别是 `analytics_page_daily` 的 `varchar(512)` 复合主键，见风险 R1）。
   - 放置 `ip2region.xdb` 文件并设 `BLOG_ANALYTICS_GEOIP_PATH`；不设则地理解析优雅降级为空。
   - 设 `ANALYTICS_ALLOWED_ORIGINS=https://www.yevpt.com,https://yevpt.com`、`BLOG_ANALYTICS_IP_SALT=<随机串>`。
5. `superpowers:finishing-a-development-branch` 决定合并/PR。

---

## 3. 遗留风险 / 坑点清单（按严重度）

> 这些来自逐任务评审的 Minor + 延后项。**带 [Phase 2 必修] 的是真正影响正确性、必须在 Phase 2 解决的**。

### 影响正确性（须重视）
- **R1 [部署前验证]** `AnalyticsPageDaily.Path varchar(512)` 是复合主键的一部分。utf8mb4 下 512 字符=2048 字节。MySQL 8（InnoDB DYNAMIC + large prefix，上限 3072 字节）能建表；**legacy ≤5.6 / REDUNDANT 行格式会建表失败**。AutoMigrate 首次跑时务必确认。若失败：把 PK 改为 `(date, path_hash)`（path md5）并把 path 降为普通列，同步改 Task 9 `UpsertPageDaily` 的 OnConflict 列和 Task 14 聚合写入。
- **R2 [Phase 2 必修] 会话 pv_count 永远=1 → bounce_rate 全为"跳出"**。根因：`internal/service/analytics/collect.go` 的 `sessionFrom` 每事件发 `PVCount=1`，而 Task 9 `repository.go` 的 `UpsertSession` 的 `DoUpdates` 把 `pv_count` 设为 excluded 值（覆盖而非自增）。Phase 1 的日 PV/UV 来自 `analytics_events`（不受影响），但会话级 pv_count / bounce 不准。**Phase 2 修法**：`UpsertSession` 改为 `pv_count = pv_count + 1`（`clause.Assignments` 表达式），并据此重算 `is_bounce`、累计 `exit_path`/`duration`。
- **R3 [Phase 2 必修] `avg_duration` / `bounce_rate` 当前恒为 0**。`internal/service/analytics/query.go` 的 `AggregateDay` 用 `TODO(Phase2)` 占位。Phase 2 要从 `analytics_sessions`（当天）聚合出 avg_duration 和 bounce_rate（依赖 R2 先修对 pv_count/duration）。

### 健壮性 / 可维护性（建议修，不阻塞）
- **R4** UTF-8 字节截断：`SanitizePath`（`sanitize.go` `path[:512]`）和 `enrich.go`（`title[:255]`）按字节切，可能切断多字节 rune 产生非法 UTF-8。若入库走 UTF-8 严格校验会出问题。改用 `[]rune` 截断。
- **R5** `AggregateDay` 的 `SUM(is_authenticated)` / `SUM(NOT is_authenticated)` 依赖 MySQL 的 bool→0/1 语义，换 Postgres 会坏。加注释标明 MySQL-only。
- **R6** `AggregateDay` 的聚合 SQL 表达式**未被测试断言**（sqlmock 用宽松的 `FROM analytics_events` 前缀匹配）。一个把 `visitor_id`↔`user_id` 写反的回归不会被测出。建议补一个用 `sqlmock.QueryMatcherRegexp` 断言 `COUNT(DISTINCT CASE WHEN ...)` 关键子串的测试。
- **R7** 调度器漏天：`internal/worker/analytics/scheduler.go` 的 `lastRolled` 是单槽内存值。进程跨**两个**午夜停机会漏聚合中间那一天，无自动补。可恢复（`RollupDay(date)` 支持任意日期回填），但建议加一个启动时 catch-up 或运维命令（Phase 2/3）。
- **R8** geoip happy-path 解析未测（需真 `.xdb` fixture）；ip2region 依赖是 **pseudo-version**（`go get -u` 可能再次改变 API，当前已适配到 v3.16.0 IPv6 API：`NewWithBuffer(version, buf)` + `Search`）；`ip2regionGeo.logger` 字段未使用。
- **R9** 调度器 `tick` 时间门控逻辑（after-00:30 / yesterday / lease 分支）无单测（无时钟抽象）。`RollupDay`/`Cleanup` 单元已测。
- **R10** 无整体优雅关停：worker 用 `context.Background()`（与现有 notification worker 一致）。ingestor 的**最终 flush 已修**为 detached ctx（`context.WithoutCancel`+5s timeout），关停时最后一批不会因 ctx 取消而丢。

### 行为差异 / 决议记录（知道即可）
- **R11** `DetectBot`（`botfilter.go`）已**改为先查 UA 黑名单、再看 device 标志**（偏离 Task 5 简报原定的"device 优先"）。原因：UA 库把 `Googlebot/2.1` 解析为 device=bot，但期望 `bot_reason="ua_blacklist"`（更具体）。Task 5 全部 4 个用例仍绿、`is_bot` 判定不变，只是审计标签更准。已被两次独立评审接受。
- **R12** `ClassifyReferer` 用 `strings.Contains` 做 host 匹配，偏宽：`"x.com"`/`"so.com"` 等裸条目可能误伤（如 `matrix.com`→social）；`(?i)bot` 正则会误伤如 `Cubot` 机型 UA。对"分析分桶"场景保守可接受（宁可多算 bot）。
- **R13** 响应约定：项目 `pkg/response` **没有 422**，校验失败统一用 `response.Fail(c, response.CodeBadRequest, ...)`（HTTP 200、envelope code=400）。这是项目既有惯例（见 `internal/handler/article/article.go`），admin handler 已遵循，**别自造 422**。

### 测试覆盖缺口（择机补）
- tablet/bot 设备分支（Task 3）、6/8 repo 写方法仅编译覆盖（Task 9）、空白名单分支（Task 12）、bot 不刷在线未显式断言（Task 11）、Task 16 配置加载。

---

## 4. 关键架构事实（接手前必须知道）

- **共享 ingestor（最易错）**：全代码只构造**一个** `Ingestor`（`router.go` 的 `newAnalyticsCollectHandler` 内）。它经 `AnalyticsRuntime` 从 `router.Setup` 透出 → `main.go` 传给 `bootstrap.StartAnalyticsWorker`（只调一次）→ `go ingestor.Run(ctx)` drain 的是**同一实例**。collect handler 是生产者、worker 是消费者。**任何改动都不能再 new 第二个 ingestor**，否则事件提交到一个、Run 另一个 → 全部 buffer 满后丢弃。
- **`DayAggregate` 在 repo 包**（`internal/repository/analytics`），worker 包 import 它（避免循环依赖）。worker 的 `RollupReader`/`RollupWriter` 是结构化接口，`repo.Repository` 自动满足。
- **身份识别**：`/collect` 挂 `OptionalAuth`；用 `jwt.GetClaims(c)`（返回 `*jwt.Claims` 或 nil）取 `Claims.UserId int64` → 转 `*uint`。client 永不自报 user_id。
- **Origin 反伪造**：handler 算 `OriginAllowed`（空白名单=放行=开发），塞进 `RawEvent.OriginAllowed`；`enrich` 映射 `IsSuspect = !OriginAllowed`。聚合期 `is_suspect=0` 过滤。
- **三档计数**：Redis 今日计数 key `analytics:pv:{yyyymmdd}` / `analytics:uv:{yyyymmdd}`（+`:registered`/`:anonymous`）。identity = `u:<userid>`（登录）或 `visitorID`（匿名）。聚合 UV 去重：全部=`COALESCE(user_id,visitor_id)`、注册=`user_id`、匿名=`visitor_id`。
- **`/collect` 契约**：`POST /collect`，body `{event_type(page_view|heartbeat), path, title, referer, session_id, screen}`，UA/IP/Origin 由后端取，**永远返回 204**（含坏输入，不漏细节）。
- **聚合调度**：每分钟 tick，Asia/Shanghai 过 00:30 后用 Redis 租约 `SET analytics:rollup:lock:<date> NX EX 2h` 对**昨天** `RollupDay`（幂等 upsert，可对任意日期回填）。每日清理过期原始数据 + trim 在线 ZSET。

---

## 5. Phase 1 前端（下一份要写的计划）

> 在前端仓 `/Volumes/External/SynologyDrive/Codes/Blog/blog-frontend`（pnpm workspace；`apps/web` = Next.js App Router SSR = 被监控前台；`apps/admin` = React CSR = 不监控；`packages/*` 共享）。测试用 vitest。**先 brainstorm 不需要**（设计已在 spec 定稿），但要按 `superpowers:writing-plans` 写一份独立计划再执行。

要交付 3 块：

1. **`packages/tracker`（新建共享包）— tracker SDK**
   - PV 上报：监听 App Router 路由变化（`usePathname`），**绑真实导航 + 页面可见，绝不绑组件挂载**（否则 Next `<Link>` 预取会造幽灵 PV，见 spec）。
   - 心跳：页面可见时每 ~15s 发 `heartbeat`；`visibilitychange` 隐藏即停；`pagehide`/隐藏时补发一次。
   - `session_id`：前端生成存 `sessionStorage`，30min 无心跳算新会话。**不发 user_id**。
   - 传输：`fetch(url, { keepalive: true, credentials: 'include' })` 发到 **web BFF `/api/collect`**（同源，HttpOnly cookie 自动带）。
   - 载荷：`{event_type, path, title, referer, session_id, screen}`。
   - 单测：路由变化触发一次 PV、预取不触发、心跳节流、隐藏停发。

2. **`apps/web/app/api/collect/route.ts`（新建）— BFF 中转（方案 A）**
   - 读服务端 HttpOnly cookie 解析登录态；登录则给转发请求加 `Authorization: Bearer <token>`，匿名则不加。
   - 转发到 Go 后端 `POST /collect`。
   - 透传 `visitor_id` cookie，并把后端的 `Set-Cookie`（首次下发 visitor_id）回写浏览器；透传 `Origin`、`X-Forwarded-For`。
   - 薄代理，无业务逻辑。注意 keepalive/beacon 请求体大小限制（够用）。

3. **接入**：在 `apps/web` 根 layout 注入 tracker（仅 web，admin 不注入）。确认生产 `ANALYTICS_ALLOWED_ORIGINS` 含 web 域名、不含 admin 域名。

**前置确认**：web 的 JWT 锁在 BFF 的服务端 HttpOnly cookie，浏览器读不到——所以必须走 BFF 中转（已定方案 A）。

---

## 6. Phase 2（会话指标精化 + 维度接口 + 公开展示）

> 写独立计划再执行。

1. **修 R2 + R3（会话指标）**：`UpsertSession` 改 `pv_count = pv_count + 1` 并重算 `is_bounce`/`duration`/`exit_path`；`AggregateDay` 从 `analytics_sessions` 算出当天 `avg_duration`/`bounce_rate`，填进 `analytics_daily`。补对应测试。
2. **维度接口**：`GET /admin/analytics/dimensions?dimension=referer_type|device|browser|os|country|user_type&from&to`。repo 的 `QueryDimRange` 已存在，只需加 handler + DTO + 路由 + swagger。
3. **前台公开 API**（无需登录）：
   - `GET /analytics/public/summary`（站点累计/今日访问/在线 + 注册vs匿名人数聚合）
   - `GET /analytics/public/popular?limit`（热门文章/页面榜，小 limit）
   - 硬规矩：**只出聚合数字**（不暴露单访客/user_id/IP/`country` 以下/`/admin/*` 路径）；结果进 Redis 短 TTL 缓存（`public_cache_ttl`，已在配置）；`RateLimitNormal` 兜底；聚合时排除 `/admin/*` 路径。
4. **回填工具**：一个命令或 admin 接口，对指定日期区间循环 `RollupDay`（已支持），用于改了 bot 规则后重刷历史、或补 R7 漏掉的天。

---

## 7. Phase 3（进阶）

> 写独立计划再执行。

1. **headless 高级爬虫识别**：tracker 端采集 `navigator.webdriver`、缺失鼠标/滚动交互等信号，作为 `suspect` 提示发给后端；后端富化时参考（不直接判死）。
2. **访问路径 / 漏斗分析**：基于 `analytics_events` 按 `session_id` 拼接路径序列（原始表保留期内可分析）。可能需要新的聚合表或按需查询。
3. **`/collect` 签名 token 反伪造**：BFF 在 SSR 时（已知用户）签一个短期 HMAC 身份令牌，tracker 带上，后端验签——把伪造门槛抬高。需 BFF 与后端共享密钥 + 令牌刷新。当前仅靠 Origin 校验 + 限频 + bot 过滤。

---

## 8. 文件/产物索引（已落地）

- 后端新增包：`internal/service/analytics/`（referer/useragent/sanitize/botfilter/geoip/enrich/realtime/collect/dedup/query/types）、`internal/repository/analytics/`、`internal/worker/analytics/`（ingest/rollup/scheduler/session_adapter）、`internal/handler/analytics/`（collect/admin）、`internal/dto/analytics/`、`internal/model/analytics.go`。
- 改动：`internal/dbschema/schema.go`（迁移注册）、`internal/router/router.go`（装配+路由+AnalyticsRuntime）、`internal/bootstrap/bootstrap.go`（StartAnalyticsWorker）、`cmd/server/main.go`、`go.mod/go.sum`（+mileusna/useragent、+lionsoul2014/ip2region）。
- 依赖：UA 解析 `github.com/mileusna/useragent`；IP→地理 `github.com/lionsoul2014/ip2region/binding/golang/xdb`（需 `ip2region.xdb` 文件，缺则降级）。
- SDD 工件：`.superpowers/sdd/`（progress.md 账本、各 task-N-brief.md / task-N-report.md / review-*.diff）。**`.superpowers/sdd/` 是 git-ignored scratch，`git clean -fdx` 会清掉账本——丢了从 `git log` 重建。**

## 9. 继续 SDD 的操作要点

- 接手先 `cat .superpowers/sdd/progress.md`：里面标 complete 的任务**别重做**，从第一个未完成的（Task 16）开始。
- 每任务：`task-brief PLAN N` 提简报 → 派 implementer（opus，给简报路径 + 跨任务修正 + 报告路径）→ implementer 回 DONE → `review-package <上一个HEAD> HEAD` → 派 reviewer（opus，给简报+报告+diff+global constraints）→ 通过则 append 一行到账本。
- 模板在 skill 目录：`implementer-prompt.md` / `task-reviewer-prompt.md`。
