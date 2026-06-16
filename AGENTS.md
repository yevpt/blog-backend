# Blog Backend — AI 编码规范

Go 1.25+ 个人博客后端 API，模块 `github.com/vpt/blog-backend`。

## 技术栈

Gin、GORM/MySQL、zap、viper、jwt/v5、bcrypt、go-redis、AWS SDK S3（Garage）、swaggo/swag。

## 输出控制

- 默认简洁；回复过长会直接报错，故长内容必须摘要、大任务分阶段。
- 优先改文件而非贴代码：只给关键片段/关键错误；未明确要求不输出完整文件 / diff / 日志 / lockfile / 构建产物。
- 每次只说：做了什么、改了什么、验证了什么、有何风险。

## 全局红线（任何时候都成立）

- 禁用全局变量保存 db、redis、logger、mailer 等基础设施；一律构造注入。
- 生产代码禁 `fmt.Println`，用 `zap.Logger`。
- 禁直接返回 `model.*` 给前端或写进 Swagger。

## 场景 skill（按需引入，详规在各 skill 内）

具体规范已拆进 skill，遇到对应场景时按 `description` 自动引入；源在 `.agents/skills/`，软链到 `.claude/skills/`，新增后执行 `make skills`。

- **git-commit** — 写 commit message 时。Conventional Commits + 中文主题，`commit-msg` 钩子强制校验。
- **go-layering** — 写/改 handler·service·repository 代码时。分层边界、依赖注入、接口 mock、包组织、`interface{}` 口径。
- **http-api** — 新增/改 HTTP 接口时。路由分组、`jwt.GetClaims`、`pkg/response`、Swagger 注解 + `make swag`。
- **go-readability** — 组织代码/重构/写注释时。入口门面、按职责拆文件、函数短小、中文注释规范。
- **go-testing** — 写/改测试时。sqlmock/gomock/httptest 分层用法、`_test` 包名、改接口跑相关测试。
