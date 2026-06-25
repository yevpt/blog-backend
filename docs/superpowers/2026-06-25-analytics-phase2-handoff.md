# 站点分析 Phase 2/3 交接文档（给 Codex 继续）

> 接手人：请先读这份文档，再读 Phase 2 计划 `docs/superpowers/plans/2026-06-25-analytics-phase2-backend.md`。本文档记录当前进度、踩过的坑、必须遵守的约定，以及剩余任务。

## 0. 一句话现状

- **Phase 1 后端 + 前端**：已全部完成并本地合并进 `dev`（前端仓 `blog-frontend` 同样已合并 `dev`）。
- **Phase 2 后端**：在分支 `feat/analytics-phase2`（从 `dev` 切出，基点 `b1cbd29`）。**Task 1–4 已完成并提交**，**剩 Task 5（回填接口）、Task 6（重生成 swag）**，之后做整分支最终评审 + 合并 `dev`。
- **Phase 3**：计划尚未编写（见第 5 节）。

## 1. 仓库与分支

- 后端：`/Volumes/External/SynologyDrive/Codes/Blog/blog-backend`，当前分支 `feat/analytics-phase2`。
- 前端：`/Volumes/External/SynologyDrive/Codes/Blog/blog-frontend`，当前分支 `dev`（Phase 1 前端已合入）。

Phase 2 已落提交（`git log --oneline`）：
```
b4e8f29 feat(analytics): 新增前台公开总览与热门页面接口     (Task 4)
bccdf98 feat(analytics): 新增后台维度分布接口               (Task 3)
3721f34 feat(analytics): 日聚合补全平均停留与跳出率         (Task 2)
82888f5 fix(analytics): 会话 PV 计数改为自增并重算跳出       (Task 1)
aac6118 docs(analytics): 新增 Phase 2 后端实现计划          (计划)
b1cbd29 (dev 基点) fix(analytics): 实时计数排除伪造来源事件
```

## 2. 执行流程（subagent-driven-development）

每个 Task 走「实现子代理 → 评审子代理」两步，全程账本记录在 `.superpowers/sdd/phase2-progress.md`（**注意是 phase2-progress.md，不要覆盖 Phase 1 的 progress.md**）。

工具脚本（已验证可用）：
- 取任务简报：`~/.claude/plugins/cache/claude-plugins-official/superpowers/6.0.3/skills/subagent-driven-development/scripts/task-brief <plan.md> <N>` → 生成 `.superpowers/sdd/task-N-brief.md`。
- 生成评审包：`.../scripts/review-package <baseSHA> <headSHA>` → 生成 diff 文件供评审子代理读。

**Task 5 的简报已生成**：`.superpowers/sdd/task-5-brief.md`（可直接派实现子代理）。

> 子代理模型：本环境要求用全限定名 `claude-opus-4-8-thinking-high`（不能用简称 `opus`，会报 Invalid model）。Codex 自行决定是否照此分派；若 Codex 自己实现也可，关键是遵守下面的约定。

## 3. 必须遵守的约定 / 我踩过的坑（重要）

1. **测试一律手写 fake，禁止 gomock。** analytics 各包没有生成式 mock。
   - repository 测试：用现成 helper `newRepo(t) (repo.Repository, sqlmock.Sqlmock)` + go-sqlmock，匹配器是**正则**（`regexp`/`regexp.QuoteMeta`）。写操作 `ExpectBegin/ExpectExec(<regexp>)/ExpectCommit`，读操作 `ExpectQuery`，结尾 `require.NoError(t, mock.ExpectationsWereMet())`。参考 `internal/repository/analytics/{repository,query}_test.go`。
   - service 测试：手写 fake（见 `internal/service/analytics/query_test.go` 的 `fakeQueryRepo`/`fakeRealtime`），同包 `analytics_test`，可直接复用；Redis 相关服务传 `rdb=nil` 让缓存分支空跑。
   - handler 测试：手写 fake service（见 `admin_test.go` 的 `fakeQuery`）+ `gin.TestMode` + 复用 `doGET`/`decode`，断言 `response.CodeOK`/`response.CodeBadRequest`（注意：本项目无 422，校验失败是 HTTP 200 + 包体 code=400）。
2. **sqlmock 正则要对齐真实 SQL。** 实现前写的正则可能和 GORM 渲染不一致；**先跑 RED，sqlmock 会打印实际 SQL**，据此调整正则再实现到 GREEN。例：会话自增渲染为 `` `pv_count`=pv_count + 1 ``。
3. **`AggregateDay` 的测试是「有序」期望。** 它依次发 daily + 6 维度 + page 查询；Task 2 又加了第 8 条 `analytics_sessions` 查询，所以**必须改原 `TestAggregateDay`**（在 `AggregateDay(...)` 调用前追加该期望 + 断言），否则报未满足/意外查询。
4. **service 层禁止 import worker（会循环依赖）。** `worker` 已经 import `service`（`session_adapter.go`）。Task 5 回填要复用 `worker.Rollup.RollupDay`，做法是**以函数值 `rollupDay func(context.Context, string) error` 注入** `BackfillService`，在 router 里 `backfillRollup := analyticsworker.NewRollup(repo, repo, log)` 再传 `backfillRollup.RollupDay`。RollupDay 幂等无状态，可与调度器的实例分开（**注意：ingestor 必须全局唯一，但 Rollup 不需要**）。
5. **`NewAdminHandler` 签名在 Task 5 会变。** Task 5 给它加第二个参数 `svc.BackfillService`，**必须同步改 `admin_test.go` 里所有 `hdl.NewAdminHandler(...)` 调用点**（传 `&fakeBackfill{}`）和 router 里的真实装配。`newRouter` 也要加 `POST /admin/analytics/backfill` 路由。
6. **`shanghaiTZ()` 全包只能有一个定义。** 已在 Task 4 的 `service/analytics/public.go` 定义；Task 5 的 `backfill.go` 直接复用，别再定义。
7. **commit-msg 钩子强制 Conventional Commits + 中文主题。** 别用 `--no-verify`。前端仓的 pre-commit 钩子会跑 lint（约 15s）。
8. **`make swag` 在本沙箱未安装 `swag`。** Task 3/4 只写了 swagger 注解、没重生成 `docs/`。Task 6 统一重生成；若环境仍无 swag，就保留注解、记录待维护者补跑，不要卡住。
9. **隐私红线（公开接口）**：只暴露聚合数字，热门页面 SQL 层 `NOT LIKE '/admin/%'` 排除后台；公开路由放 `registerPublicRoutes` + `RateLimitNormal`，**不要**放进 admin 组。
10. **部署侧（跨仓，别忘）**：后端环境变量 `ANALYTICS_ALLOWED_ORIGINS` 必须列出 web 源、排除 admin，否则所有 PV 会被标 suspect 而不计数。

## 4. 剩余 Phase 2 任务

### Task 5：后台回填接口（简报已生成 `.superpowers/sdd/task-5-brief.md`）
新增 `POST /admin/analytics/backfill?from=&to=`（管理员）：
- `service/analytics/backfill.go`：`BackfillService.Backfill(ctx, from, to) (days, err)`，注入 `rollupDay` 函数，逐日（含端点、Asia/Shanghai）调用，遇错即停返回已完成天数。
- `dto.BackfillResult{From,To,Days}`。
- `AdminHandler` 加 `backfill` 字段 + 改 `NewAdminHandler` 签名（见坑 #5）；`parseRequiredRange`（from/to 必填 + 跨度上限 `maxBackfillDays=92`）。
- router：构造专用 `backfillRollup`（见坑 #4）+ 注册 admin 路由。
- 测试：service 用注入函数验证「闭区间循环」「遇错即停」；handler 验证 happy / 缺参 400 / 超跨度 400。
- 提交信息：`feat(analytics): 新增后台日聚合回填接口`。

完成后：`review-package <b4e8f29> <Task5SHA>` → 评审子代理（重点核：无 import cycle、签名改动同步、闭区间、跨度上限）。

### Task 6：重生成 Swagger
读 `Makefile` 的 `swag` 目标，跑 `make swag`；`go build ./... && go test ./...` 全绿；`docs/` 有变更才提交 `docs(analytics): 重新生成 Phase 2 接口 Swagger`。

### Phase 2 收尾
- 最终整分支评审：`review-package $(git merge-base dev HEAD) HEAD`，派一个资深评审子代理（隐私/鉴权/分层/各不变量）。
- 决定合并：用户对 Phase 1 的选择是「本地快进合并回 dev」。Phase 2 建议同样：`git checkout dev && git merge --ff-only feat/analytics-phase2`，跑 `go build ./... && go test ./...` 验证后删分支。**合并前请向用户确认**。

## 5. Phase 3（尚未写计划）

依据原始交接文档 `docs/superpowers/2026-06-25-analytics-handoff.md` 第 7 节。预期为「健壮性/反作弊」一类（headless 爬虫识别、路径漏斗、签名令牌防伪造上报等——以该文件第 7 节为准）。流程：先 `writing-plans` 写 `docs/superpowers/plans/<date>-analytics-phase3-*.md`，再按 subagent-driven 执行。**写计划前务必先精读相关现有代码并核对真实约定**（本次最大教训：第一版 Phase 2 计划误用 gomock，核对后才发现全仓是手写 fake——见坑 #1）。

## 6. 关键文件地图（Phase 2 触达）

- 模型：`internal/model/analytics.go`（`AnalyticsSession`/`AnalyticsDaily`/`AnalyticsDailyDim`/`AnalyticsPageDaily`）。
- 仓储：`internal/repository/analytics/{repository,query}.go`（`AggregateDay`/`eventScope`/`sessionScope`/`QueryDimRange`/`QueryTopPagesPublic`/`QueryTotalsSegmented`/`dayRangeUTC`）。
- 服务：`internal/service/analytics/{query,public,collect,realtime}.go`（+ 待建 `backfill.go`）。
- 处理器：`internal/handler/analytics/{admin,public,collect}.go`。
- 路由装配：`internal/router/router.go`（`newAnalyticsCollectHandler`、`routeHandlers`、`registerPublicRoutes`、`registerAdminRoutes`、`AnalyticsRuntime`）。
- worker：`internal/worker/analytics/{rollup,scheduler,ingest,session_adapter}.go`；启动 `internal/bootstrap/bootstrap.go` `StartAnalyticsWorker`。
- 配置：`pkg/config/config.go` `AnalyticsConfig`（含 `PublicCacheTTL`、`Timezone`、`OnlineWindow` 等）。
- 账本/简报：`.superpowers/sdd/phase2-progress.md`、`.superpowers/sdd/task-*-brief.md|report.md`。

## 7. 验证基线

每个 Task 提交前：`go build ./... && go test ./internal/repository/analytics/... ./internal/service/analytics/... ./internal/handler/analytics/...`。收尾用 `go test ./...`（注意 `grep -Ev '^ok '` 在全通过时会以 rc=1 退出，是 grep 行为不是测试失败，改用 `go test ./... > /tmp/out 2>&1; echo exit=$?` 判断）。
