---
name: "http-api"
description: "Use when adding or modifying an HTTP endpoint — covers route registration, identity/authorization, request bounds, abuse controls, cross-resource cleanup, unified response, Swagger, and verification. Trigger whenever creating/changing a handler, route, API request/response shape, file upload, or endpoint side effect."
license: "MIT"
metadata:
  scope: "project"
---

# HTTP 接口

接口实现不是只把 happy path 跑通。每次新增/修改接口，先做一次 **API Risk Pass**，再写代码。

## API Risk Pass

在动代码前，用最短形式确认 5 件事；简单查询接口也要过一遍，允许写“无”：

1. **谁能调**：公开 / 登录 / VIP / admin；是否只能操作自己的资源。
2. **输入边界**：字符串长度、枚举、数组数量、分页上限、文件大小/类型。
3. **资源成本**：是否会发邮件、写对象存储、跑压缩、批量写库、调用第三方；高成本必须限流。
4. **失败收口**：DB、Redis、Garage、第三方任一步失败时，是否会留下半成品。
5. **旧状态清理**：更新/删除时，不再引用的对象、缓存、关系表、消息是否要同步清理。

如果任一项答案不明确，先读现有相近接口；仍不明确再问用户，不要靠猜。

## 路由

- 只在 `internal/router` 注册，按权限显式分组：公开、`Auth`、VIP、admin。
- handler 用 `jwt.GetClaims(c)` 取 `UserId`、`Username`、`Roles`。
- 需要完整用户信息时，用已有中间件写入的用户详情；不要信任请求体里的 `user_id`。
- 高成本写接口加限流：优先按登录用户 ID，其次按 IP。复用 `internal/middleware`，不要另造全局限流器。

## Handler 边界

- handler 只做绑定、轻量读取、身份提取、调用 service、选择响应；不写业务规则和数据库逻辑。
- 请求体要有上限：multipart 文件读取必须用 `LimitReader` 或同等机制，不能无上限 `ReadAll`。
- 绑定失败、大小超限、枚举错误统一走 `response.Fail`，不要直接 `c.JSON`。

## Service 边界

- service 做业务校验、权限判断、资源上限、跨资源编排和失败清理。
- 涉及 Garage/Redis/第三方 + DB 的接口，必须设计补偿：
  - 对象成功、DB 失败：删除本次新对象。
  - DB 成功、旧对象不再引用：提交后删除旧对象。
  - 更新替换列表：保留仍引用的旧资源，只清理不再引用的资源。
- 资源 key 不由前端决定；前端传来的 URL/key 必须校验存在和归属范围。

## 文件上传

只要接口接收文件，必须同时定义：

- 单文件大小上限、总文件数上限。
- 真实内容校验：不能只看后缀或 `Content-Type`。
- 存储策略：对象 key、是否去重、是否压缩/转码、保存哪些元数据。
- 更新语义：新文件、旧 URL/key、排序、删除旧文件如何表达。
- 失败清理：本次成功上传但后续失败的对象必须删除。

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

业务错误保持 HTTP 200 + 非 0 `code`；真实认证、授权、限流、服务异常使用对应 HTTP 状态。

## Swagger（新增/改接口后必跑 `make swag`，确认 `docs/` 出现对应 path）

注解写在 handler 方法上方，**不写**在 router 注册处。

必含：`@Summary @Description @Tags @Accept @Produce` 必要 `@Param` `@Success @Router`。真实非 2xx 必写 `@Failure`（401/403/404/429/500）。

- 请求体引用 `internal/dto` 请求 DTO；成功响应用 `response.Response{data=dto.Xxx}`。
- **禁**暴露 `model.*`。
- `response.Fail` 的业务错误不要虚标 HTTP 400，应在 `@Success 200` 描述 `code != 0`。
- **禁**用 `// POST /path` 代替 OpenAPI 注解。

## 测试底线

改接口至少覆盖：

- 成功路径。
- 身份/权限失败。
- 输入边界：数量、长度、大小、枚举、分页上限中与接口相关的项。
- 高成本接口的限流或拒绝策略。
- 跨资源失败清理：尤其是 DB + Garage/Redis/第三方。
- 更新接口的旧状态清理。

能用现有 fake/sqlmock/httptest 覆盖就不要只靠手测。最后运行相关包测试；改 Swagger 跑 `make swag`。
