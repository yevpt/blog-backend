---
name: "http-api"
description: "Use when adding or modifying an HTTP endpoint — covers route registration by permission group, reading identity via jwt.GetClaims, the pkg/response unified-response API, and required Swagger annotations plus make swag. Trigger whenever creating/changing a handler that serves a route, registering a route, or touching API request/response shape."
license: "MIT"
metadata:
  scope: "project"
---

# HTTP 接口

## 路由

- 只在 `internal/router` 注册，按权限显式分组：公开、`Auth`、VIP、admin。
- handler 用 `jwt.GetClaims(c)` 取 `UserId`、`Username`、`Roles`。

## 统一响应（禁直接 `c.JSON`）

```go
response.Success(c, data)
response.Fail(c, response.CodeBadRequest, "参数错误") // HTTP 200，业务 code 表错误
response.Unauthorized(c)
response.Forbidden(c)
response.NotFound(c)
response.ServerError(c)
response.TooManyRequests(c, "请求过于频繁", retryAfterSeconds)
```

## Swagger（新增/改接口后必跑 `make swag`，确认 `docs/` 出现对应 path）

注解写在 handler 方法上方，**不写**在 router 注册处。

必含：`@Summary @Description @Tags @Accept @Produce` 必要 `@Param` `@Success @Router`。真实非 2xx 必写 `@Failure`（401/403/404/429/500）。

- 请求体引用 `internal/dto` 请求 DTO；成功响应用 `response.Response{data=dto.Xxx}`。
- **禁**暴露 `model.*`。
- `response.Fail` 的业务错误不要虚标 HTTP 400，应在 `@Success 200` 描述 `code != 0`。
- **禁**用 `// POST /path` 代替 OpenAPI 注解。
