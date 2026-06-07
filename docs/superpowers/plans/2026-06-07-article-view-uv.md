# 文章阅读上报（UV 去重）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 替换 read 接口为 view 接口，支持 UV 去重（visitor_id Cookie + Redis SET NX EX），文章和碎语两个模块同时改造。

**Architecture:** VisitorID 中间件负责 Cookie 读/写，UV Service 封装 Redis 去重逻辑（通用），Handler 从 Context 取 visitor_id 调 UV Service，仅新访客时 DB 原子 +1。

**Tech Stack:** Go 1.25+, Gin, go-redis, UUID (google/uuid), gomock

---

## File Structure

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/middleware/visitor.go` | VisitorID 中间件 |
| Create | `internal/service/uv/uv.go` | UV 去重服务接口与实现 |
| Create | `internal/service/uv/uv_test.go` | UV 服务单元测试 |
| Modify | `internal/dto/article.go` | `ArticleReadResp` → `ArticleViewResp` |
| Modify | `internal/dto/moment.go` | `MomentReadResp` → `MomentViewResp` |
| Modify | `internal/service/article/article.go` | 注入 UVService，`Read` → `View` |
| Modify | `internal/service/moment/service.go` | 注入 UVService，`Read` → `View` |
| Modify | `internal/service/moment/moment.go` | 接口签名 `Read` → `View` |
| Modify | `internal/handler/article/article.go` | `Read` → `View`，取 visitor_id |
| Modify | `internal/handler/moment/query.go` | `Read` → `View`，取 visitor_id |
| Modify | `internal/router/router.go` | 替换路由，挂 VisitorID 中间件 |
| Modify | `internal/handler/article/article_test.go` | adapter 适配 View |
| Modify | `internal/service/article/article_test.go` | 适配新接口 |
| Modify | `internal/handler/moment/moment_test.go` | adapter 适配 View |
| Modify | `internal/service/moment/moment_test.go` | 适配新接口 |

---

### Task 1: 新增 UV 去重服务

**Files:**
- Create: `internal/service/uv/uv.go`
- Create: `internal/service/uv/uv_test.go`

- [ ] **Step 1: 创建 UV 服务接口与实现**

```go
// internal/service/uv/uv.go
package uv

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// UVService 通用 UV 去重服务，基于 Redis SET NX EX 实现。
// 同一 visitor 对同一资源在 window 时间窗口内只计一次。
type UVService interface {
	// CheckAndMark 检查并标记 UV。
	// prefix: 业务前缀，如 "article:viewed"、"moment:viewed"
	// targetID: 资源 ID
	// visitorID: 访客标识
	// window: 去重时间窗口
	// 返回 true 表示新访客（应计入），false 表示已计过。
	CheckAndMark(ctx context.Context, prefix, targetID, visitorID string, window time.Duration) (bool, error)
}

type uvService struct {
	rdb *redis.Client
}

// NewService 创建 UV 去重服务实例。
func NewService(rdb *redis.Client) UVService {
	return &uvService{rdb: rdb}
}

func (s *uvService) CheckAndMark(ctx context.Context, prefix, targetID, visitorID string, window time.Duration) (bool, error) {
	key := fmt.Sprintf("%s:%s:visitor:%s", prefix, targetID, visitorID)
	ok, err := s.rdb.SetNX(ctx, key, 1, window).Result()
	if err != nil {
		return false, fmt.Errorf("UV 去重写入失败: %w", err)
	}
	return ok, nil
}
```

- [ ] **Step 2: 编写 UV 服务单元测试**

```go
// internal/service/uv/uv_test.go
package uv_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/service/uv"
)

func TestUVService_CheckAndMark_NewVisitor(t *testing.T) {
	svc := uv.NewService(testRedisClient)
	ctx := context.Background()

	isNew, err := svc.CheckAndMark(ctx, "article:viewed", "42", "visitor-abc", 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, isNew, "新访客应返回 true")
}

func TestUVService_CheckAndMark_RepeatVisitor(t *testing.T) {
	svc := uv.NewService(testRedisClient)
	ctx := context.Background()

	_, err := svc.CheckAndMark(ctx, "article:viewed", "43", "visitor-xyz", 24*time.Hour)
	require.NoError(t, err)

	isNew, err := svc.CheckAndMark(ctx, "article:viewed", "43", "visitor-xyz", 24*time.Hour)
	require.NoError(t, err)
	assert.False(t, isNew, "重复访客应返回 false")
}

func TestUVService_CheckAndMark_DifferentVisitors(t *testing.T) {
	svc := uv.NewService(testRedisClient)
	ctx := context.Background()

	_, err := svc.CheckAndMark(ctx, "article:viewed", "44", "visitor-1", 24*time.Hour)
	require.NoError(t, err)

	isNew, err := svc.CheckAndMark(ctx, "article:viewed", "44", "visitor-2", 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, isNew, "不同访客应返回 true")
}

func TestUVService_CheckAndMark_DifferentPrefixes(t *testing.T) {
	svc := uv.NewService(testRedisClient)
	ctx := context.Background()

	_, err := svc.CheckAndMark(ctx, "article:viewed", "45", "visitor-same", 24*time.Hour)
	require.NoError(t, err)

	isNew, err := svc.CheckAndMark(ctx, "moment:viewed", "45", "visitor-same", 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, isNew, "不同前缀应视为不同记录")
}
```

注意：以上测试需要 Redis 连接。如果项目没有集成测试基础设施，可改用 `go-redis/redismock` 或 miniredis。检查项目现有测试模式——当前项目使用 `go-sqlmock` 和 `gomock`，不使用真实 Redis 测试。因此需要引入 miniredis 来做单元测试。

- [ ] **Step 3: 安装 miniredis 依赖**

Run: `go get github.com/alicebob/miniredis/v2`

- [ ] **Step 4: 补全 UV 测试的 Redis 辅助函数并运行测试**

在 `uv_test.go` 中添加 miniredis 辅助：

```go
var testRedisClient *redis.Client

func TestMain(m *testing.M) {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	testRedisClient = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	code := m.Run()
	mr.Close()
	os.Exit(code)
}
```

Run: `go test ./internal/service/uv/... -v -cover`

- [ ] **Step 5: 提交 UV 服务**

```bash
git add internal/service/uv/ go.sum go.mod
git commit -m "feat(uv): 新增 UV 去重服务，基于 Redis SET NX EX 实现"
```

---

### Task 2: 新增 VisitorID 中间件

**Files:**
- Create: `internal/middleware/visitor.go`

- [ ] **Step 1: 创建 VisitorID 中间件**

```go
// internal/middleware/visitor.go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// VisitorIDKey 是 Context 中存储 visitor_id 的 key。
	VisitorIDKey = "visitor_id"
	// VisitorIDCookie 是 Cookie 中 visitor_id 的名称。
	VisitorIDCookie = "visitor_id"
)

// VisitorID 从 Cookie 读取 visitor_id，不存在时生成 UUID v4 并写入 Cookie。
// Cookie 配置：HttpOnly、SameSite=Lax、Max-Age=1 年、Path=/。
func VisitorID() gin.HandlerFunc {
	return func(c *gin.Context) {
		visitorID, err := c.Cookie(VisitorIDCookie)
		if err != nil || visitorID == "" {
			visitorID = uuid.NewString()
			c.SetCookie(
				VisitorIDCookie,
				visitorID,
				86400*365,   // Max-Age: 1 年
				"/",
				"",
				false, // Secure: 开发环境 false，生产由反向代理处理
				true,  // HttpOnly
			)
			// SameSite 通过相同 Cookie 设置无法指定，需要用 SameSite 常量
			// Gin SetCookie 不支持 SameSite 参数，需要额外设置
		}
		c.Set(VisitorIDKey, visitorID)
		c.Next()
	}
}

// GetVisitorID 从 gin.Context 读取 visitor_id。
func GetVisitorID(c *gin.Context) string {
	v, _ := c.Get(VisitorIDKey)
	id, ok := v.(string)
	if !ok {
		return ""
	}
	return id
}
```

注意：Gin 的 `SetCookie` 不支持 `SameSite` 参数。需要手动设置 `SameSite` 属性。需要用 `http.SetCookie` 替代，或在响应后用 `c.Header("Set-Cookie", ...)` 设置。最佳方案是直接构建 `http.Cookie` 并设置。

修正方案——手动构建 Cookie：

```go
// internal/middleware/visitor.go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	VisitorIDKey    = "visitor_id"
	VisitorIDCookie = "visitor_id"
)

// VisitorID 从 Cookie 读取 visitor_id，不存在时生成 UUID v4 并写入 Cookie。
func VisitorID() gin.HandlerFunc {
	return func(c *gin.Context) {
		visitorID, err := c.Cookie(VisitorIDCookie)
		if err != nil || visitorID == "" {
			visitorID = uuid.NewString()
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     VisitorIDCookie,
				Value:    visitorID,
				MaxAge:   86400 * 365,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
		c.Set(VisitorIDKey, visitorID)
		c.Next()
	}
}

// GetVisitorID 从 gin.Context 读取 visitor_id。
func GetVisitorID(c *gin.Context) string {
	v, _ := c.Get(VisitorIDKey)
	id, ok := v.(string)
	if !ok {
		return ""
	}
	return id
}
```

- [ ] **Step 2: 安装 uuid 依赖**

Run: `go get github.com/google/uuid`

- [ ] **Step 3: 提交 VisitorID 中间件**

```bash
git add internal/middleware/visitor.go go.sum go.mod
git commit -m "feat(middleware): 新增 VisitorID 中间件，Cookie 读写 visitor_id"
```

---

### Task 3: 改造 Article 层（DTO → Service → Handler）

**Files:**
- Modify: `internal/dto/article.go`
- Modify: `internal/service/article/article.go`
- Modify: `internal/handler/article/article.go`

- [ ] **Step 1: 重命名 DTO**

在 `internal/dto/article.go` 中，将 `ArticleReadResp` 重命名为 `ArticleViewResp`（字段不变）：

```go
// ArticleViewResp 阅读数响应。
type ArticleViewResp struct {
	// ID 文章 ID。
	ID uint `json:"id" example:"1"`
	// ViewCount 阅读数量。
	ViewCount uint `json:"view_count" example:"21"`
}
```

注意：字段名也从 `ReadCount` 改为 `ViewCount` 以与类型名保持一致。

- [ ] **Step 2: 修改 Article Service 接口和实现**

在 `internal/service/article/article.go` 中：

1. 接口 `Read(id uint) (*dto.ArticleReadResp, error)` → `View(id uint, visitorID string) (*dto.ArticleViewResp, error)`
2. 结构体新增 `uvSvc uv.UVService` 字段
3. `NewArticleService` 签名增加 `uvSvc uv.UVService` 参数
4. 实现改为：

```go
func (s *articleService) View(id uint, visitorID string) (*dto.ArticleViewResp, error) {
	isNew := true
	if visitorID != "" {
		var err error
		isNew, err = s.uvSvc.CheckAndMark(context.Background(), "article:viewed", strconv.FormatUint(uint64(id), 10), visitorID, 24*time.Hour)
		if err != nil {
			// UV 去重失败时降级为直接 +1，保证可用性
			isNew = true
		}
	}
	if !isNew {
		article, err := s.repo.FindPublicDetail(id, nil)
		if err != nil || article == nil {
			return nil, ErrArticleNotFound
		}
		return &dto.ArticleViewResp{ID: article.Article.ID, ViewCount: article.Article.ReadCount}, nil
	}
	article, err := s.repo.IncrementReadCount(id)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, ErrArticleNotFound
	}
	return &dto.ArticleViewResp{ID: article.ID, ViewCount: article.ReadCount}, nil
}
```

需要新增 import: `"context"`, `"strconv"`, `"time"`, `"github.com/vpt/blog-backend/internal/service/uv"`

- [ ] **Step 3: 修改 Article Handler**

在 `internal/handler/article/article.go` 中：

1. `Read` 方法 → `View` 方法
2. 从 context 取 visitor_id：

```go
// View 增加文章阅读数（UV 去重）。
// @Summary 增加文章阅读数
// @Description 同一访客同一文章 24 小时内只增加一次阅读数。
// @Tags 文章
// @Accept json
// @Produce json
// @Param id path int true "文章 ID"
// @Success 200 {object} response.Response{data=dto.ArticleViewResp} "统一响应；code=0 表示更新成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 404 {object} response.Response "文章不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /articles/{id}/view [post]
func (h *ArticleHandler) View(c *gin.Context) {
	id, ok := bindUintPath(c, "id")
	if !ok {
		return
	}
	visitorID := middleware.GetVisitorID(c)

	resp, err := h.svc.View(id, visitorID)
	writeArticleResponse(c, resp, err)
}
```

import 新增 `"github.com/vpt/blog-backend/internal/middleware"`

- [ ] **Step 4: 更新文章 handler 测试适配**

在 `internal/handler/article/article_test.go` 中：

1. `stubArticleService.Read` → `stubArticleService.View`
2. 签名改为 `View(id uint, visitorID string) (*dto.ArticleViewResp, error)`
3. stub 返回 `&dto.ArticleViewResp{ID: id, ViewCount: 2}`
4. 路由从 `/articles/:id/read` → `/articles/:id/view`
5. 如有测试用路由，需加 VisitorID 中间件或手动设 context

- [ ] **Step 5: 更新文章 service 测试适配**

在 `internal/service/article/article_test.go` 中：

1. `NewArticleService(repo, nil)` → `NewArticleService(repo, nil, uvSvc)` 三参数
2. 根据需要新增 View 相关测试用例

- [ ] **Step 6: 提交 Article 层改造**

```bash
git add internal/dto/article.go internal/service/article/ internal/handler/article/
git commit -m "feat(article): Read 接口改为 View，支持 UV 去重"
```

---

### Task 4: 改造 Moment 层（DTO → Service → Handler）

**Files:**
- Modify: `internal/dto/moment.go`
- Modify: `internal/service/moment/moment.go`
- Modify: `internal/service/moment/service.go`
- Modify: `internal/handler/moment/query.go`

- [ ] **Step 1: 重命名 DTO**

在 `internal/dto/moment.go` 中，`MomentReadResp` → `MomentViewResp`，字段 `ReadCount` → `ViewCount`：

```go
// MomentViewResp 阅读数响应。
type MomentViewResp struct {
	// ID 碎语 ID。
	ID uint `json:"id" example:"1"`
	// ViewCount 阅读数量。
	ViewCount uint `json:"view_count" example:"21"`
}
```

- [ ] **Step 2: 修改 Moment Service 接口和实现**

在 `internal/service/moment/moment.go` 中：

1. 接口 `Read(id uint) (*dto.MomentReadResp, error)` → `View(id uint, visitorID string) (*dto.MomentViewResp, error)`
2. 结构体新增 `uvSvc uv.UVService` 字段
3. `NewMomentService` 签名增加 `uvSvc uv.UVService` 参数

在 `internal/service/moment/service.go` 中：

```go
func (s *momentService) View(id uint, visitorID string) (*dto.MomentViewResp, error) {
	isNew := true
	if visitorID != "" {
		var err error
		isNew, err = s.uvSvc.CheckAndMark(context.Background(), "moment:viewed", strconv.FormatUint(uint64(id), 10), visitorID, 24*time.Hour)
		if err != nil {
			isNew = true
		}
	}
	if !isNew {
		moment, err := s.repo.FindPublicDetail(id, nil)
		if err != nil {
			return nil, mapRepoError(err)
		}
		if moment == nil {
			return nil, ErrMomentNotFound
		}
		return &dto.MomentViewResp{ID: moment.Moment.ID, ViewCount: moment.Moment.ReadCount}, nil
	}
	moment, err := s.repo.IncrementReadCount(id)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return &dto.MomentViewResp{ID: moment.ID, ViewCount: moment.ReadCount}, nil
}
```

删去原 `Read` 方法。新增 import: `"context"`, `"strconv"`, `"time"`, `"github.com/vpt/blog-backend/internal/service/uv"`

- [ ] **Step 3: 修改 Moment Handler**

在 `internal/handler/moment/query.go` 中：

`Read` → `View`，从 context 取 visitor_id，更新 swag 注解路由为 `/moments/{id}/view`：

```go
// View 增加碎语阅读数（UV 去重）。
// @Summary 增加碎语阅读数
// @Description 同一访客同一碎语 24 小时内只增加一次阅读数。
// @Tags 碎语
// @Accept json
// @Produce json
// @Param id path int true "碎语 ID"
// @Success 200 {object} response.Response{data=dto.MomentViewResp} "统一响应；code=0 表示更新成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 404 {object} response.Response "碎语不存在"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /moments/{id}/view [post]
func (h *MomentHandler) View(c *gin.Context) {
	id, ok := bindMomentID(c, "id")
	if !ok {
		return
	}
	visitorID := middleware.GetVisitorID(c)

	resp, err := h.svc.View(id, visitorID)
	writeMomentResponse(c, resp, err)
}
```

- [ ] **Step 4: 更新 moment 测试适配**

在 `internal/handler/moment/moment_test.go` 和 `internal/service/moment/moment_test.go` 中：

1. stub/adapter 中 `Read` → `View`，签名改为 `View(id uint, visitorID string) (*dto.MomentViewResp, error)`
2. `NewMomentService(repo, resolver)` → `NewMomentService(repo, resolver, uvSvc)`
3. 移除旧 `Read` 方法适配，新增 `View` 方法适配

- [ ] **Step 5: 提交 Moment 层改造**

```bash
git add internal/dto/moment.go internal/service/moment/ internal/handler/moment/
git commit -m "feat(moment): Read 接口改为 View，支持 UV 去重"
```

---

### Task 5: 修改路由和依赖注入

**Files:**
- Modify: `internal/router/router.go`

- [ ] **Step 1: 更新路由注册**

在 `internal/router/router.go` 中：

1. import 新增 `"github.com/vpt/blog-backend/internal/service/uv"` 和 `"github.com/vpt/blog-backend/internal/middleware"`
2. `routeHandlers` 新增 `uvSvc uv.UVService` 字段
3. `newRouteHandlers` 中创建 `uvSvc := uv.NewService(redisClient)` 并注入到 `articleSvc` 和 `momentSvc`
4. `NewArticleService(articleRepo, objectURLResolver)` → `NewArticleService(articleRepo, objectURLResolver, uvSvc)`
5. `NewMomentService(momentRepo, objectURLResolver)` → `NewMomentService(momentRepo, objectURLResolver, uvSvc)`
6. 路由注册：
   - `r.POST("/articles/:id/read", handlers.article.Read)` → `r.POST("/articles/:id/view", middleware.OptionalAuth(jwtManager), handlers.article.View)`
   
   注意：View 接口需要 VisitorID 中间件。为了最小化改动，将 VisitorID 中间件包装为路由组级别。
   
   在 `registerPublicRoutes` 中：
   ```go
   // 公开路由需要 visitor_id
   r.POST("/articles/:id/view", middleware.VisitorID(), handlers.article.View)
   r.POST("/moments/:id/view", middleware.VisitorID(), handlers.article.View)
   ```
   删除原有的：
   ```go
   r.POST("/articles/:id/read", handlers.article.Read)
   r.POST("/moments/:id/read", handlers.moment.Read)
   ```

- [ ] **Step 2: 验证编译**

Run: `go build ./...`

- [ ] **Step 3: 提交路由改造**

```bash
git add internal/router/router.go
git commit -m "feat(router): 路由从 read 改为 view，挂载 VisitorID 中间件"
```

---

### Task 6: 更新 Swagger 文档

**Files:**
- Modify: `docs/` (自动生成)

- [ ] **Step 1: 运行 swag 生成**

Run: `make swag`

- [ ] **Step 2: 检查生成结果**

确认 `docs/swagger.yaml` 和 `docs/swagger.json` 中出现 `/articles/{id}/view` 和 `/moments/{id}/view` 路径，且无 `/articles/{id}/read` 或 `/moments/{id}/read`。

- [ ] **Step 3: 提交 Swagger 文档**

```bash
git add docs/
git commit -m "docs(swag): 更新 Swagger 文档，read→view"
```

---

### Task 7: 运行全量测试

- [ ] **Step 1: 运行 go test**

Run: `go test ./... -v -cover`

- [ ] **Step 2: 修复所有测试失败**

根据失败输出修复。常见问题：
- stub/adapter 签名不匹配
- import 路径变更
- `NewArticleService`/`NewMomentService` 构造函数参数数量变化

- [ ] **Step 3: 最终提交**

```bash
git add -A
git commit -m "test: 修复全量测试适配 View 接口"
```

---

### Task 8: 生成 Swagger 并最终验证

- [ ] **Step 1: make swag 并检查swagger**

Run: `make swag && grep -c 'view' docs/swagger.yaml`

- [ ] **Step 2: go build 验证**

Run: `go build ./...`

- [ ] **Step 3: go test 全量验证**

Run: `go test ./... -v -cover`