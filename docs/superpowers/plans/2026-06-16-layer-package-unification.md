# 三层布局统一重构 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 handler/service/repository 三层中仍是扁平单文件的领域（user、category、tag、friendlink、social_auth）迁移为按领域分包的子包，使三层布局风格统一，并清理迁移残留的死代码。

**Architecture:** 沿用项目已有子包约定（参照 `internal/{handler,service,repository}/guestbook/`、`article/`）。每个领域在每层成为独立子包，包内有门面入口文件 `<domain>.go`（构造函数 + 接口 + 对外类型/sentinel error），实现按职责拆到 `query.go`/`mutation.go`/`mapper.go`/`response.go`。DI 接线集中在 `internal/router/router.go`。分阶段执行，每阶段 `go build ./...` + 该领域 `go test` 绿灯后才进下一阶段。

**Tech Stack:** Go 1.25、Gin、GORM、gomock（`go.uber.org/mock`）、testify、swaggo/swag。

---

## 项目约定（执行前必读）

执行本计划前，先用 Skill 工具加载这些项目 skill 并严格遵循：`go-layering`、`go-readability`、`go-testing`、`http-api`、`git-commit`。要点：

- **分层边界**：handler 只绑参/调 service/选 response；service 只调 repository 接口、不碰 GORM；repository 返回 `model.*` 或包内聚合类型，**禁返回 `dto.*`**。
- **包组织**：一层内风格统一；门面入口文件只放对外构造函数/接口/公开方法，实现细节拆到同包其他文件。
- **依赖注入**：db/redis/logger/mailer/jwt 全部构造注入，禁全局变量。
- **`interface{}` 口径**：字面量统一写 `any`（gomock 生成文件除外，重新生成即可）。
- **commit message**：`<type>(<scope>): <中文主题>`，由 `commit-msg` 钩子强制校验。本重构统一用 `refactor` type。

## 命名约定（本计划统一口径）

| 领域 | 子包目录 | package 名 | router import 别名 |
|---|---|---|---|
| friendlink | `internal/{layer}/friendlink/` | `friendlink` | `friendlinkhandler` / `friendlinkservice` / `friendlinkrepo` |
| category | `internal/{layer}/category/` | `category` | `categoryhandler` / `categoryservice` / `categoryrepo` |
| tag | `internal/{layer}/tag/` | `tag` | `taghandler` / `tagservice` / `tagrepo` |
| social_auth | `internal/repository/socialauth/` | `socialauth` | `socialauthrepo` |
| user | `internal/{layer}/user/` | `user` | `userhandler` / `userservice` / `userrepo` |

## 门面文件模板（参照 `internal/service/guestbook/guestbook.go`）

迁移后每个领域的入口文件结构（以 service 层为例）：

```go
package category // 与目录名一致

import ( /* dto、<domain>repo、storage 等 */ )

// 1. sentinel error（如有）
var ErrXxx = errors.New("...")

// 2. 对外接口
type CategoryService interface { /* 方法签名 */ }

// 3. 实现结构体（小写）
type categoryService struct { repo categoryrepo.CategoryRepository /* ... */ }

// 4. 构造函数
func NewCategoryService(repo categoryrepo.CategoryRepository) CategoryService {
    return &categoryService{repo: repo}
}
```

实现方法拆到 `query.go`（读）、`mutation.go`（写）、`mapper.go`（model⇆dto）；handler 层把响应组装拆到 `response.go`、参数绑定拆到 `binding.go`（参照 `internal/handler/guestbook/`、`internal/handler/comment/`）。

## 验证命令（每阶段复用）

```bash
go build ./...                                   # 全量编译
go test ./internal/handler/<d>/... ./internal/service/<d>/... ./internal/repository/<d>/...  # 领域测试
go vet ./...                                     # 静态检查
```

mockgen 重新生成（gomock 领域）：

```bash
go run go.uber.org/mock/mockgen \
  -destination=internal/repository/<d>/mock/mock_<d>_repository.go \
  -package=mock \
  github.com/vpt/blog-backend/internal/repository/<d> <Interface>Repository
```

---

## Task 0: 死代码清理 + interface{}→any + 共享 strutil 抽取

**Files:**
- Delete（11 个空壳文件，均仅 1 行 `package X`）:
  - `internal/handler/comment.go` `internal/handler/moment.go` `internal/handler/media.go` `internal/handler/message.go`
  - `internal/service/comment.go` `internal/service/moment.go` `internal/service/media.go` `internal/service/message.go`
  - `internal/repository/comment.go` `internal/repository/moment.go` `internal/repository/message.go`
- Create: `pkg/strutil/strutil.go`
- Delete: `internal/service/string.go`
- Modify: `internal/service/category.go`、`internal/service/tag.go`、`internal/service/friendlink.go`（改用 `strutil.CleanOptional`）
- Modify（interface{}→any）: 见步骤 4

- [ ] **Step 1: 确认 11 个文件确实是空壳，然后删除**

```bash
cd /Volumes/External/SynologyDrive/Codes/Blog/blog-backend
for f in internal/handler/comment.go internal/handler/moment.go internal/handler/media.go internal/handler/message.go \
         internal/service/comment.go internal/service/moment.go internal/service/media.go internal/service/message.go \
         internal/repository/comment.go internal/repository/moment.go internal/repository/message.go; do
  echo "$f: $(wc -l < "$f") 行"
done
```

Expected: 每个文件输出 `1 行`。确认后删除：

```bash
git rm internal/handler/comment.go internal/handler/moment.go internal/handler/media.go internal/handler/message.go \
       internal/service/comment.go internal/service/moment.go internal/service/media.go internal/service/message.go \
       internal/repository/comment.go internal/repository/moment.go internal/repository/message.go
```

- [ ] **Step 2: 创建共享 strutil 包**

`pkg/strutil/strutil.go`：

```go
// Package strutil 提供字符串通用处理工具。
package strutil

import "strings"

// CleanOptional 规整可选字符串：去首尾空白；trim 后为空则返回 nil。
// 用于把前端传入的空白/空串归一为「未设置」。
func CleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
```

- [ ] **Step 3: 替换 cleanOptionalString 调用并删除旧文件**

在 `internal/service/category.go`、`internal/service/tag.go`、`internal/service/friendlink.go` 中：
- 把 `cleanOptionalString(` 全部替换为 `strutil.CleanOptional(`
- 在各文件 import 块加入 `"github.com/vpt/blog-backend/pkg/strutil"`

然后删除旧 helper：

```bash
git rm internal/service/string.go
```

- [ ] **Step 4: interface{} → any（生产代码）**

逐文件把 `interface{}` 字面量改为 `any`（仅限非测试、非 gomock 生成文件）。涉及文件：

```bash
grep -rln "interface{}" --include="*.go" internal pkg \
  | grep -v "_test.go" | grep -v "/mock/" | grep -v "docs.go"
```

对上面列出的每个文件，将 `interface{}` 替换为 `any`（逐处确认语义，通常是 map 值或函数参数类型）。**不要**改 `internal/repository/mock/*.go` 与 `docs/docs.go`。

- [ ] **Step 5: 编译 + 测试 + 静态检查**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: 全部 PASS（行为零变化）。

- [ ] **Step 6: 提交**

```bash
git add -A
git commit -m "refactor(cleanup): 删除空壳死文件并抽取 strutil 共享工具"
```

（若 interface{}→any 改动较多想分开提交，可拆成第二个 commit：`refactor(style): 统一 interface{} 为 any`。）

---

## Task 1: 迁移 friendlink（流程模板，无跨域共享，测试用手写 fake）

**Files:**
- Move: `internal/handler/friendlink.go` → `internal/handler/friendlink/friendlink.go`
- Move: `internal/handler/friendlink_test.go` → `internal/handler/friendlink/friendlink_test.go`
- Move: `internal/service/friendlink.go` → `internal/service/friendlink/friendlink.go`
- Move: `internal/service/friendlink_test.go` → `internal/service/friendlink/friendlink_test.go`
- Move: `internal/repository/friendlink.go` → `internal/repository/friendlink/friendlink.go`
- Move: `internal/repository/friendlink_test.go` → `internal/repository/friendlink/friendlink_test.go`
- Modify: `internal/router/router.go`

- [ ] **Step 1: 建目录并 git mv 文件**

```bash
mkdir -p internal/handler/friendlink internal/service/friendlink internal/repository/friendlink
git mv internal/handler/friendlink.go      internal/handler/friendlink/friendlink.go
git mv internal/handler/friendlink_test.go internal/handler/friendlink/friendlink_test.go
git mv internal/service/friendlink.go      internal/service/friendlink/friendlink.go
git mv internal/service/friendlink_test.go internal/service/friendlink/friendlink_test.go
git mv internal/repository/friendlink.go      internal/repository/friendlink/friendlink.go
git mv internal/repository/friendlink_test.go internal/repository/friendlink/friendlink_test.go
```

- [ ] **Step 2: 改 package 声明**

- `internal/handler/friendlink/friendlink.go`、`internal/service/friendlink/friendlink.go`、`internal/repository/friendlink/friendlink.go`：`package handler|service|repository` → `package friendlink`
- 三个 `friendlink_test.go`：原 `package handler|service|repository`（内部测试）改为 `package friendlink`；若原本是 `xxx_test` 外部测试则改为 `package friendlink_test`（读文件确认）。

- [ ] **Step 3: 修正包内/跨包引用**

- service 层 `friendlink.go`：原引用 `repository.FriendLinkRepository`、`repository.FriendLinkUpdateData` 改为引入 `friendlinkrepo "github.com/vpt/blog-backend/internal/repository/friendlink"` 并改为 `friendlinkrepo.FriendLinkRepository` 等。
- handler 层 `friendlink.go`：原引用 `service.FriendLinkService` 改为引入 `friendlinkservice "github.com/vpt/blog-backend/internal/service/friendlink"` 并改为 `friendlinkservice.FriendLinkService`。
- service 测试里的 `repository.FriendLinkUpdateData` 等类型同步改为 `friendlinkrepo.*`（fake 实现随测试一起迁移，无需改 mock，因为 friendlink 用手写 fake）。
- handler 测试里的 `service.FriendLinkService` 同步改为 `friendlinkservice.*`。

- [ ] **Step 4:（可选，按 go-readability 判断）拆分大文件**

`friendlink.go`（~279 行）若同时含读/写/转换逻辑，按 guestbook 模板把实现拆为 `query.go`/`mutation.go`/`mapper.go`，门面 `friendlink.go` 仅留接口+构造函数+错误。文件不大可保持单文件。

- [ ] **Step 5: 更新 router.go 接线**

import 块新增（删除随后不再需要的根包引用判断见 Task 5）：

```go
friendlinkhandler "github.com/vpt/blog-backend/internal/handler/friendlink"
friendlinkservice "github.com/vpt/blog-backend/internal/service/friendlink"
friendlinkrepo    "github.com/vpt/blog-backend/internal/repository/friendlink"
```

`routeHandlers` 字段类型：`friendLink *handler.FriendLinkHandler` → `friendLink *friendlinkhandler.FriendLinkHandler`

`newRouteHandlers` 内构造调用：

```go
friendLinkRepo := friendlinkrepo.NewFriendLinkRepository(db)
friendLinkSvc := friendlinkservice.NewFriendLinkService(friendLinkRepo, objectStore)
// ...
friendLink: friendlinkhandler.NewFriendLinkHandler(friendLinkSvc),
```

（路由注册行 `handlers.friendLink.ListPublic` 等不变。）

- [ ] **Step 6: 编译 + 测试**

```bash
go build ./... && go vet ./... && \
go test ./internal/handler/friendlink/... ./internal/service/friendlink/... ./internal/repository/friendlink/...
```

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add -A
git commit -m "refactor(friendlink): 迁移友链三层为按领域分包"
```

---

## Task 2: 迁移 category（gomock 领域，mock 需重新生成到子包）

**Files:**
- Move: `internal/handler/category.go` → `internal/handler/category/category.go`（+ `category_test.go`）
- Move: `internal/service/category.go` → `internal/service/category/category.go`（+ `category_test.go`）
- Move: `internal/repository/category.go`、`category_query.go`、`category_mutation.go` → `internal/repository/category/`（+ `category_test.go`）
- Create: `internal/repository/category/mock/mock_category_repository.go`（mockgen 生成）
- Delete: `internal/repository/mock/mock_category_repository.go`
- Modify: `internal/router/router.go`、`internal/service/category/category_test.go`（mock import 路径）

- [ ] **Step 1: 建目录并 git mv**

```bash
mkdir -p internal/handler/category internal/service/category internal/repository/category
git mv internal/handler/category.go      internal/handler/category/category.go
git mv internal/handler/category_test.go internal/handler/category/category_test.go
git mv internal/service/category.go      internal/service/category/category.go
git mv internal/service/category_test.go internal/service/category/category_test.go
git mv internal/repository/category.go          internal/repository/category/category.go
git mv internal/repository/category_query.go    internal/repository/category/query.go
git mv internal/repository/category_mutation.go internal/repository/category/mutation.go
git mv internal/repository/category_test.go     internal/repository/category/category_test.go
```

- [ ] **Step 2: 改 package 声明**（同 Task 1 套路）

handler/service/repository 三个目录下的非测试文件 → `package category`；测试文件按内部/外部测试改 `package category` 或 `package category_test`。

- [ ] **Step 3: 修正跨包引用**

- service `category.go`：`repository.CategoryRepository` → 引入 `categoryrepo "...internal/repository/category"`，改 `categoryrepo.CategoryRepository`；`cleanOptionalString` 已在 Task 0 改为 `strutil.CleanOptional`。
- handler `category.go`：`service.CategoryService` → 引入 `categoryservice "...internal/service/category"`，改 `categoryservice.CategoryService`。

- [ ] **Step 4: 重新生成 mock 到子包，删除旧 mock**

```bash
mkdir -p internal/repository/category/mock
go run go.uber.org/mock/mockgen \
  -destination=internal/repository/category/mock/mock_category_repository.go \
  -package=mock \
  github.com/vpt/blog-backend/internal/repository/category CategoryRepository
git rm internal/repository/mock/mock_category_repository.go
```

- [ ] **Step 5: 更新 service 测试的 mock import**

`internal/service/category/category_test.go`：把 `"github.com/vpt/blog-backend/internal/repository/mock"` 改为 `"github.com/vpt/blog-backend/internal/repository/category/mock"`（`mock.NewMockCategoryRepository` 用法不变）。同时把测试里对 `repository.*` 类型/参数的引用改为 `categoryrepo.*`（如有）。

- [ ] **Step 6:（可选）按 go-readability 拆分 service `category.go`（~240 行）为 `query.go`/`mutation.go`/`mapper.go`。**

- [ ] **Step 7: 更新 router.go 接线**

```go
categoryhandler "github.com/vpt/blog-backend/internal/handler/category"
categoryservice "github.com/vpt/blog-backend/internal/service/category"
categoryrepo    "github.com/vpt/blog-backend/internal/repository/category"
```

字段 `category *handler.CategoryHandler` → `*categoryhandler.CategoryHandler`；构造：

```go
categoryRepo := categoryrepo.NewCategoryRepository(db)
categorySvc := categoryservice.NewCategoryService(categoryRepo)
// ...
category: categoryhandler.NewCategoryHandler(categorySvc),
```

- [ ] **Step 8: 编译 + 测试**

```bash
go build ./... && go vet ./... && \
go test ./internal/handler/category/... ./internal/service/category/... ./internal/repository/category/...
```

Expected: PASS。

- [ ] **Step 9: 提交**

```bash
git add -A
git commit -m "refactor(category): 迁移分类三层为按领域分包"
```

---

## Task 3: 迁移 tag（gomock 领域 + service 依赖 articleSvc）

**Files:**
- Move: `internal/handler/tag.go`(+test) → `internal/handler/tag/`
- Move: `internal/service/tag.go`(+test) → `internal/service/tag/`
- Move: `internal/repository/tag.go`、`tag_query.go`、`tag_mutation.go`(+test) → `internal/repository/tag/`
- Create: `internal/repository/tag/mock/mock_tag_repository.go`
- Delete: `internal/repository/mock/mock_tag_repository.go`
- Modify: `internal/router/router.go`、tag service 测试

- [ ] **Step 1: 建目录并 git mv**

```bash
mkdir -p internal/handler/tag internal/service/tag internal/repository/tag
git mv internal/handler/tag.go      internal/handler/tag/tag.go
git mv internal/handler/tag_test.go internal/handler/tag/tag_test.go
git mv internal/service/tag.go      internal/service/tag/tag.go
git mv internal/service/tag_test.go internal/service/tag/tag_test.go
git mv internal/repository/tag.go          internal/repository/tag/tag.go
git mv internal/repository/tag_query.go    internal/repository/tag/query.go
git mv internal/repository/tag_mutation.go internal/repository/tag/mutation.go
git mv internal/repository/tag_test.go     internal/repository/tag/tag_test.go
```

- [ ] **Step 2: 改 package 声明 → `package tag` / `package tag_test`（按文件确认）。**

- [ ] **Step 3: 修正跨包引用**

- service `tag.go`：`repository.TagRepository` → `tagrepo.TagRepository`（引入 `tagrepo "...internal/repository/tag"`）。`articleservice.ArticleService` 引用保持不变（article 已是子包）。`cleanOptionalString` 已改为 `strutil.CleanOptional`。
- handler `tag.go`：`service.TagService` → `tagservice.TagService`（引入 `tagservice "...internal/service/tag"`）。

- [ ] **Step 4: 重新生成 mock，删除旧 mock**

```bash
mkdir -p internal/repository/tag/mock
go run go.uber.org/mock/mockgen \
  -destination=internal/repository/tag/mock/mock_tag_repository.go \
  -package=mock \
  github.com/vpt/blog-backend/internal/repository/tag TagRepository
git rm internal/repository/mock/mock_tag_repository.go
```

- [ ] **Step 5: 更新 tag service 测试 mock import** 为 `internal/repository/tag/mock`；`repository.*` 类型引用改 `tagrepo.*`。注意 tag service 测试若用到 article service 的 mock/fake，保持其现有引用不变。

- [ ] **Step 6:（可选）拆分 service `tag.go`（~242 行）。**

- [ ] **Step 7: 更新 router.go 接线**

```go
taghandler "github.com/vpt/blog-backend/internal/handler/tag"
tagservice "github.com/vpt/blog-backend/internal/service/tag"
tagrepo    "github.com/vpt/blog-backend/internal/repository/tag"
```

字段 `tag *handler.TagHandler` → `*taghandler.TagHandler`；构造（注意 `articleSvc` 必须在 tagSvc 之前已构造，现状即如此）：

```go
tagRepo := tagrepo.NewTagRepository(db)
tagSvc := tagservice.NewTagService(tagRepo, articleSvc)
// ...
tag: taghandler.NewTagHandler(tagSvc),
```

- [ ] **Step 8: 编译 + 测试**

```bash
go build ./... && go vet ./... && \
go test ./internal/handler/tag/... ./internal/service/tag/... ./internal/repository/tag/...
```

- [ ] **Step 9: 提交**

```bash
git add -A
git commit -m "refactor(tag): 迁移标签三层为按领域分包"
```

---

## Task 4: 迁移 social_auth（仅 repository 层，被 oauth service 消费）

**Files:**
- Move: `internal/repository/social_auth.go` → `internal/repository/socialauth/socialauth.go`
- Move: `internal/repository/social_auth_test.go` → `internal/repository/socialauth/socialauth_test.go`
- Modify: `internal/service/oauth/oauth.go`、`internal/router/router.go`

- [ ] **Step 1: 建目录并 git mv**

```bash
mkdir -p internal/repository/socialauth
git mv internal/repository/social_auth.go      internal/repository/socialauth/socialauth.go
git mv internal/repository/social_auth_test.go internal/repository/socialauth/socialauth_test.go
```

- [ ] **Step 2: 改 package 声明** → `package socialauth` / `package socialauth_test`（按文件确认）。

- [ ] **Step 3: 更新 oauth service 引用**

`internal/service/oauth/oauth.go`：把对 `repository.SocialAuthRepository` / `repository.NewSocialAuthRepository` 相关引用改为引入 `socialauthrepo "github.com/vpt/blog-backend/internal/repository/socialauth"` 并改用 `socialauthrepo.SocialAuthRepository`。先 grep 确认引用点：

```bash
grep -n "SocialAuth" internal/service/oauth/oauth.go
```

- [ ] **Step 4: 更新 router.go 接线**

```go
socialauthrepo "github.com/vpt/blog-backend/internal/repository/socialauth"
```

构造调用 `repository.NewSocialAuthRepository(db)` → `socialauthrepo.NewSocialAuthRepository(db)`。

- [ ] **Step 5: 编译 + 测试**

```bash
go build ./... && go vet ./... && \
go test ./internal/repository/socialauth/... ./internal/service/oauth/...
```

- [ ] **Step 6: 提交**

```bash
git add -A
git commit -m "refactor(socialauth): 迁移社交登录仓储为独立子包"
```

---

## Task 5: 迁移 user（最大、blast radius 最大，含 UserCacheService 跨层共享）

**Files:**
- Move: `internal/handler/user.go`(+`user_test.go`) → `internal/handler/user/`
- Move: `internal/service/user.go`、`user_cache.go`、`user_detail.go`、`user_cache_test.go`、`user_test.go` → `internal/service/user/`
- Move: `internal/repository/user.go`(+`user_test.go`) → `internal/repository/user/`
- Create: `internal/repository/user/mock/mock_user_repository.go`
- Delete: `internal/repository/mock/mock_user_repository.go`、空目录 `internal/repository/mock/`
- Modify: `internal/router/router.go`、`internal/middleware/auth.go`(+`auth_test.go`)、`internal/service/auth/auth.go`、`internal/service/oauth/oauth.go`

- [ ] **Step 1: 建目录并 git mv**

```bash
mkdir -p internal/handler/user internal/service/user internal/repository/user
git mv internal/handler/user.go      internal/handler/user/user.go
git mv internal/handler/user_test.go internal/handler/user/user_test.go
git mv internal/service/user.go            internal/service/user/user.go
git mv internal/service/user_cache.go      internal/service/user/cache.go
git mv internal/service/user_cache_test.go internal/service/user/cache_test.go
git mv internal/service/user_detail.go     internal/service/user/detail.go
git mv internal/service/user_test.go       internal/service/user/user_test.go
git mv internal/repository/user.go      internal/repository/user/user.go
git mv internal/repository/user_test.go internal/repository/user/user_test.go
```

- [ ] **Step 2: 改 package 声明** → handler/service/repository 下非测试文件 `package user`；测试文件按内部/外部确认（`package user` 或 `package user_test`）。

- [ ] **Step 3: 修正包内/跨包引用**

- service `user/user.go`：构造函数 `NewUserService(cache UserCacheService, repo repository.UserRepository, resolver storage.ObjectURLResolver)` —— `UserCacheService` 现与 user 同包（来自 cache.go），保持不带包名；`repository.UserRepository` 改为引入 `userrepo "...internal/repository/user"` 并改 `userrepo.UserRepository`。
- service `user/cache.go`、`detail.go`：同样把 `repository.*` 改 `userrepo.*`。
- handler `user/user.go`：`service.UserService` → 引入 `userservice "...internal/service/user"`，改 `userservice.UserService`。

- [ ] **Step 4: 重新生成 user repo mock，删除旧 mock 与空目录**

```bash
mkdir -p internal/repository/user/mock
go run go.uber.org/mock/mockgen \
  -destination=internal/repository/user/mock/mock_user_repository.go \
  -package=mock \
  github.com/vpt/blog-backend/internal/repository/user UserRepository
git rm internal/repository/mock/mock_user_repository.go
rmdir internal/repository/mock 2>/dev/null || true
```

- [ ] **Step 5: 更新 service 测试 mock import** 为 `internal/repository/user/mock`；测试里 `repository.*` 引用改 `userrepo.*`。

- [ ] **Step 6: 更新跨层共享 UserCacheService 的引用点**

`UserCacheService` 从根 `service` 包迁到 `internal/service/user` 包后，更新所有外部引用：

- `internal/middleware/auth.go`：`service.UserCacheService` → 引入 `userservice "github.com/vpt/blog-backend/internal/service/user"`，改 `userservice.UserCacheService`。
- `internal/middleware/auth_test.go`：同样把 `service.UserCacheService` 注释/类型改为 `userservice.UserCacheService`（mockUserCache stub 不变，只换被实现的接口包名）。
- `internal/service/auth/auth.go`：
  - `service.UserCacheService`（字段 line 63、参数 line 77）→ `userservice.UserCacheService`；import 把 `"...internal/service"` 改为 `userservice "...internal/service/user"`。
  - `repository.UserRepository`（字段 line 58、参数 line 72）→ `userrepo.UserRepository`；import 把 `"...internal/repository"` 改为 `userrepo "...internal/repository/user"`（确认 auth.go 不再用根 repository 的其他符号，是则替换、否则并存）。
- `internal/service/oauth/oauth.go`：
  - 已 `import userservice "...internal/service"`，把路径改为 `"...internal/service/user"`，`userservice.UserCacheService` 用法不变。
  - `repository.UserRepository`（字段 line 55、参数 line 64）→ `userrepo.UserRepository`；oauth 的 SocialAuthRepository 引用已在 Task 4 改为 `socialauthrepo`，故此处改完后根 `"...internal/repository"` import 应已无其他用途，删除它，新增 `userrepo "...internal/repository/user"`。

同步更新这两个构造函数被调用处的参数类型推断（router 中 `authservice.NewAuthService(userRepo, ...)`、`oauthservice.NewOAuthService(..., userRepo, ...)` 传入的 `userRepo` 现为 `userrepo.UserRepository`，类型自动匹配，无需改调用代码）。

先 grep 全量确认引用点（迁移前快照）：

```bash
grep -rn "service.UserCacheService\|service.UserService\|service.NewUserService\|service.NewUserCacheService" --include="*.go" internal | grep -v "internal/service/user/"
```

逐条改完后该 grep 应只剩 user 子包内部引用。

- [ ] **Step 7: 更新 router.go 接线**

```go
userhandler "github.com/vpt/blog-backend/internal/handler/user"
userservice "github.com/vpt/blog-backend/internal/service/user"
userrepo    "github.com/vpt/blog-backend/internal/repository/user"
```

- 字段：`user *handler.UserHandler` → `*userhandler.UserHandler`；`userCache service.UserCacheService` → `userservice.UserCacheService`。
- 构造：

```go
userRepo := userrepo.NewUserRepository(db)
userCacheSvc := userservice.NewUserCacheService(userRepo, objectStore, redisClient)
userSvc := userservice.NewUserService(userCacheSvc, userRepo, objectStore)
// authSvc、oauthSvc 继续接收 userCacheSvc（其参数类型现为 userservice.UserCacheService）
// ...
user: userhandler.NewUserHandler(userSvc),
```

注意：`authservice.NewAuthService(userRepo, ...)` 与 `oauthservice.NewOAuthService(..., userRepo, ...)` 调用代码不变；其签名中的 repo 参数类型已在 Step 6 改为 `userrepo.UserRepository`，传入的 `userRepo` 类型自动匹配。

- [ ] **Step 8:（可选）按 go-readability 拆分 service `user/user.go`（~348 行）为 `query.go`/`mutation.go`/`mapper.go`，handler `user/user.go`（~337 行）拆出 `binding.go`/`response.go`。**

- [ ] **Step 9: 编译 + 测试（含受影响的 middleware/auth/oauth）**

```bash
go build ./... && go vet ./... && \
go test ./internal/handler/user/... ./internal/service/user/... ./internal/repository/user/... \
        ./internal/middleware/... ./internal/service/auth/... ./internal/service/oauth/...
```

Expected: PASS。

- [ ] **Step 10: 提交**

```bash
git add -A
git commit -m "refactor(user): 迁移用户三层为按领域分包并更新共享缓存接口引用"
```

---

## Task 6: 全量收口验证

**Files:** 无新增改动，仅校验。

- [ ] **Step 1: 确认根包不再残留扁平领域文件**

```bash
ls internal/handler/*.go internal/service/*.go internal/repository/*.go
```

Expected: handler 仅剩 `health.go`、`test.go`（及其测试）；service 顶层无业务文件；repository 顶层无业务文件。`internal/repository/mock/` 目录已删除。

- [ ] **Step 2: 全量编译 / 静态检查 / 测试**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: 全部 PASS。

- [ ] **Step 3: Swagger 一致性**

```bash
make swag && git status --porcelain docs/
```

Expected: `docs/` 无改动（API 未变）或仅有因注解位置变动产生的等价 diff；若有非预期 diff，回查是否误改了接口签名。

- [ ] **Step 4: 路由不变性比对**

人工核对 `internal/router/router.go` 的 `registerPublicRoutes`/`registerAuthedRoutes`/`registerVIPRoutes`/`registerAdminRoutes` 中所有路由路径、HTTP 方法、中间件、handler 绑定与重构前一致（仅 handler 变量的包类型变化，绑定方法名不变）。

- [ ] **Step 5:（如有 Swagger 重生成）提交**

```bash
git add -A
git commit -m "refactor(docs): 重生成 swagger 文档"   # 仅当 docs/ 有改动时
```

---

## Self-Review 备注（已核对）

- **Spec 覆盖**：死代码清理(Task0)、5 领域迁移(Task1-5)、interface{}→any(Task0)、收口验证(Task6) 均有对应任务。
- **类型一致性**：构造函数与接口名取自实际代码（`NewUserService(cache UserCacheService, repo, resolver)`、`NewTagService(repo, articleSvc)` 等）；import 别名口径见「命名约定」表，全计划统一。
- **mock 差异**：friendlink 用手写 fake（随测试迁移，无 mockgen）；category/tag/user 用 gomock（需重生成到子包并删旧）；与现状一致。
- **共享 helper**：`cleanOptionalString` 在 Task0 抽到 `pkg/strutil`，被 category/tag/friendlink 复用，避免迁移后重复。
- **风险点**：user 阶段的 `UserCacheService` 跨 middleware/auth/oauth 共享，Step6 用 grep 兜底确认引用清零。
