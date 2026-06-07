# 文章阅读上报（UV 去重）设计

## 目标

新增 `POST /articles/:id/view` 接口，支持 UV 去重（同一访客同一文章 24h 内只计一次），替换原有 `POST /articles/:id/read`。同时对 `POST /moments/:id/read` 也进行同等改造，替换为 `POST /moments/:id/view`。

## 核心概念

- **PV**（Page View）：不再单独追踪，阅读数即 UV 数
- **UV**（Unique Visitor）：同一 visitor_id + 同一资源 24h 内只增加一次
- **visitor_id**：由后端生成的 UUID v4，通过 Cookie 下发，有效期 1 年

## 架构

```
请求 → VisitorID 中间件 (Cookie) → Handler → UV Service (Redis SET NX EX) → Repo (DB 原子+1)
```

三层分工：
1. **VisitorID 中间件**：读/写 `visitor_id` Cookie，将值存入 `gin.Context`
2. **UV Service**：通用去重服务，Redis `SET NX EX` 实现 24h 窗口去重
3. **Handler**：取 `visitor_id`，调用 UV Service 判断是否新访客，仅首次计入时增加阅读数

## 详细设计

### 1. VisitorID 中间件 (`internal/middleware/visitor.go`)

- 读取 `visitor_id` Cookie
- 如果不存在，生成 UUID v4 并写入 Cookie：
  ```
  Set-Cookie: visitor_id=<uuid>; HttpOnly; SameSite=Lax; Max-Age=31536000; Path=/
  ```
- 将 `visitor_id` 写入 `gin.Context`，key 为 `visitorID`
- 提供 `GetVisitorID(c *gin.Context) string` 供下游读取

### 2. UV 去重服务 (`internal/service/uv/`)

接口定义：

```go
type UVService interface {
    CheckAndMark(ctx context.Context, prefix, targetID, visitorID string, window time.Duration) (bool, error)
}
```

- `prefix`：业务前缀，如 `article:viewed`、`moment:viewed`
- `targetID`：资源 ID（文章 ID、碎语 ID）
- `visitorID`：访客标识
- `window`：去重时间窗口（文章 24h）
- Redis Key 格式：`{prefix}:{targetID}:visitor:{visitorID}`
- 实现：`SET key 1 NX EX windowSeconds`
  - 首次设置成功 → `isNew=true`（新访客）
  - key 已存在 → `isNew=false`（已计过）

### 3. Article View

- 路由：`POST /articles/:id/view`（替换 `POST /articles/:id/read`）
- 中间件：`VisitorID`
- Handler 逻辑：
  1. 从 path 取 `id`
  2. 从 context 取 `visitorID`
  3. 调用 `uvService.CheckAndMark(ctx, "article:viewed", id, visitorID, 24h)`
  4. 若 `isNew=true`，调用 `repo.IncrementReadCount(id)`
  5. 返回 `{ id, view_count }`（无论是否新访客都返回当前阅读数）
- DTO：`ArticleReadResp` 重命名为 `ArticleViewResp`（字段不变）
- 降级：`visitorID` 为空时（中间件未挂或异常），仍执行 +1（保证可用性）

### 4. Moment View

- 路由：`POST /moments/:id/view`（替换 `POST /moments/:id/read`）
- 逻辑与 Article View 完全一致，仅 prefix 改为 `moment:viewed`
- DTO：`MomentReadResp` 重命名为 `MomentViewResp`

### 5. 路由变更

替换：
- `POST /articles/:id/read` → `POST /articles/:id/view`（挂 VisitorID 中间件）
- `POST /moments/:id/read` → `POST /moments/:id/view`（挂 VisitorID 中间件）

## 文件变更清单

| 操作 | 文件 | 说明 |
|------|------|------|
| 新增 | `internal/middleware/visitor.go` | VisitorID 中间件 |
| 新增 | `internal/service/uv/uv.go` | UV 去重服务接口与实现 |
| 新增 | `internal/service/uv/uv_test.go` | UV 服务单元测试 |
| 修改 | `internal/dto/article.go` | ReadResp → ViewResp |
| 修改 | `internal/dto/moment.go` | ReadResp → ViewResp |
| 修改 | `internal/service/article/article.go` | 注入 UVService，Read → View |
| 修改 | `internal/service/moment/service.go` | 注入 UVService，Read → View |
| 修改 | `internal/handler/article/article.go` | Read → View，取 visitor_id |
| 修改 | `internal/handler/moment/query.go` | Read → View，取 visitor_id |
| 修改 | `internal/router/router.go` | 替换路由，挂 VisitorID 中间件 |

## 未包含

- 管理端阅读统计接口（后续可扩展）
- Redis key 清理（24h 自动过期无需清理）
- UV 数据持久化到独立表（当前阅读数直接走 DB 原子 +1，无需额外表）