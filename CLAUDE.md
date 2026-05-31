# Blog Backend — AI 编码规范

Go 1.25+ 个人博客后端 API，模块 `github.com/vpt/blog-backend`。本文件、`CLAUDE.md`、`.cursorrules` 必须保持一致。

## 技术栈

Gin、GORM/MySQL、zap、viper、jwt/v5、bcrypt、go-redis、AWS SDK S3（Garage）、swaggo/swag。

## 目录职责

```text
internal/handler/    HTTP 层：绑定参数、调 service、返回 response，不写业务逻辑。
internal/service/    业务层：只通过 repository 接口访问数据。
internal/repository/ 数据层：封装 GORM，返回 model，不返回 dto。
internal/model/      GORM 表结构，禁止直接返回前端。
internal/dto/        API 入参/出参唯一结构来源。
internal/middleware/ Gin 中间件。
internal/router/     路由注册唯一入口。
pkg/                 无业务依赖的基础设施包。
docs/                swag 生成文档，可由 make swag 重建。
```

## 分层规则

- handler：只绑定参数、调用 service、选择 response；禁止 SQL/GORM/业务规则。
- service：承载业务逻辑；只能依赖 repository 接口。
- repository：承载数据查询；返回 `model.*`，不返回 `dto.*`。
- dto：对外请求和响应；返回前必须把 `model.*` 转成 `dto.*`。
- 依赖注入：db、redis、logger、mailer、jwt manager 等全部构造注入，禁止全局变量。
- Service 和 Repository 层都定义接口，便于 gomock 测试。

## 代码组织与阅读性

所有代码优先考虑人的阅读性：能运行，也要让人容易进入、顺着路径理解，结构清爽、克制、有诗意和美观。

- 每个包/模块/独立功能区尽量有清晰入口文件（如 `storage.go`、`client.go`、`manager.go`、`router.go`），只放外部真正需要的公开类型、构造函数、公开方法和主要调用方式。
- 入口文件是门面：只放对外功能和薄转发；测试替换点、mock 接口、内部状态、第三方库适配、算法细节等不要放入口，除非它们确实是外部 API。
- 内部实现按职责拆文件，如初始化、签名、路径处理、配置转换、数据查询、业务策略、测试辅助；每个文件只承担一种清晰职责，禁止大杂烩。
- 阅读路径要明确：先看入口理解“做什么、怎么用”，再按需进入细节；不要让读者一开始陷入底层细节或测试设施。
- 函数保持短小；当一个函数同时做校验、初始化、分支策略、第三方调用和结果转换时，拆成有名字的小函数。
- 命名体现业务语义和阅读顺序，避免只有技术细节；内部函数也要从名字看出职责。
- 新增或明显扩展独立模块/工具/基础设施能力时，优先补 `README.md` 或包级说明：用途、入口文件、配置、调用、返回值、测试。
- 用户反馈“可读性不好/看得头大/拆分不清楚”时，优先调整组织结构和阅读路径，而不是只补注释。

## 路由与权限

- 路由只在 `internal/router` 注册，按权限显式分组：公开、`Auth`、VIP、admin。
- handler 通过 `jwt.GetClaims(c)` 获取 `UserId`、`Username`、`Roles`。

## 统一响应

handler 禁止直接 `c.JSON`，必须用 `pkg/response`：

```go
response.Success(c, data)
response.Fail(c, response.CodeBadRequest, "参数错误") // HTTP 200，业务 code 表达错误
response.Unauthorized(c)
response.Forbidden(c)
response.NotFound(c)
response.ServerError(c)
response.TooManyRequests(c, "请求过于频繁", retryAfterSeconds)
```

## Swagger / OpenAPI

所有对外 HTTP 接口必须写 swag 注解；新增或修改接口后执行 `make swag` 并检查 `docs/swagger.yaml/json` 出现对应 `paths`。

必须包含：`@Summary`、`@Description`、`@Tags`、`@Accept`、`@Produce`、必要 `@Param`、`@Success`、`@Router`。真实非 2xx 响应必须写 `@Failure`（401/403/404/429/500 等）。

约束：

- 注解写在 handler 方法上方，不写 router 注册处。
- 请求体引用 `internal/dto` 请求 DTO；成功响应用 `response.Response{data=dto.Xxx}`。
- Swagger 禁止暴露 `model.*`。
- `response.Fail` 的业务错误不要虚标 HTTP 400，应在 `@Success 200` 描述 `code != 0`。
- 禁止用 `// POST /path` 代替 OpenAPI 注解。

## 注释规范

除 Go 关键词、包名、类型名、协议名等技术术语外，注释使用中文。

- 公开方法/函数：写清职责和关键约束。
- 公开类型、常量、包级变量：写清用途。
- 结构体字段：写清含义，尤其 DTO 和配置结构。
- 函数体步骤：每个逻辑步骤写一行注释，说明做什么和为什么。

## 测试规范

- Repository：`go-sqlmock`。
- Service：`gomock` mock Repository，核心业务必须覆盖。
- Handler：`httptest` + `testify`。
- 测试文件 `xxx_test.go`，包名用 `_test` 后缀（同包测试仅在确需访问内部实现时使用）。
- 修改接口至少跑相关包测试；修改公共逻辑跑 `go test ./...`。

## Commit 规范

格式：`<类型>: <中文描述>`，不加句号，首行不超过 72 字符。类型：`feat`、`fix`、`chore`、`refactor`、`test`、`docs`、`perf`。

## 禁止事项

- handler 写 SQL/GORM/业务逻辑。
- 直接返回 `model.*` 给前端或写入 Swagger 响应。
- 用全局变量保存 db、redis、logger 等基础设施。
- 生产代码使用 `fmt.Println`，必须用 `zap.Logger`。
- 接口定义使用 `interface{}`；应使用泛型或具体类型。
- 新增/修改 HTTP 接口但不补 OpenAPI 注解、不执行 `make swag`。
