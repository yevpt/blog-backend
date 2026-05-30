# Blog Backend — AI 编码规范

本文件供 Claude Code 和 AI 工具参考，确保生成的代码风格、架构和规范保持一致。

---

## 项目概述

Go 1.25+ 个人博客 API 服务（纯后端），分层架构，Docker 部署。

**模块名**：`github.com/vpt/blog-backend`

---

## 技术栈速查

| 用途 | 库 |
|------|----|
| Web 框架 | `github.com/gin-gonic/gin` |
| ORM | `gorm.io/gorm` + `gorm.io/driver/mysql` |
| 日志 | `go.uber.org/zap` |
| 配置 | `github.com/spf13/viper` |
| JWT | `github.com/golang-jwt/jwt/v5` |
| 密码 | `golang.org/x/crypto/bcrypt` |
| Redis | `github.com/redis/go-redis/v9` |
| 对象存储 | `github.com/aws/aws-sdk-go-v2/service/s3`（Garage） |

---

## 目录结构与各层职责

```
internal/handler/    ← HTTP 层：解析参数，调 service，返回响应。不写业务逻辑。
internal/service/    ← 业务逻辑层：核心规则在这里。不直接操作数据库。
internal/repository/ ← 数据访问层：所有 GORM 查询在这里。不写业务逻辑。
internal/model/      ← GORM 数据库模型（对应数据库表结构）。
internal/dto/        ← 请求/响应 DTO（HTTP 传输用，与 model 解耦）。
internal/middleware/ ← Gin 中间件（鉴权、RBAC、日志、恢复）。
internal/router/     ← 所有路由注册（唯一入口，新增路由在此处添加）。
pkg/                 ← 无业务依赖的基础设施包。
```

---

## 命名规范

- **文件名**：`snake_case.go`（如 `article_comment.go`）
- **结构体**：`PascalCase`（如 `ArticleHandler`）
- **接口**：以 `I` 开头或直接描述行为（如 `ArticleRepository` 或 `IArticleRepository`）
- **方法**：`PascalCase`（公开），`camelCase`（私有）
- **常量**：`PascalCase` 或 `ALL_CAPS`（视情况）

---

## 编码原则

### 分层规则（严格遵守）

1. **handler 层**：只做参数绑定、调用 service、返回响应。不写 SQL，不写业务规则。
2. **service 层**：只写业务逻辑。需要数据时调 repository 接口，不直接用 gorm。
3. **repository 层**：只写数据查询。返回 model，不返回 dto。
4. **model 不直接暴露给 HTTP 层**：必须通过 dto 转换后再返回给前端。

### 依赖注入

所有依赖（db、redis、logger 等）通过构造函数参数注入，不使用全局变量：

```go
// 正确
func NewArticleHandler(svc ArticleService) *ArticleHandler { ... }

// 禁止
var globalDB *gorm.DB
```

### 接口定义

Service 和 Repository 层均定义接口，方便测试时 mock：

```go
// internal/service/article.go
type ArticleService interface {
    GetById(id uint) (*dto.ArticleResp, error)
    Create(req *dto.ArticleCreateReq) error
}

type articleService struct {
    repo repository.ArticleRepository
}
```

---

## 权限中间件（RBAC）

路由注册时通过中间件声明，无需在 handler 内重复校验：

```go
// 公开（默认，任何人可访问）
r.GET("/articles", handler.List)

// 需要登录（任意角色）
authed := r.Group("/", middleware.Auth(jwtMgr))
authed.POST("/comments", handler.CreateComment)

// 需要 VIP 权限（VIP 或 Admin）
vip := r.Group("/", middleware.Auth(jwtMgr), middleware.RequireRole(roles.VipRole))

// 仅管理员
admin := r.Group("/admin", middleware.Auth(jwtMgr), middleware.RequireRole(roles.AdminRole))
```

在 handler 中获取当前登录用户：

```go
claims := jwt.GetClaims(c) // 返回 *jwt.Claims，含 UserId、Username、Roles
```

---

## 统一 API 响应

**禁止在 handler 中直接调用 `c.JSON`**，统一使用 `response` 包：

```go
import "github.com/vpt/blog-backend/pkg/response"

response.Success(c, data)               // 成功，HTTP 200，code=0
response.Fail(c, response.CodeBadRequest, "参数错误") // 业务失败
response.Unauthorized(c)               // 401
response.Forbidden(c)                  // 403
response.NotFound(c)                   // 404
response.ServerError(c)                // 500
```

---

## 注释规范

**除 Go 关键词、包名、类型名等技术术语外，注释全部使用中文。**

注释要覆盖以下层次：

### 公开方法/函数
说明**职责（是什么）+ 关键约束（为什么这样设计）**：

```go
// FindByIdentifier 根据标识符查询用户，支持 username / email / phone 三合一匹配。
// 用户可能记不清注册时用的是哪个标识符，三合一避免登录失败的困惑。
// 找不到时返回 nil, nil（不视为错误），由调用方决定如何处理。
func (r *userRepo) FindByIdentifier(identifier string) (*model.User, error) { ... }
```

### 公开类型、常量、包级变量
说明**是什么 + 用途**：

```go
// ErrTooManyRequests 短期发送频率超限时返回，区别于日频次耗尽的 ErrDailyLimitExceeded。
var ErrTooManyRequests = errors.New("发送过于频繁，请稍后再试")

// RateLimitConfig 限流中间件的参数配置，通常使用预设的 Strict/Normal，无需手动构造。
type RateLimitConfig struct {
    Window      time.Duration // 计数窗口长度，到期后计数重置
    SoftLimit   int           // 超过此次数触发 429，但不封禁 IP
    HardLimit   int           // 超过此次数写入全局封禁标记
    BanDuration time.Duration // IP 封禁持续时长
}
```

### 函数体内的关键步骤
只在**非显而易见的步骤**上注释，说明**原因/约束**，不描述代码在做什么：

```go
// 用户不存在时仍执行 bcrypt 比对，防止通过响应时间差枚举账号是否存在
if user == nil {
    bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
    return nil, ErrInvalidCredential
}
```

### 反例（禁止）

```go
// 查询用户（❌ 好名字已经说明了，注释毫无价值）
func (r *userRepo) FindByIdentifier(...) { ... }

// 遍历角色列表（❌ 代码本身已经表达，不需要重复）
for _, role := range roles { ... }
```

---

## Commit 规范

格式：`<类型>: <中文描述>`，描述不加句号，首行不超过 72 字符。

| 类型 | 适用场景 |
|------|---------|
| `feat` | 新功能、新接口、新模块 |
| `fix` | 缺陷修复（含安全问题、逻辑错误） |
| `chore` | 依赖安装、构建配置、工具脚本 |
| `refactor` | 重构（不改变外部行为） |
| `test` | 仅新增或修改测试代码 |
| `docs` | 仅文档变更（README、注释等） |
| `perf` | 性能优化 |

示例：

```
feat: JWT 扩展 TokenType，新增 GenerateAccess/GenerateRefresh
fix: 修复限流中间件 Incr+Expire 竞态，消除 key 永不过期风险
chore: 添加 gomail、testify、sqlmock、gomock、miniredis 依赖
refactor: 将 auth 逻辑从 user service 拆分为独立的 auth service
test: 补充 UserRepository FindByIdentifier 的边界测试用例
```

---

## 测试规范

- **Repository 层**：使用 `go-sqlmock` mock 数据库连接
- **Service 层**：使用 `gomock` mock Repository 接口（核心，必须覆盖）
- **Handler 层**：使用 `httptest` + `testify` 测试 HTTP 行为
- 测试文件命名：`xxx_test.go`，包名加 `_test` 后缀

---

## 禁止事项

- 禁止在 handler 层写 SQL 或业务逻辑
- 禁止直接将 `model.*` 返回给前端（必须通过 dto 转换）
- 禁止使用全局变量存储 db、redis 等基础设施
- 禁止在生产代码中 `fmt.Println`（使用 `zap.Logger`）
- 禁止在接口定义中使用 `interface{}`（使用泛型或具体类型）
