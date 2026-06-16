# 三层布局统一重构 — 设计文档

日期：2026-06-16
状态：已与用户确认设计，待 writing-plans

## 背景与问题

`internal/` 下 handler、service、repository 三层存在「布局割裂」：较新的领域已按子包组织（`article/`、`auth/`、`comment/`、`moment/`、`guestbook/`、`oauth/`、`captcha/`、`avatar/`、`uv/`），较老的领域仍是根包下的扁平单文件（`user`、`category`、`tag`、`friendlink`、`social_auth`）。两种风格在同一层并存，违反 `go-layering` skill 明确约定的「一个层内风格统一，禁两者并存」，是当前代码「混乱、可读性差」的主要来源。

此外存在两类残留：
- **11 个空壳死文件**：上一次迁移未清理干净，内容仅 1 行 `package X`。
- **约 24 处 `interface{}` 字面量**（排除 gomock 生成文件），约定要求统一写 `any` 或具体类型/泛型。

### 已核实的现状基线
- 红线干净：无 `fmt.Println`、无全局基础设施变量、handler 未直接 import/返回 `model.*`。
- `go build ./...` 当前通过。
- DI 接线集中在 `internal/router/router.go` 的 `newRouteHandlers`；基础设施初始化在 `internal/bootstrap/bootstrap.go`。
- 扁平领域使用根包构造函数：`handler.NewXHandler` / `service.NewXService` / `repository.NewXRepository`。
- repository 接口 mock 位于 `internal/repository/mock/`（gomock 生成）；子包领域的 mock 位于各自 `<domain>/mock/`（如 `repository/article/mock/`）。
- `UserCacheService` 被跨层共享：`middleware/auth.go`、`service/auth/auth.go`、`service/oauth/oauth.go`、`router.go` 均引用 `service.UserCacheService`。

## 目标形态

每个领域在三层中均为独立子包，与现有 `article/`、`guestbook/`、`comment/` 完全一致：

```
internal/{handler,service,repository}/<domain>/
  <domain>.go    门面：构造函数 + 接口 + 对外类型 / sentinel error
  query.go       读操作实现
  mutation.go    写操作实现
  mapper.go      model ⇆ dto 转换（service 层）
  response.go    响应组装（handler 层，按需）
  mock/          gomock 生成的接口 mock（repository 层）
  <domain>_test.go
```

文件拆分粒度参照已迁移领域的实际情况，不强制每个文件都存在（小领域可只有门面 + 一两个实现文件）。

## 待处理清单（已核实）

| 类别 | 内容 |
|---|---|
| 死代码（11 个空壳文件） | handler 的 `comment/moment/media/message`；service 的 `comment/moment/media/message`；repository 的 `comment/moment/message` |
| 需迁移的扁平领域（5 个） | `user`（含 `user_cache`、`user_detail`、`string` 辅助文件归属）、`category`、`tag`、`friendlink`、`social_auth`（仅 repository 层） |
| `interface{}` → `any` | 生产代码约 24 处；gomock 生成文件通过重新生成处理 |

## 方案：按领域分阶段迁移

一次性改动 5 个领域 + 接线 + mock + 测试体量过大、难审查、出错难定位。采用分阶段方式，**每个阶段结束都必须 `go build ./...` 通过且该领域 `go test` 绿灯**，再进入下一阶段。风险可控、可随时停。

### 阶段顺序（低风险 → 高风险）

0. **死代码清理**：删除 11 个空壳文件 + `interface{}→any`。零行为变化。
1. **friendlink**（~1040 行，无跨域共享，作为流程模板验证）。
2. **category**。
3. **tag**（service 依赖 `articleSvc`，注意 `router` 接线顺序）。
4. **social_auth**（仅 repository 层，被 `service/oauth` 消费）。
5. **user**（最大、最复杂：`UserCacheService` 被 middleware/auth/oauth/router 共享，blast radius 最大，压轴）。

### 每个领域的迁移动作

1. 将扁平文件移入 `internal/<layer>/<domain>/` 子目录。
2. 修改 `package` 声明为 `<domain>`。
3. 抽出门面入口文件 `<domain>.go`：仅放对外构造函数、接口、公开类型、sentinel error。
4. 按职责把实现拆到 `query.go` / `mutation.go` / `mapper.go` / `response.go`。
5. 更新 `router.go` 接线：import 别名（如 `userservice`、`friendlinkrepo`）+ 构造函数调用。
6. repository 接口 mock 迁到子包 `<domain>/mock/` 并重新生成；更新 service 测试引用。
7. 测试文件改为 `<domain>_test` 外部包名并随领域迁移。
8. 跑 `go build ./...` + 该领域 `go test`。

### 跨层引用更新（user 阶段重点）

`service.UserCacheService` / `service.UserService` / `service.NewUserCacheService` 等根包符号迁移后变为 `userservice.*`，需同步更新：`middleware/auth.go`(+test)、`service/auth/auth.go`、`service/oauth/oauth.go`、`router.go`、`handler/user.go`(+test)。

## 约束（贯穿全程）

- **API 零变更优先**：路由路径、DTO 字段、行为默认不动。如发现 API 设计问题，逐项单独与用户确认后再改（用户已授权「允许顺带优化」）。
- 不手改 `docs/docs.go`（swag 生成）；接口变动后用 `make swag` 重新生成。
- gomock 生成文件不手改，重新生成。
- 每阶段 build + test 绿灯才进下一阶段。

## 验证策略

- 阶段级：`go build ./...` + 受影响领域 `go test ./internal/.../<domain>/...`。
- 全量收口：`go build ./...`、`go test ./...`、`make swag`（确认无 diff 或仅预期 diff）。
- 行为不变性：路由表与迁移前一致（路径、方法、中间件、handler 绑定逐条比对 `router.go`）。

## 不在本次范围

- 不做与布局统一无关的重构（业务逻辑改写、性能优化、依赖升级）。
- 不调整已是子包的领域（article/auth/comment/moment/guestbook/oauth/captcha/avatar/uv）。
- 数据库 schema、migrations 不动。
