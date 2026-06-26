# 站点分析（Analytics）配置与部署

自建站点分析（类 GA/百度统计）的全部配置项、跨仓耦合、部署操作集中说明。配置散落在「后端 `config.yaml` / 后端环境变量 / 前端 web 环境变量」三处，本文档是唯一权威索引。

> 配置优先级（高覆盖低）：`config.yaml` → `config.{env}.yaml` → `config.local.yaml`(不提交) → 环境变量 `BLOG_*`。详见 [README 配置说明](../README.md#配置说明)。

---

## 1. 一眼看懂：最小可用 vs 完整

| 模式 | 必填 | 效果 |
|---|---|---|
| **最小可用** | `ANALYTICS_ALLOWED_ORIGINS`、`ANALYTICS_SITE_HOST`、`ANALYTICS_IP_SALT` | PV/UV/来源/设备/会话/在线、后台与公开接口全部可用 |
| **+ 地理** | 再加 `ANALYTICS_GEOIP_V4_PATH` / `ANALYTICS_GEOIP_V6_PATH` + 挂载 xdb | 增加国家/省份/城市/ISP/国家代码维度；不配则地理留空、不报错 |
| **+ 反伪造 token** | 再加 `ANALYTICS_COLLECT_TOKEN_SECRET`（前后端同值） | `/collect` 校验签名 token，提高伪造上报门槛；不配则该校验关闭（系统照常工作） |

> token 这一组**要么前后端都配、要么都留空，切忌只配一边**（只配一边会导致真实上报全部被判 suspect、PV/UV 不计数）。

---

## 2. 后端 `config.yaml`（`analytics:` 块）

非敏感、可提交的默认值。生产敏感值经环境变量覆盖（见第 3 节）。

| 键 | 默认 | 说明 |
|---|---|---|
| `timezone` | `Asia/Shanghai` | 切天时区。聚合/今日计数按此切天 |
| `retention_days` | `90` | 原始事件保留天数，超期清理 |
| `online_window` | `90s` | 在线判定时间窗 |
| `session_timeout` | `30m` | 会话超时（前端心跳停止后视为会话结束） |
| `bounce_duration` | `10s` | 跳出判定的停留阈值 |
| `channel_buffer` | `4096` | 异步落库 channel 容量，满则丢弃（记 drop 计数） |
| `public_cache_ttl` | `60s` | 公开统计接口的 Redis 缓存 TTL |
| `geoip_v4_path` | `""` | ip2region IPv4 xdb 路径，空则关闭 IPv4 地理解析。**经 env 注入** |
| `geoip_v6_path` | `""` | ip2region IPv6 xdb 路径，空则关闭 IPv6 地理解析。**经 env 注入** |
| `site_host` | `""` | 站点 apex 顶级域，referer 内/外链判定用。**经 env 注入**，见第 5 节 |
| `ip_salt` | `"change_me"` | IP 哈希盐，仅后端用。**经 env 注入随机串**，见第 6 节 |
| `collect_token_secret` | `""` | collect 签名 token 的 HMAC secret。**经 env 注入**，见第 6 节 |
| `collect_token_ttl` | `6m` | token 有效期上界。**须 ≥ 前端 TTL**，故比前端 5m 多 1m 作时钟余量，见第 6 节 |

---

## 3. 后端环境变量（生产注入）

部署机 `.env` 里设下表左列；`docker-compose.yml` 已把它们映射进容器（注意：`ANALYTICS_ALLOWED_ORIGINS` 走 `os.Getenv` 无前缀，其余走 viper 的 `BLOG_` 前缀）。模板见 [`.env.example`](../.env.example)。

| `.env`（部署机） | 容器内变量 | 必填 | 示例 |
|---|---|---|---|
| `ANALYTICS_ALLOWED_ORIGINS` | `ANALYTICS_ALLOWED_ORIGINS` | ✅ 生产必填 | `https://www.example.com,https://example.com` |
| `ANALYTICS_SITE_HOST` | `BLOG_ANALYTICS_SITE_HOST` | ✅ 建议填 | `example.com` |
| `ANALYTICS_IP_SALT` | `BLOG_ANALYTICS_IP_SALT` | ✅ 必填随机串 | `<openssl rand -hex 32>` |
| `ANALYTICS_GEOIP_V4_PATH` | `BLOG_ANALYTICS_GEOIP_V4_PATH` | 用 IPv4 地理才填 | `/app/geoip/ip2region_v4.xdb` |
| `ANALYTICS_GEOIP_V6_PATH` | `BLOG_ANALYTICS_GEOIP_V6_PATH` | 用 IPv6 地理才填 | `/app/geoip/ip2region_v6.xdb` |
| `ANALYTICS_COLLECT_TOKEN_SECRET` | `BLOG_ANALYTICS_COLLECT_TOKEN_SECRET` | 用 token 才填 | `<与前端同一随机串>` |

> ⚠️ `ANALYTICS_ALLOWED_ORIGINS` **生产不填 = 所有 PV 被判 suspect、不计数**。必须列全 www 与裸域、**不含 admin 域名**，且为**精确全等**（含 `https://`）。

---

## 4. 前端（web）环境变量

前台 `apps/web` 部署环境（`blog-frontend` 仓）。模板见 `blog-frontend/apps/web/.env.example`。

| 变量 | 必填 | 说明 |
|---|---|---|
| `API_BASE_URL` | ✅ | BFF 转发 `/collect` 的后端地址 |
| `ANALYTICS_COLLECT_TOKEN_SECRET` | 用 token 才填 | **必须与后端 `BLOG_ANALYTICS_COLLECT_TOKEN_SECRET` 完全相同**（变量名不同、值相同） |
| `ANALYTICS_COLLECT_TOKEN_TTL_MS` | 可选 | 默认 `300000`(5m)；**须 ≤ 后端 `collect_token_ttl`(6m)** |

---

## 5. 两个「域名配置」别搞混

你的域名出现在**两个用途不同、规则不同**的配置里：

| 配置 | 作用 | 怎么填 | 匹配规则 |
|---|---|---|---|
| `ANALYTICS_SITE_HOST` | referer 内/外链分类 | **apex 一个**：`example.com`（勿填 www） | **后缀匹配**，自动覆盖 `www.*` 等所有子域名 |
| `ANALYTICS_ALLOWED_ORIGINS` | `/collect` Origin 反伪造 | **www 与裸域都列全**：`https://www.example.com,https://example.com` | **精确全等**，不做后缀匹配 |

- `site_host` 填 apex 即可（填 www 反而会把裸域来源误判为 external）；留空则内部来源被判 external，不影响其他功能。
- `ALLOWED_ORIGINS` 少列一个域名，该域名的 PV 就会被标 suspect。

---

## 6. 密钥、TTL 与时钟

**生成随机串**（`IP_SALT` 和 `COLLECT_TOKEN_SECRET` 各生成一个）：
```bash
openssl rand -hex 32
# 或：python3 -c "import secrets; print(secrets.token_hex(32))"
```
- `ANALYTICS_IP_SALT`：纯后端用，**不需与任何系统一致**；设好后保持稳定（改值会让历史 `ip_hash` 与新值对不上）。
- `ANALYTICS_COLLECT_TOKEN_SECRET`：前后端**同一个值**。

**TTL 关系**：前端签 `exp = now + 前端TTL`，后端拒绝 `exp > now + collect_token_ttl`。故须 **后端 ≥ 前端**；当前默认后端 6m / 前端 5m，留 1 分钟吸收时钟偏差。

**时钟（容器部署）**：容器没有自己的时钟，用宿主机时钟。
- web 与后端**同一台宿主机** → 无偏差，无需处理。
- **不同宿主机** → 在各宿主机 OS 开 NTP（`chrony`/`systemd-timesyncd`，不在容器内）。叠加上面的 6m/5m 余量后，常见偏差已可忽略。

---

## 7. 部署操作

1. **数据库迁移**：跑 `make dbsetup`（= `go run ./cmd/dbsetup`）执行 AutoMigrate，建 analytics 表并补新增列。AutoMigrate 是**加法式幂等**——以后模型新增字段/表重跑即可；删列/改名/改类型需手写 SQL。
   - ⚠️ 首跑确认 `analytics_page_daily` 建表成功（`path` varchar(512) 在复合主键中；MySQL 8 正常，≤5.6 可能报索引超长）。
2. **地理库（可选）**：下载 `ip2region_v4.xdb` 与 `ip2region_v6.xdb` 放到挂载目录（compose 已挂 `./geoip:/app/geoip:ro`），分别设置 `ANALYTICS_GEOIP_V4_PATH=/app/geoip/ip2region_v4.xdb`、`ANALYTICS_GEOIP_V6_PATH=/app/geoip/ip2region_v6.xdb`。
3. **前端**：部署含 tracker + BFF 路由（`apps/web/app/api/collect`、`apps/web/app/api/analytics-token`）的版本。

---

## 8. 上线验证

- 访问前台任意页 → `analytics_events` 出现新行且 `is_suspect=0`、后台 `GET /admin/analytics/overview` 在线数增长。
- **今日数据**走 Redis 实时、秒可见；**历史趋势**由日聚合 worker 在 **00:30（Asia/Shanghai）** 聚合昨天，故次日才有。
- 公开接口 `GET /analytics/public/summary`、`/analytics/public/popular` 返回脱敏聚合。

---

## 9. 接口一览

| 分组 | 接口 |
|---|---|
| 上报 | `POST /collect`（公开，恒 204） |
| 后台（需登录 + Admin） | `/admin/analytics/overview` `trend` `pages` `dimensions` `friend-links` `realtime` `paths` `funnel` `backfill` |
| 前台公开（限频 + 缓存 + 脱敏） | `/analytics/public/summary` `/analytics/public/popular` |

完整出入参见 Swagger（`make swag` 生成，导入 Apifox）。设计背景见 [`docs/superpowers/specs/2026-06-25-self-hosted-analytics-design.md`](superpowers/specs/2026-06-25-self-hosted-analytics-design.md)。

---

## 10. 本地开发测试

**本地几乎零额外配置**：前端 `API_BASE_URL` 已在 `apps/web/.env.local` 配好（OAuth 等代理共用）；analytics 的 `ANALYTICS_ALLOWED_ORIGINS`/`collect_token_secret`/`geoip_v4_path`/`geoip_v6_path` 本地留空即为「放行 + 不校验 token + 关闭地理」，`ip_salt` 有默认值。

### 准备（一次性）
```bash
# 本地 Redis（config.local.yaml 默认 localhost:6379）
redis-server

# ⚠️ 迁移数据库结构（含 is_suspect 等新列），改过模型后都要重跑
make dbsetup

# 起后端（热重载，:8080）
make dev
```

### 测法 A：跑前端走全链路（推荐，最贴近真实）
```bash
# blog-frontend 仓
pnpm dev:web          # http://localhost:3000
```
浏览器翻几页 → tracker 自动发 `page_view` + 每 15s 心跳 → 经 BFF `/api/collect` → 后端。登录后再翻，事件带 `user_id`（注册档）。**这就是"啥也不配、访问网页就有数据"的路径。**

### 测法 B：直接 curl `/collect`（只验后端，不起前端）
```bash
# -c/-b 维持同一 visitor_id；-A 给浏览器 UA（否则被判 bot 不计数）
curl -i -c /tmp/cj -b /tmp/cj -X POST http://localhost:8080/collect \
  -H 'Content-Type: application/json' -H 'Origin: http://localhost:3000' \
  -A 'Mozilla/5.0 (Macintosh) Chrome/120' \
  -d '{"event_type":"page_view","path":"/hello","title":"Hi","session_id":"sess-1"}'
# 期望 204；心跳改 event_type=heartbeat、同 session_id
```

### 怎么看数据
| 看什么 | 命令 |
|---|---|
| 实时（Redis，秒见） | `redis-cli ZCARD analytics:online`；`redis-cli GET analytics:pv:$(TZ=Asia/Shanghai date +%Y%m%d)` |
| 公开接口（免登录） | `curl http://localhost:8080/analytics/public/summary` |
| 原始入库 | `SELECT * FROM analytics_events ORDER BY id DESC LIMIT 10;` |
| 后台今日（需 admin token） | `GET /admin/analytics/overview`（今日走 Redis、立即有） |
| 聚合表 trend/pages/dimensions | 先 `POST /admin/analytics/backfill?from=<今天>&to=<今天>` 强制聚合，再查 |
| 友链长期来源 | 先 `POST /admin/analytics/backfill?from=<今天>&to=<今天>` 写入 `analytics_friend_link_daily`，再查 `/admin/analytics/friend-links` |

> 日聚合 worker 正常 00:30（Asia/Shanghai）跑昨天；本地测聚合表用 `backfill` 强制聚合任意日期。

### 常见「看着没数据」的坑
- **curl 不加 `-A` 浏览器 UA** → 判 `is_bot`、入库但不计数（用浏览器访问 web 无此问题）。
- **去重**：同 `session_id`+`path` 5s 内重复 PV 不计——连发换 `path` 或隔 5s。
- **新访客**：同一 `visitor_id` 仅首个 page_view `is_new_visitor=true`；重测先 `redis-cli DEL analytics:visitor:seen:<id>` 或 `FLUSHDB`。
- **visitor_id 是 HttpOnly cookie**：curl 不带 cookie jar 每次都算新访客。
