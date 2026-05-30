# Blog Backend — AI 编码规范

Go 1.25+ 个人博客 API 服务（纯后端），分层架构，Docker 部署。**模块名**：`github.com/vpt/blog-backend`

---

## 技术栈

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

## 目录结构

```
internal/handler/    ← HTTP 层：绑参数、调 service、返响应。不写业务逻辑。
internal/service/    ← 业务逻辑层。不直接操作数据库。
internal/repository/ ← 数据访问层：所有 GORM 查询。返回 model，不返回 dto。
internal/model/      ← GORM 模型（对应表结构）。
internal/dto/        ← 请求/响应 DTO，与 model 解耦。
internal/middleware/ ← Gin 中间件。
internal/router/     ← 所有路由注册的唯一入口。
pkg/                 ← 无业务依赖的基础设施包。
```

---

## 核心规则

### 分层（严格遵守）

1. **handler**：参数绑定 → 调 service → 返响应。禁止写 SQL 和业务规则。
2. **service**：业务逻辑。通过 repository 接口访问数据，不直接用 gorm。
3. **repository**：数据查询。返回 model，不返回 dto。
4. **model 禁止直接暴露给前端**，必须转成 dto。

### 依赖注入与接口

- 所有依赖（db、redis、logger 等）通过构造函数注入，禁止全局变量。
- Service 和 Repository 层均定义接口，便于 mock 测试。

### 权限中间件（RBAC）

```go
r.GET("/articles", handler.List)                                                       // 公开
authed := r.Group("/", middleware.Auth(jwtMgr))                                        // 需登录
vip   := r.Group("/", middleware.Auth(jwtMgr), middleware.RequireRole(roles.VipRole))  // VIP+
admin := r.Group("/admin", middleware.Auth(jwtMgr), middleware.RequireRole(roles.AdminRole)) // 仅管理员

claims := jwt.GetClaims(c) // handler 中获取当前用户，含 UserId/Username/Roles
```

### 统一 API 响应

禁止在 handler 中直接调用 `c.JSON`，统一使用 `response` 包：

```go
response.Success(c, data)
response.Fail(c, response.CodeBadRequest, "参数错误")
response.Unauthorized(c) / response.Forbidden(c) / response.NotFound(c) / response.ServerError(c)
response.TooManyRequests(c, "请求过于频繁", retryAfterSeconds)
```

---

## 注释规范

除技术术语（Go 关键词、包名、类型名）外全部使用中文，覆盖以下层次：

- **公开方法/函数**：职责（是什么）+ 关键约束（为什么这样设计）
- **公开类型、常量、包级变量**：是什么 + 用途
- **结构体字段**：用途描述
- **函数体关键步骤**：只注释非显而易见的原因/约束，不描述代码在做什么

```go
// FindByIdentifier 根据标识符查询用户，支持 username / email / phone 三合一匹配。
// 找不到时返回 nil, nil，不视为错误，由调用方决定处理方式。
func (r *userRepo) FindByIdentifier(identifier string) (*model.User, error) { ... }

// ErrTooManyRequests 短期发送频率超限，区别于日频次耗尽的 ErrDailyLimitExceeded。
var ErrTooManyRequests = errors.New("发送过于频繁，请稍后再试")

type RateLimitConfig struct {
    SoftLimit int // 超过此次数触发 429，但不封禁 IP
    HardLimit int // 超过此次数写入全局封禁标记
}

// 用户不存在时仍执行 bcrypt 比对，防止通过响应时间差枚举账号是否存在
if user == nil { ... }
```

---

## Commit 规范

格式：`<类型>: <中文描述>`，不加句号，首行 ≤72 字符。

`feat` 新功能 | `fix` 缺陷修复 | `chore` 依赖/工具 | `refactor` 重构 | `test` 测试 | `docs` 文档 | `perf` 性能

```
feat: JWT 扩展 TokenType，新增 GenerateAccess/GenerateRefresh
fix: 修复限流中间件 Incr+Expire 竞态，消除 key 永不过期风险
chore: 添加 gomail、testify、sqlmock、gomock、miniredis 依赖
```

---

## 测试规范

- **Repository**：`go-sqlmock` mock 数据库连接
- **Service**：`gomock` mock Repository 接口（核心，必须覆盖）
- **Handler**：`httptest` + `testify`
- 测试文件：`xxx_test.go`，包名加 `_test` 后缀

---

## 禁止事项

- handler 层写 SQL 或业务逻辑
- 直接将 `model.*` 返回给前端
- 全局变量存储 db、redis 等基础设施
- 生产代码中使用 `fmt.Println`（用 `zap.Logger`）
- 接口定义中使用 `interface{}`（用泛型或具体类型）
