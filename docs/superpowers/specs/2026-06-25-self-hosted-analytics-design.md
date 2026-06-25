# 自建站点分析系统设计（Self-Hosted Analytics）

## 目标

为博客自建一套类 Google Analytics / 百度统计的监控系统，数据自主可控，同时服务两个场景：

- **后台管理**：查看 PV/UV 趋势、热门页面排行、来源/设备/地理等维度分布、实时在线。
- **前台公开展示**：展示脱敏后的聚合数据（站点累计访问、今日访问、当前在线、热门文章）。

采集口径为「完整档」：站点 PV/UV、来源分类、设备/浏览器/OS、地理、新老访客、会话/停留时长、跳出率、访问路径、实时在线人数。

数据需按**全部 / 注册用户 / 匿名用户**三档细分，用于后台分析与前台展示。

## 范围与约束

- 被监控对象：前台 `web`（Next.js App Router，SSR 首屏 + 客户端路由），域名 `https://www.yevpt.com` / `https://yevpt.com`。
- `admin`（React CSR，`https://admin.yevpt.com`）**不被监控**、不上报。
- 技术栈沿用现状：Gin、GORM/MySQL、go-redis、zap、viper；定时任务沿用进程内 worker（ticker + MySQL 租约）模式。
- 复用已有基础设施：`middleware.VisitorID`（visitor_id Cookie）、`service/uv`（Redis 去重思路）、CORS/RateLimit 中间件、`internal/worker/notification` worker 模式。
- 全局红线：禁全局变量持有基础设施、生产禁 `fmt.Println`、禁直接返回 `model.*` 给前端。

## 存储策略（方案 C：原始短存 + 永久聚合 + Redis 实时）

- 原始事件**短期保留**（默认 90 天，可配），支撑路径/会话/跳出等需明细的分析。
- 定时 job 把每天滚动成**永久日聚合表**，原始过期后历史数字仍在。
- **实时在线**走 Redis（心跳 key + TTL，不落库）。
- **今日 PV/UV 热计数器**走 Redis（供前台秒开），数据以聚合表/原始为准、可随时重建。

## 关键决策（已确认）

- 统计时区：**Asia/Shanghai** 固定切天（原始存 UTC，聚合/今日计数按此时区切天）。
- IP 隐私：**不存明文 IP**，解析地理后仅存截断哈希 `ip_hash`。
- Origin 反伪造：`/collect` 使用**专用环境变量** `ANALYTICS_ALLOWED_ORIGINS`（含 www/apex，不含 admin），与全局 CORS 解耦，避免影响 admin 跨域；CORS 现状不动，仍由 Nginx 管理。
- 三期全部实现，分期仅作为实现顺序与里程碑。
- 用户身份识别走**方案 A（BFF 中转）**：token 锁在 web BFF 的服务端 HttpOnly cookie，浏览器读不到，故 tracker 发到 web BFF，由 BFF 用 cookie 解析登录态、附 `Authorization: Bearer` 转发给后端，后端 `OptionalAuth` 解析 `user_id`。client 永不自报 user_id（不可信）。
- 细分口径：身份键 `identity = COALESCE(user_id, visitor_id)`。**全部 UV** = distinct `identity`；**注册 UV** = distinct `user_id`（同一人多设备算一个）；**匿名 UV** = distinct `visitor_id`（user_id 为空）。PV 三档按 `is_authenticated` 切分。

---

## 架构总览

```
[web tracker JS] --fetch(keepalive)--> web BFF /api/collect
                                    │ BFF：读 HttpOnly cookie → 附 Authorization: Bearer，透传 visitor_id
                                    ▼
                                  POST /collect (后端，挂 OptionalAuth → 解析 user_id)
                                    │ 同步：Origin/限频/字段校验 → UA/IP-geo/referer 富化 → botfilter
                                    ├─ Redis 实时层：ZADD online、INCR 今日 PV、SADD 今日 UV
                                    └─ 有界 channel ──> [ingest batch worker] ──> 原始表
                                                                                   analytics_events / analytics_sessions
[analytics worker (ticker + 租约)]
   ├─ 日滚动聚合(00:30 Asia/Shanghai, 聚合昨天, upsert 幂等, 支持回填)
   │     └─> analytics_daily / analytics_daily_dim / analytics_page_daily
   ├─ 原始数据清理(超保留期删除)
   └─ 在线表清理(ZREMRANGEBYSCORE)

[后台 API /admin/analytics/*]  ── 读聚合表 + Redis
[公开 API /analytics/public/*] ── 读聚合表 + Redis，短 TTL 缓存 + 脱敏 + 限频
```

---

## 数据模型

### 原始层（短期保留，默认 90 天）

**`analytics_events`** — 每次 PV 一行，支撑访问路径分析：

| 字段 | 说明 |
|---|---|
| `id` | 主键 |
| `event_type` | `page_view` |
| `visitor_id` | 访客标识（复用 Cookie） |
| `user_id` | 登录用户 ID（可空，由后端 `OptionalAuth` 解析，不信前端） |
| `is_authenticated` | 是否登录态（派生） |
| `session_id` | 会话标识（前端生成） |
| `path` | 页面路径（脱敏：剥离敏感 query，长度截断） |
| `title` | 页面标题（长度截断） |
| `referer_host` | 来源 host（只留 host） |
| `referer_type` | `direct` / `search` / `social` / `external` / `internal` |
| `device_type` | `desktop` / `mobile` / `tablet` / `bot` |
| `browser` / `os` | UA 解析结果 |
| `country` / `region` | IP→地理解析结果 |
| `ip_hash` | IP 截断后哈希，不存明文 |
| `is_new_visitor` | 是否新访客 |
| `is_bot` / `bot_reason` | bot 判定结果与命中原因 |
| `is_suspect` | Origin 不匹配等可疑标记（入库但不计聚合） |
| `created_at` | **服务器接收时间**，聚合按此切天 |

索引：`created_at`、`visitor_id`、`session_id`、`path`。

**`analytics_sessions`** — 每会话一行，心跳只 upsert 此表（不为每次心跳插行）：

| 字段 | 说明 |
|---|---|
| `session_id` | 主键 |
| `visitor_id` | 访客标识 |
| `user_id` | 登录用户 ID（可空；会话内登录则回填） |
| `is_authenticated` | 会话是否含登录态 |
| `first_seen` / `last_seen` | 会话起止（服务器时间） |
| `pv_count` | 会话内 PV 数 |
| `entry_path` / `exit_path` | 入口/出口路径 |
| `duration` | `last_seen − first_seen` 秒数 |
| `is_bounce` | `pv_count ≤ 1` 且 `duration < 阈值` |
| `device_type`/`browser`/`os`/`country`/`region`/`referer_type` | 冗余维度，便于聚合 |
| `is_bot` | bot 判定 |

> 会话超时 30 分钟无心跳视为结束（前端生成新 session_id）。

### 聚合层（永久保留）

**`analytics_daily`** — 每天一行站点总览（含三档细分）：
`date` / `pv` / `uv` / `sessions` / `new_visitors` / `avg_duration` / `bounce_rate`
/ `registered_pv` / `registered_uv` / `anonymous_pv` / `anonymous_uv`
（`uv` = 全部 UV = distinct `identity`；注册/匿名 UV 各按身份键去重）

**`analytics_daily_dim`** — 长表，每天每维度每取值一行：
`date` / `dimension`（`referer_type`·`device`·`browser`·`os`·`country`·`user_type`）/ `dim_value` / `pv` / `uv`
其中 `user_type` 取值 `registered` / `anonymous`，对应 UV 按各自身份键去重（registered 按 `user_id`，anonymous 按 `visitor_id`）。
唯一键：`(date, dimension, dim_value)`。

**`analytics_page_daily`** — 每天每路径一行，支撑热门页面排行：
`date` / `path` / `title` / `pv` / `uv`
唯一键：`(date, path)`。

### 实时层（Redis，不落库、可重建）

- **在线人数**：`ZSET analytics:online`，member=`visitor_id`，score=最后心跳时间戳；在线数 = 近 ~90s 内成员计数，过期剔除。
- **今日热计数器**（非 bot 才计，TTL 到次日 + 缓冲，Asia/Shanghai 切天）：
  - `analytics:pv:{yyyymmdd}` — `INCR` 累加今日 PV；另含 `:registered` / `:anonymous` 两个分档 key；
  - `analytics:uv:{yyyymmdd}` — `SET` 存身份键 `identity`，`SCARD` 取今日全部 UV；另 `:registered`（存 user_id）/ `:anonymous`（存 visitor_id）两个 SET 取分档 UV（博客量级精确且省，不上 HLL）。
- 公开 summary 结果缓存：`analytics:public:summary`，短 TTL（60s）。

### 与现有计数的关系

文章/动态的 `/view`（`service/uv`）是单资源精确计数，保留不变；站点分析的 `analytics_page_daily` 是全站路径维度，两者用途不同、并存。

---

## 采集链路

### 前端 tracker（`packages/tracker`，web 引入，admin 不引入）

- **PV 上报**：绑定 App Router **真实导航**（`usePathname` 变化）+ 页面可见，**不绑组件挂载**，避免 Next.js `<Link>` 预取造成幽灵 PV。
- **心跳**：页面可见时每 ~15s 发 `heartbeat`；`visibilitychange` 隐藏即停；`pagehide`/隐藏时补发一次 beacon 以减少时长低估。
- **标识**：`visitor_id` 复用 Cookie；`session_id` 前端生成存 `sessionStorage`，30 分钟无心跳视为新会话。**不传 user_id**（由服务端解析）。
- **传输**：`fetch(url, { keepalive: true, credentials: 'include' })` 发到 **web BFF `/api/collect`**（同源，HttpOnly cookie 自动带上；`keepalive` 保证卸载时也能发出）。
- **载荷**（仅可信字段）：`event_type` / `path` / `title` / `referer` / `session_id` / `screen`。UA、IP、地理一律后端解析。

### web BFF 中转 `/api/collect`（方案 A）

- 读服务端 HttpOnly cookie 解析登录态；登录则附 `Authorization: Bearer <token>` 转发给 Go 后端 `POST /collect`，匿名则不附。
- 透传 `visitor_id` cookie，并把后端的 `Set-Cookie`（首次下发 visitor_id）回写浏览器。
- 透传 `Origin`、真实客户端 IP（`X-Forwarded-For`）等富化所需头。
- 薄代理，不做业务逻辑。

### 后端 `POST /collect`（公开、挂 `OptionalAuth`）

同步处理（快）：
1. `OptionalAuth`：有合法 Bearer → 解析 `user_id` 并置 `is_authenticated=true`；无则匿名。
2. CORS + `RateLimitNormal`（按 IP/visitor 限频）。
3. Origin 校验：命中 `ANALYTICS_ALLOWED_ORIGINS` 正常；缺失/不匹配（生产）标 `is_suspect`；开发环境名单为空则放行。
4. 字段校验：缺 `session_id`/`visitor_id`、referer 与 path 不自洽、必填缺失 → 丢弃。
5. 富化：UA→device/browser/os；IP→country/region 并生成 `ip_hash`；referer→分类；path/referer 脱敏。
6. botfilter 判定 `is_bot`/`bot_reason`。
7. 更新 Redis 实时层（非 bot 才计今日计数，并按 `is_authenticated` 分别累加注册/匿名今日计数；online 始终更新）。
8. 事件投入**有界 channel**（满则丢弃 + 记 drop 计数指标）。

异步处理（ingest batch worker）：
- 批量从 channel 取事件 → page_view 插 `analytics_events` + upsert `analytics_sessions`；heartbeat 仅 upsert session 的 `last_seen`/`pv 不变`。

### botfilter 判定单元

独立单元，输入 UA/IP/行为信号，输出 `(is_bot bool, reason string)`：
- **UA 黑名单**：命中 `bot/spider/crawler/headless/curl/python-requests/Googlebot/Bingbot/Baiduspider…`（社区正则表）。
- **行为校验**：心跳间隔异常过快、单 visitor 短时 PV 暴增 → 标记。
- 命中即标 `is_bot=true` 并入库，但聚合期剔除（保留全量便于规则调整后回溯）。
- 单测覆盖各命中分支。

---

## 聚合 job（analytics worker：ticker 循环 + MySQL 租约）

仿 `internal/worker/notification`，在 `bootstrap` 组装、`go worker.Run(ctx)` 启动。

1. **日滚动聚合**：每天 Asia/Shanghai **00:30** 聚合**昨天**：
   - 过滤 `is_bot=false` 且 `is_suspect=false`；
   - 写 `analytics_daily`（含 registered/anonymous 分档 PV/UV）/ `analytics_daily_dim`（含 `user_type` 维度）/ `analytics_page_daily`，全程 **upsert（幂等）**；
   - 分档 UV 按各自身份键去重：全部 UV 用 `COALESCE(user_id, visitor_id)`、注册 UV 用 `user_id`、匿名 UV 用 `visitor_id`；
   - 支持指定日期**回填重算**（bot 规则调整后重刷历史）；
   - MySQL 租约确保某天只被一个实例聚合一次。
2. **原始数据清理**：每天删除超过保留期（默认 90 天，可配）的 `analytics_events` / `analytics_sessions`。
3. **在线表清理**：定期 `ZREMRANGEBYSCORE` 剔除过期在线成员（读时再按时间窗过滤，双保险）。

---

## 读取 API

遵 `http-api`：路由分组、`pkg/response` 统一响应、出参走 `internal/dto`（禁返回 `model.*`）、Swagger + `make swag`。查询只打聚合表 + Redis。

### 后台（`admin` 分组，`Auth + RequireRole(AdminRole)`）

| 接口 | 用途 |
|---|---|
| `GET /admin/analytics/overview` | 今日 PV/UV、当前在线、累计 PV/UV、较昨日环比；均含全部/注册/匿名三档 |
| `GET /admin/analytics/trend?from&to&metric=pv\|uv\|sessions&segment=all\|registered\|anonymous` | 按天趋势序列，可按用户类型细分 |
| `GET /admin/analytics/dimensions?dimension=referer_type\|device\|browser\|os\|country&from&to` | 维度分布 |
| `GET /admin/analytics/pages?from&to&limit` | 热门页面排行 |
| `GET /admin/analytics/realtime` | 当前在线数 +（可选）最近活跃路径 |

入参边界：`from/to` 默认近 7 天、最大跨度封顶（365 天）；`limit` 封顶（100）；非法区间返回 422。

### 前台公开（无需登录，限频 + 缓存 + 脱敏）

| 接口 | 用途 |
|---|---|
| `GET /analytics/public/summary` | 站点累计访问、今日访问、当前在线；可含「注册用户数 / 访客数」聚合占比 |
| `GET /analytics/public/popular?limit` | 热门文章/页面榜（小 limit） |

公开侧硬规矩：
1. **脱敏**：只出聚合数字；不暴露单访客、`user_id`、具体用户用了什么设备/行为、IP、`country` 以下细粒度、`/admin/*` 路径（聚合时即排除）。注册/匿名仅出**人数聚合**，不出名单。
2. **缓存**：结果进 Redis 短 TTL（60s），扛刷且避免每次打库。
3. **限频**：`RateLimitNormal` 兜底。

---

## 分层落点（遵 `go-layering`）

- `internal/handler/analytics/` — 上报 handler + 查询 handler，按职责拆文件。
- `internal/service/analytics/` — 富化、botfilter、聚合读、缓存。
- `internal/repository/analytics/` — 聚合表查询、原始表写入/清理。
- `internal/worker/analytics/` — 日聚合 / 清理 / 在线清理。
- `internal/model/` — 上述表的 GORM 模型。
- `internal/dto/analytics/` — 出入参。
- `internal/middleware/` — 复用 `VisitorID`、`OptionalAuth`；新增 Origin 校验逻辑（或并入 collect handler）。
- 对外接口暴露 interface 以便 gomock。

前端侧（`blog-frontend`）：
- `packages/tracker/` — tracker SDK（PV/心跳、fetch keepalive、session_id 管理），web 引入、admin 不引入。
- `apps/web/app/api/collect/route.ts` — BFF 中转路由（方案 A）：读 HttpOnly cookie、附 Bearer、透传 visitor_id 与 Set-Cookie，转发后端 `/collect`。

---

## 隐私 / 安全验收清单

- [ ] 不存明文 IP，仅 `ip_hash`（截断后哈希）。
- [ ] `path`/`referer` 入库前剥离敏感 query，referer 只留 host。
- [ ] bot 多层过滤（JS 门槛 + UA 黑名单 + 行为校验 + 限频），`is_bot`/`bot_reason` 入库、聚合剔除。
- [ ] `/collect` Origin 反伪造（专用 `ANALYTICS_ALLOWED_ORIGINS`），不匹配标 suspect。
- [ ] 用户可控字符串长度截断 + 清洗；后台展示转义（防存储型 XSS）。
- [ ] 异步 channel 有界，满则丢弃 + drop 计数，不拖垮主流程。
- [ ] 公开 API 仅出聚合、短 TTL 缓存、限频、排除 `/admin/*`。
- [ ] 聚合 upsert 幂等、支持指定日期回填重算。
- [ ] `user_id` 由后端 `OptionalAuth` 解析，client 不自报；`user_id` 及用户设备/行为仅后台可见，公开 API 不暴露。

---

## 实现分期（三期全做，作为实现顺序）

**Phase 1 — 采集与基础报表（核心闭环，含注册/匿名细分）**
tracker（PV + 心跳，fetch keepalive）→ web BFF `/api/collect` 中转（解析登录态、透传 visitor_id）→ 后端 `/collect`（`OptionalAuth` 解析 user_id + 校验/富化/botfilter/Origin）→ 原始表（含 `user_id`/`is_authenticated`）+ 有界 channel + ingest worker → Redis 在线/今日计数（含注册/匿名分档）→ 日聚合 job（daily/dim/page，含 user_type 与三档 UV）→ 后台 overview/trend/pages（可按 segment 细分）→ 保留期清理。交付后后台即可看全部/注册/匿名的趋势与排行。

**Phase 2 — 会话指标与公开展示**
会话时长/跳出率精化 → dimensions 维度接口 → 前台公开 summary/popular + 缓存脱敏 → 回填工具。

**Phase 3 — 进阶**
headless 高级爬虫识别（`navigator.webdriver` 等前端信号）、访问路径/漏斗分析、`/collect` 签名 token 反伪造。

---

## 配置项（viper / 环境变量）

| 配置 | 说明 | 默认 |
|---|---|---|
| `ANALYTICS_ALLOWED_ORIGINS` | `/collect` Origin 白名单，逗号分隔 | 空（开发放行） |
| analytics.timezone | 切天时区 | `Asia/Shanghai` |
| analytics.retention_days | 原始数据保留天数 | 90 |
| analytics.heartbeat_interval | 前端心跳间隔 | 15s |
| analytics.session_timeout | 会话超时 | 30m |
| analytics.online_window | 在线判定时间窗 | 90s |
| analytics.bounce_duration | 跳出时长阈值 | 据实定 |
| analytics.channel_buffer | 有界 channel 容量 | 据实定 |
| analytics.public_cache_ttl | 公开接口缓存 TTL | 60s |

部署：`docker-compose.yml` 的 `blog-server.environment` 增加 `ANALYTICS_ALLOWED_ORIGINS: ${ANALYTICS_ALLOWED_ORIGINS:-}`，`.env.example` 补充该变量；生产 `.env` 填 `https://www.yevpt.com,https://yevpt.com`。
