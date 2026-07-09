# 运行时与安全

本文记录权限、限流、通知 worker 与统计 worker 等运行时机制。

## 权限体系

三种角色，权重依次降低：

| 角色 | 标识 | 说明 |
|------|------|------|
| 管理员 | `ROLE_ADMIN` | 可访问所有接口 |
| VIP | `ROLE_VIP` | 可访问 VIP 及以下接口 |
| 普通用户 | `ROLE_NORMAL` | 默认角色 |

路由注册时通过中间件声明权限：

```go
r.GET("/articles", ...)
authed := r.Group("/", middleware.Auth(jwtMgr, userCache))
vip := r.Group("/", middleware.Auth(jwtMgr, userCache), middleware.RequireRole(roles.VipRole))
admin := r.Group("/admin", middleware.Auth(jwtMgr, userCache), middleware.RequireRole(roles.AdminRole))
```

`internal/router/routes.go` 按公开、登录、VIP、Admin 四层注册路由。

## 限流与封禁

所有限流统一实现在 `internal/middleware/ratelimit.go`，按 IP 或登录用户 ID 维度用 Redis 做滑动窗口计数：超过软限只返回 `429` 提示减速，超过硬限则触发封禁。

| 档位 | 适用场景 | 窗口 / 软限 / 硬限 | 硬限后递进封禁 |
|------|------|------|------|
| Strict | 验证码、注册、密码重置等高风险接口 | 60s / 5 / 20 | 10min -> 30min -> 2h -> 24h |
| Normal | 后台敏感操作，如 analytics backfill | 60s / 10 / 30 | 10min -> 30min -> 2h -> 24h |
| Loose | 登录、管理员登录、OAuth 授权/回调 | 120s / 30 / 100 | 10min -> 30min -> 2h -> 24h |
| Public | 公开 GET 接口兜底防护 | 300s / 5000 / 20000 | 5min -> 30min -> 2h -> 24h -> 48h -> 7天 |
| 上传类 | 碎语、临时图片、头像等 | 各接口独立配置 | 与 Strict 同曲线 |

设计要点：

- 按档位隔离封禁 key，例如 `ban:strict:ip:<IP>` 与 `ban:public:ip:<IP>` 互不连带。
- 同一档位在窗口期内反复触发硬限会逐级延长封禁时长。
- 高并发下只有第一个请求负责升级计数与落键封禁，避免突发流量被重复加罚。
- Redis 命令出错时安全退化；Redis 不可用期间限流整体 fail open。
- 暂无后台解封接口，误封后需运维直连 Redis 删除对应 `ban:<档位>:ip:<IP>` key。

## 通知 worker

通知拆分为站内信与邮件两条链路，由 `internal/worker/notification` 异步驱动：

| 链路 | 默认节奏 | 说明 |
|------|----------|------|
| Dispatch | 5s 一轮 | 业务事件解析收件人与偏好，写入站内信 |
| Plan | 30s 一轮 | 按配额规划邮件发送批次 |
| Send | 按配置间隔 | 领取邮件批次并通过 SMTP 发送 |

站内信走 `/notifications*` 接口；邮件任务、批次与配额由 `/admin/notifications/*` 管理。`email.worker_enabled=false` 时只关闭邮件链路，站内信不受影响。

## 站点访问统计

访问统计链路：

```text
前端 /collect
  -> CollectHandler 校验 token 与 VisitorID
  -> Enricher 富化 UA / referer / GeoIP
  -> Redis 实时层维护今日和在线状态
  -> Ingestor 异步落库
  -> Scheduler 每日 Rollup 与清理
```

关键约束：

- collect handler 与 worker 必须共享同一个 `Ingestor` 实例。
- 切天时区、在线窗口、原始事件保留天数来自 `analytics` 配置。
- GeoIP xdb 路径为空时关闭对应 IPv4/IPv6 地理解析。
- 公开统计接口有缓存 TTL，完整配置见 [站点分析配置](analytics-configuration.md)。

## 第三方登录

当前 Provider：GitHub、Gitee、QQ、微博、百度。

本地 OAuth App 的 callback URL 必须与配置里的 `redirect_uri` 精确一致。授权流程使用一次性 Redis state；支持的平台启用 PKCE。第三方 access token 只在后端保存，不返回前端。
