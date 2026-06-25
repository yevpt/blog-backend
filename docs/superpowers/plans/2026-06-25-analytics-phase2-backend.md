# Analytics Phase 2 Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refine session metrics (fix R2 pv_count + R3 avg_duration/bounce_rate), add the admin dimensions endpoint, add unauthenticated public summary/popular APIs (aggregate-only, cached), and add an admin backfill endpoint that re-runs day rollups over a date range.

**Architecture:** Builds on the merged Phase 1 backend. Session pv_count becomes a true increment via a GORM `ON DUPLICATE KEY UPDATE` expression; `AggregateDay` gains a session-level pass that fills `avg_duration`/`bounce_rate` into `analytics_daily`. New read endpoints reuse the existing repo/service/handler layering and `pkg/response`; public endpoints add a thin Redis short-TTL cache and exclude `/admin/*` paths. Backfill reuses the existing idempotent `worker.Rollup.RollupDay` injected as a function value (service layer must NOT import the worker package — `worker` already imports `service`, so the reverse would be a cycle).

**Tech Stack:** Go 1.25, Gin, GORM/MySQL, go-redis, zap, swaggo/swag.

## Global Constraints

- **Layering (red line):** handler → service → repository; inject via constructors; NO global infra vars; NO `fmt.Println` (use zap). NEVER return `model.*` to frontend/Swagger — map to `dto.*`.
- **No import cycle:** `internal/service/analytics` MUST NOT import `internal/worker/analytics` (worker imports service via `session_adapter.go`). Backfill injects `rollupDay func(context.Context, string) error`.
- **Response convention:** success `response.Success(c, data)`; validation failure `response.Fail(c, response.CodeBadRequest, msg)` (HTTP 200, envelope code=400 — project has NO 422); server error `response.ServerError(c)`. Success envelope code is `response.CodeOK`. Mirror `internal/handler/analytics/admin.go`.
- **TESTING CONVENTIONS (this repo's analytics packages use HAND-WRITTEN FAKES, NOT gomock):**
  - **Repository tests** (`package analytics_test` in `internal/repository/analytics/*_test.go`): use the existing `newRepo(t) (repo.Repository, sqlmock.Sqlmock)` helper and go-sqlmock with `regexp`/`regexp.QuoteMeta` matchers (sqlmock default matcher is regexp). Writes (INSERT/upsert) → `mock.ExpectBegin()` + `mock.ExpectExec(<regexp>)` + `mock.ExpectCommit()`. Reads → `mock.ExpectQuery(<regexp>)`. Always end with `require.NoError(t, mock.ExpectationsWereMet())`.
  - **Service tests** (`package analytics_test` in `internal/service/analytics/*_test.go`): use hand-written fakes (see `fakeQueryRepo`, `fakeRealtime` in `query_test.go`). NEVER introduce gomock. For Redis-backed services, pass `rdb = nil` so the cache path no-ops.
  - **Handler tests** (`package analytics_test` in `internal/handler/analytics/admin_test.go`): hand-written fake services (see `fakeQuery`), `gin.SetMode(gin.TestMode)`, build a `gin.New()` router, use the existing `doGET`/`decode` helpers and assert `response.CodeOK`/`response.CodeBadRequest`.
  - There are NO generated mock files for the analytics packages. Do NOT run mockgen for these.
- **Aggregation day boundary:** Asia/Shanghai half-open `[start,end)` in UTC via `dayRangeUTC` (already in `repository/analytics/query.go`). Event scope filters `is_bot=0 AND is_suspect=0`. Sessions have `is_bot` (no `is_suspect` column) — filter `is_bot=0` only.
- **MySQL-only SQL** is acceptable; add a comment where `TIMESTAMPDIFF`/bool arithmetic is relied on.
- **Privacy red line (public APIs):** public endpoints expose ONLY aggregate numbers. NEVER a single visitor/user_id/IP; EXCLUDE `/admin/*` paths from popular pages. No per-user rows.
- **Public cache + abuse:** public endpoints cache response JSON in Redis with TTL = `cfg.Analytics.PublicCacheTTL` (already in config), and register with `middleware.RateLimitNormal(redisClient)`.
- **Admin auth:** admin endpoints register under the existing `admin := r.Group("/admin", middleware.Auth(...), middleware.RequireRole(roles.AdminRole))` group in `registerAdminRoutes`.
- **Swagger:** every new endpoint gets swaggo annotations matching `admin.go`; run `make swag` once at the end. Swagger `data=` types must be `dto.*`.
- **Branch & commits:** all work on `feat/analytics-phase2` (cut from `dev`). Conventional Commits + Chinese subject; `commit-msg` hook enforces format. Run `go build ./... && go test ./...` before each commit.

---

## File Structure

- Modify `internal/repository/analytics/repository.go` — `UpsertSession` increment; add `QueryTopPagesPublic` + `QueryTotalsSegmented` (+ interface entries).
- Modify `internal/repository/analytics/query.go` — `AggregateDay` session pass; add the two new query impls + a `sessionScope` helper.
- Modify `internal/service/analytics/collect.go` — `sessionFrom` sets `IsBounce=true`.
- Modify `internal/service/analytics/query.go` — `QueryReader` + `QueryService` gain `Dimensions`.
- New `internal/service/analytics/public.go` — `PublicService` (summary/popular + Redis cache) + `shanghaiTZ()`.
- New `internal/service/analytics/backfill.go` — `BackfillService` (loops injected rollupDay).
- Modify `internal/dto/analytics/analytics.go` — add `DimensionPoint`, `PublicSummary`, `PublicPageStat`, `BackfillResult`.
- Modify `internal/handler/analytics/admin.go` — add `Dimensions` + `Backfill` handlers; `AdminHandler` gains a `BackfillService`.
- New `internal/handler/analytics/public.go` — `PublicHandler`.
- Modify `internal/router/router.go` — wire new services/handlers + routes; construct a backfill `Rollup` + `PublicService`/`PublicHandler`.
- Tests: colocated `_test.go` (extend existing files).

---

## Task 1: R2 — session pv_count true increment

**Files:** `internal/repository/analytics/repository.go`, `internal/service/analytics/collect.go`, test `internal/repository/analytics/repository_test.go`.

**Behavior:** INSERT a fresh session with `pv_count=1, is_bounce=true`; on conflict (same session_id) `pv_count = pv_count + 1`, `is_bounce = false`, refresh `last_seen`/`exit_path`, recompute `duration = TIMESTAMPDIFF(SECOND, first_seen, <last_seen>)`, update `user_id`/`is_authenticated`.

- [ ] **Step 1: Mark a fresh session as a bounce**

In `internal/service/analytics/collect.go`, find `sessionFrom` (it builds `model.AnalyticsSession` with `PVCount: 1`) and add `IsBounce: true` to the returned struct literal (a single-PV session is a bounce until a second PV arrives). Change nothing else.

- [ ] **Step 2: Write the failing repository test**

In `internal/repository/analytics/repository_test.go`, add (mirroring `TestUpsertDaily`):

```go
func TestUpsertSession_IncrementsPVCount(t *testing.T) {
	r, mock := newRepo(t)
	mock.ExpectBegin()
	// 冲突分支必须自增 pv_count（而非覆盖）。正则匹配渲染后的 ON DUPLICATE KEY UPDATE。
	mock.ExpectExec("ON DUPLICATE KEY UPDATE.*pv_count.*pv_count \\+ 1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := r.UpsertSession(context.Background(), model.AnalyticsSession{
		SessionID: "s1", VisitorID: "v1", PVCount: 1, IsBounce: true,
		FirstSeen: time.Unix(1000, 0), LastSeen: time.Unix(1100, 0),
		EntryPath: "/a", ExitPath: "/b",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
```

Add `"time"` to the test imports if missing.

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/repository/analytics/ -run TestUpsertSession_IncrementsPVCount -v`
Expected: FAIL — current `UpsertSession` uses `AssignmentColumns` (overwrites pv_count), so the increment regex won't match. The sqlmock failure message prints the ACTUAL rendered SQL — note it; if the real increment renders slightly differently (e.g. backtick-quoted `` `pv_count` ``), adjust the regex in Step 2 to match the actual SQL after Step 4.

- [ ] **Step 4: Implement the increment**

Replace the `DoUpdates` in `UpsertSession` (repository.go). `gorm` and `gorm/clause` are already imported:

```go
func (r *repository) UpsertSession(ctx context.Context, s model.AnalyticsSession) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			// 同一会话再来一次 PV：累计计数、刷新末路径/时间，并据此重算停留与跳出。
			"pv_count":         gorm.Expr("pv_count + 1"),
			"is_bounce":        false, // 出现第二次 PV，不再算跳出
			"last_seen":        s.LastSeen,
			"exit_path":        s.ExitPath,
			"duration":         gorm.Expr("TIMESTAMPDIFF(SECOND, first_seen, ?)", s.LastSeen),
			"user_id":          s.UserID,
			"is_authenticated": s.IsAuthenticated,
		}),
	}).Create(&s).Error
	if err != nil {
		return fmt.Errorf("会话 upsert 失败: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run repository tests**

Run: `go test ./internal/repository/analytics/ -v` → PASS (new + existing).

- [ ] **Step 6: Build + test + commit**

Run: `go build ./... && go test ./internal/repository/analytics/... ./internal/service/analytics/...`

```bash
git add internal/repository/analytics/repository.go internal/repository/analytics/repository_test.go internal/service/analytics/collect.go
git commit -m "fix(analytics): 会话 PV 计数改为自增并重算跳出"
```

---

## Task 2: R3 — avg_duration & bounce_rate in AggregateDay

**Files:** `internal/repository/analytics/query.go`, test `internal/repository/analytics/query_test.go`.

**Behavior:** `AggregateDay` now also fills `agg.Daily.AvgDuration` (int seconds, rounded) and `agg.Daily.BounceRate` (float64 in [0,1]) from `analytics_sessions` whose `first_seen ∈ [start,end)` and `is_bot=0`, as a FINAL query after the page GROUP BY.

- [ ] **Step 1: Update the existing TestAggregateDay (it will otherwise fail on an unexpected query)**

In `query_test.go`, `TestAggregateDay` currently expects daily + 6 dim + page queries. Append ONE more expectation (the session query) right before the `AggregateDay` call, and add assertions:

```go
	// 会话级指标查询（最后一条）
	mock.ExpectQuery(regexp.QuoteMeta("FROM `analytics_sessions`")).
		WillReturnRows(sqlmock.NewRows([]string{"avg_duration", "bounce_rate"}).AddRow(42.0, 0.25))
```

After the existing assertions add:

```go
	assert.Equal(t, 42, got.Daily.AvgDuration)
	assert.Equal(t, 0.25, got.Daily.BounceRate)
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/repository/analytics/ -run TestAggregateDay -v`
Expected: FAIL — AggregateDay does not yet issue the session query (sqlmock reports the session expectation unmet) and leaves AvgDuration/BounceRate at 0.

- [ ] **Step 3: Implement the session pass**

Add a helper near `eventScope` in `query.go`:

```go
// sessionScope 返回「当日开始的真人会话」过滤 builder（first_seen 落在 [start,end)，非 bot）。
func (r *repository) sessionScope(ctx context.Context, start, end time.Time) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.AnalyticsSession{}).
		Where("first_seen >= ? AND first_seen < ?", start, end).
		Where("is_bot = ?", false)
}
```

At the END of `AggregateDay` (after the page GROUP BY loop, before `return agg, nil`), add — and update the function's `TODO(Phase2)` doc comment to state it's now computed:

```go
	// 4) 会话级指标：平均停留时长与跳出率（来自 analytics_sessions，当天开始的真人会话）。
	var sess struct {
		AvgDuration float64
		BounceRate  float64
	}
	sessSelect := "COALESCE(AVG(duration), 0) as avg_duration, " +
		"COALESCE(SUM(CASE WHEN is_bounce THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 0) as bounce_rate"
	if e := r.sessionScope(ctx, start, end).Select(sessSelect).Scan(&sess).Error; e != nil {
		return DayAggregate{}, fmt.Errorf("聚合会话指标失败: %w", e)
	}
	agg.Daily.AvgDuration = int(sess.AvgDuration + 0.5) // 四舍五入到秒
	agg.Daily.BounceRate = sess.BounceRate
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/repository/analytics/ -v` → PASS.

- [ ] **Step 5: Build + commit**

Run: `go build ./... && go test ./internal/repository/analytics/...`

```bash
git add internal/repository/analytics/query.go internal/repository/analytics/query_test.go
git commit -m "feat(analytics): 日聚合补全平均停留与跳出率"
```

---

## Task 3: Admin dimensions endpoint

**Files:** `internal/dto/analytics/analytics.go`, `internal/service/analytics/query.go`, `internal/handler/analytics/admin.go`, `internal/router/router.go`; tests `internal/service/analytics/query_test.go`, `internal/handler/analytics/admin_test.go`.

**Interfaces:** add `QueryDimRange` to the `QueryReader` interface (repository already implements it); add `QueryService.Dimensions(ctx, dimension, from, to) ([]dto.DimensionPoint, error)`; route `GET /admin/analytics/dimensions?dimension=&from=&to=`.

- [ ] **Step 1: Add the DTO**

In `internal/dto/analytics/analytics.go`:

```go
// DimensionPoint 维度分布单项：某日某维度取值的 PV/UV。
type DimensionPoint struct {
	Date     string `json:"date"`
	DimValue string `json:"dim_value"`
	PV       int    `json:"pv"`
	UV       int    `json:"uv"`
}
```

- [ ] **Step 2: Write the failing service test**

In `internal/service/analytics/query_test.go`: (a) add a `QueryDimRange` method to the existing `fakeQueryRepo` so it keeps satisfying `QueryReader`; (b) add a `dim` field to capture; (c) add a test.

```go
// add field to fakeQueryRepo:
//   dim []model.AnalyticsDailyDim
func (f *fakeQueryRepo) QueryDimRange(_ context.Context, _, _, _ string) ([]model.AnalyticsDailyDim, error) {
	return f.dim, nil
}

func TestDimensions(t *testing.T) {
	r := &fakeQueryRepo{dim: []model.AnalyticsDailyDim{
		{Date: "2026-06-01", Dimension: "device", DimValue: "desktop", PV: 10, UV: 5},
	}}
	got, err := newQuerySvc(r, &fakeRealtime{}).Dimensions(context.Background(), "device", "2026-06-01", "2026-06-07")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "desktop", got[0].DimValue)
	assert.Equal(t, 10, got[0].PV)
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/service/analytics/ -run TestDimensions -v` → FAIL (`Dimensions` undefined; `fakeQueryRepo` missing method until you add it — add it in the same edit).

- [ ] **Step 4: Implement service method + extend QueryReader**

In `query.go` add to the `QueryReader` interface (keep the other entries EXACTLY as-is):

```go
	QueryDimRange(ctx context.Context, dimension, from, to string) ([]model.AnalyticsDailyDim, error)
```

Add to the `QueryService` interface:

```go
	Dimensions(ctx context.Context, dimension, from, to string) ([]dto.DimensionPoint, error)
```

Implement:

```go
// Dimensions 读取某维度在区间内的逐日分布，映射为 dto.DimensionPoint。
func (s *queryService) Dimensions(ctx context.Context, dimension, from, to string) ([]dto.DimensionPoint, error) {
	rows, err := s.repo.QueryDimRange(ctx, dimension, from, to)
	if err != nil {
		return nil, fmt.Errorf("读取维度分布失败: %w", err)
	}
	out := make([]dto.DimensionPoint, 0, len(rows))
	for _, d := range rows {
		out = append(out, dto.DimensionPoint{Date: d.Date, DimValue: d.DimValue, PV: d.PV, UV: d.UV})
	}
	return out, nil
}
```

- [ ] **Step 5: Run service tests**

Run: `go test ./internal/service/analytics/ -v` → PASS (the `var _ svc.QueryReader = (repo.Repository)(nil)` check still holds; repo already has `QueryDimRange`).

- [ ] **Step 6: Add the handler + dimension whitelist + swagger**

In `admin.go`, near `validMetrics`:

```go
var validDimensions = map[string]struct{}{
	"referer_type": {}, "device": {}, "browser": {}, "os": {}, "country": {}, "user_type": {},
}
```

```go
// Dimensions 维度分布：某维度在区间内逐日的 PV/UV。
// @Summary  站点维度分布
// @Tags     analytics
// @Produce  json
// @Param    dimension query string true  "维度：referer_type、device、browser、os、country、user_type"
// @Param    from      query string false "起始日期 YYYY-MM-DD，默认近 7 天"
// @Param    to        query string false "结束日期 YYYY-MM-DD，默认今天"
// @Success  200 {object} response.Response{data=[]dto.DimensionPoint} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/analytics/dimensions [get]
func (h *AdminHandler) Dimensions(c *gin.Context) {
	dimension := c.Query("dimension")
	if _, ok := validDimensions[dimension]; !ok {
		response.Fail(c, response.CodeBadRequest, "dimension 仅支持 referer_type、device、browser、os、country、user_type")
		return
	}
	from, to, ok := parseRange(c)
	if !ok {
		return
	}
	data, err := h.svc.Dimensions(c.Request.Context(), dimension, from, to)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}
```

- [ ] **Step 7: Handler test**

In `admin_test.go`: add a `Dimensions` method to the existing `fakeQuery` (capture `lastDimension`) and a route in `newRouter`, then tests for happy-path + invalid/missing dimension:

```go
// fakeQuery additions:
//   lastDimension string
func (f *fakeQuery) Dimensions(_ context.Context, dimension, from, to string) ([]dto.DimensionPoint, error) {
	f.lastDimension, f.lastFrom, f.lastTo = dimension, from, to
	return []dto.DimensionPoint{{Date: "2026-06-01", DimValue: "desktop", PV: 10, UV: 5}}, nil
}
// newRouter additions:
//   r.GET("/admin/analytics/dimensions", h.Dimensions)

func TestAdminDimensions_HappyPath(t *testing.T) {
	fq := &fakeQuery{}
	r := newRouter(hdl.NewAdminHandler(fq)) // NOTE: if Task 5 already added the backfill arg, pass &fakeBackfill{} too
	w := doGET(r, "/admin/analytics/dimensions?dimension=device")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeOK, decode(t, w).Code)
	assert.Equal(t, "device", fq.lastDimension)
}

func TestAdminDimensions_InvalidDimension(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}))
	w := doGET(r, "/admin/analytics/dimensions?dimension=bogus")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}
```

- [ ] **Step 8: Register the route**

In `registerAdminRoutes` (router.go), after the existing analytics routes:

```go
	admin.GET("/analytics/dimensions", handlers.analyticsAdmin.Dimensions)
```

- [ ] **Step 9: Build + test + commit**

Run: `go build ./... && go test ./internal/service/analytics/... ./internal/handler/analytics/...`

```bash
git add internal/dto/analytics internal/service/analytics/query.go internal/service/analytics/query_test.go internal/handler/analytics/admin.go internal/handler/analytics/admin_test.go internal/router/router.go
git commit -m "feat(analytics): 新增后台维度分布接口"
```

---

## Task 4: Public summary & popular APIs (cached, aggregate-only)

**Files:** `internal/repository/analytics/query.go` + `repository.go` (two new methods + interface), new `internal/service/analytics/public.go`, `internal/dto/analytics/analytics.go`, new `internal/handler/analytics/public.go`, `internal/router/router.go`; tests `internal/repository/analytics/query_test.go`, new `internal/service/analytics/public_test.go`, new `internal/handler/analytics/public_test.go`.

- [ ] **Step 1: Add DTOs**

```go
// PublicSummary 前台公开总览：仅聚合数字。
type PublicSummary struct {
	TodayPV      int64 `json:"today_pv"`
	TodayUV      int64 `json:"today_uv"`
	Online       int64 `json:"online"`
	TotalPV      int64 `json:"total_pv"`
	TotalUV      int64 `json:"total_uv"`
	RegisteredUV int64 `json:"registered_uv"`
	AnonymousUV  int64 `json:"anonymous_uv"`
}

// PublicPageStat 前台热门页面项（仅 path/title/pv）。
type PublicPageStat struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	PV    int    `json:"pv"`
}
```

- [ ] **Step 2: Failing repository tests**

In `query_test.go` add (mirror `TestQueryTopPages`/`TestQueryTotals`):

```go
func TestQueryTopPagesPublic_ExcludesAdmin(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"path", "title", "pv", "uv"}).AddRow("/a", "A", 30, 10)
	// 必须带 path NOT LIKE 排除 /admin/*
	mock.ExpectQuery("analytics_page_daily.*NOT LIKE").WillReturnRows(rows)
	got, err := r.QueryTopPagesPublic(context.Background(), "2026-06-01", "2026-06-30", 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "/a", got[0].Path)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQueryTotalsSegmented(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"total", "registered", "anonymous"}).
		AddRow(int64(40), int64(10), int64(30))
	mock.ExpectQuery(regexp.QuoteMeta("FROM `analytics_daily`")).WillReturnRows(rows)
	total, reg, anon, err := r.QueryTotalsSegmented(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(40), total)
	assert.Equal(t, int64(10), reg)
	assert.Equal(t, int64(30), anon)
	require.NoError(t, mock.ExpectationsWereMet())
}
```

> The default sqlmock matcher is regexp; `"analytics_page_daily.*NOT LIKE"` matches the rendered SELECT. If the red run shows the WHERE renders differently, adjust the pattern to the actual SQL.

- [ ] **Step 3: Run to verify fail**

Run: `go test ./internal/repository/analytics/ -run 'Public|Segmented' -v` → FAIL (methods undefined).

- [ ] **Step 4: Implement repository methods + interface**

In `query.go`:

```go
// QueryTopPagesPublic 同 QueryTopPages，但排除 /admin/* 路径，供前台公开榜单使用。
func (r *repository) QueryTopPagesPublic(ctx context.Context, from, to string, limit int) ([]model.AnalyticsPageDaily, error) {
	var out []model.AnalyticsPageDaily
	err := r.db.WithContext(ctx).
		Model(&model.AnalyticsPageDaily{}).
		Select("path, max(title) as title, sum(pv) as pv, sum(uv) as uv").
		Where("date >= ? AND date <= ?", from, to).
		Where("path NOT LIKE ?", "/admin/%").
		Group("path").Order("pv desc").Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("查询前台热门页面失败: %w", err)
	}
	return out, nil
}

// QueryTotalsSegmented 汇总累计 UV 及注册/匿名 UV（来自 analytics_daily）。
func (r *repository) QueryTotalsSegmented(ctx context.Context) (total, registered, anonymous int64, err error) {
	var row struct{ Total, Registered, Anonymous int64 }
	e := r.db.WithContext(ctx).
		Model(&model.AnalyticsDaily{}).
		Select("COALESCE(SUM(uv),0) as total, COALESCE(SUM(registered_uv),0) as registered, COALESCE(SUM(anonymous_uv),0) as anonymous").
		Scan(&row).Error
	if e != nil {
		return 0, 0, 0, fmt.Errorf("查询分档累计失败: %w", e)
	}
	return row.Total, row.Registered, row.Anonymous, nil
}
```

Add both to the `Repository` interface (repository.go) in the read-methods block.

- [ ] **Step 5: Implement PublicService (new public.go)**

```go
package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	"github.com/vpt/blog-backend/internal/model"
	"go.uber.org/zap"
)

// PublicReader 抽象前台公开统计所需的 repo 方法。
type PublicReader interface {
	QueryTotals(ctx context.Context) (pv, uv int64, err error)
	QueryTotalsSegmented(ctx context.Context) (total, registered, anonymous int64, err error)
	QueryTopPagesPublic(ctx context.Context, from, to string, limit int) ([]model.AnalyticsPageDaily, error)
}

// PublicService 提供前台公开聚合数据（仅聚合数字），结果走 Redis 短 TTL 缓存。
type PublicService interface {
	Summary(ctx context.Context) (dto.PublicSummary, error)
	Popular(ctx context.Context, limit int) ([]dto.PublicPageStat, error)
}

type publicService struct {
	repo     PublicReader
	realtime Realtime
	rdb      *redis.Client
	cacheTTL time.Duration
	logger   *zap.Logger
}

func NewPublicService(repo PublicReader, realtime Realtime, rdb *redis.Client, cacheTTL time.Duration, logger *zap.Logger) PublicService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &publicService{repo: repo, realtime: realtime, rdb: rdb, cacheTTL: cacheTTL, logger: logger}
}

const (
	publicSummaryKey = "analytics:public:summary"
	publicPopularKey = "analytics:public:popular:" // + limit
)

func (s *publicService) getCached(ctx context.Context, key string, out any) bool {
	if s.rdb == nil {
		return false
	}
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func (s *publicService) setCached(ctx context.Context, key string, v any) {
	if s.rdb == nil {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := s.rdb.Set(ctx, key, raw, s.cacheTTL).Err(); err != nil {
		s.logger.Warn("公开统计缓存写入失败", zap.String("key", key), zap.Error(err))
	}
}

func (s *publicService) Summary(ctx context.Context) (dto.PublicSummary, error) {
	var out dto.PublicSummary
	if s.getCached(ctx, publicSummaryKey, &out) {
		return out, nil
	}
	today, err := s.realtime.TodayCounters(ctx)
	if err != nil {
		return dto.PublicSummary{}, fmt.Errorf("读取今日计数失败: %w", err)
	}
	online, err := s.realtime.OnlineCount(ctx)
	if err != nil {
		return dto.PublicSummary{}, fmt.Errorf("读取在线人数失败: %w", err)
	}
	totalPV, _, err := s.repo.QueryTotals(ctx)
	if err != nil {
		return dto.PublicSummary{}, fmt.Errorf("读取累计统计失败: %w", err)
	}
	totalUV, regUV, anonUV, err := s.repo.QueryTotalsSegmented(ctx)
	if err != nil {
		return dto.PublicSummary{}, fmt.Errorf("读取分档累计失败: %w", err)
	}
	out = dto.PublicSummary{
		TodayPV: today.PV, TodayUV: today.UV, Online: online,
		TotalPV: totalPV, TotalUV: totalUV, RegisteredUV: regUV, AnonymousUV: anonUV,
	}
	s.setCached(ctx, publicSummaryKey, out)
	return out, nil
}

func (s *publicService) Popular(ctx context.Context, limit int) ([]dto.PublicPageStat, error) {
	key := fmt.Sprintf("%s%d", publicPopularKey, limit)
	var out []dto.PublicPageStat
	if s.getCached(ctx, key, &out) {
		return out, nil
	}
	tz := shanghaiTZ()
	now := time.Now().In(tz)
	to := now.Format("2006-01-02")
	from := now.AddDate(0, 0, -29).Format("2006-01-02") // 近 30 天
	rows, err := s.repo.QueryTopPagesPublic(ctx, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("读取前台热门页面失败: %w", err)
	}
	out = make([]dto.PublicPageStat, 0, len(rows))
	for _, p := range rows {
		out = append(out, dto.PublicPageStat{Path: p.Path, Title: p.Title, PV: p.PV})
	}
	s.setCached(ctx, key, out)
	return out, nil
}

// shanghaiTZ 与聚合口径一致的东八区（加载失败回退固定时区）。
func shanghaiTZ() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}
```

- [ ] **Step 6: Failing service test (hand-written fakes, rdb=nil)**

New `internal/service/analytics/public_test.go`:

```go
package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"go.uber.org/zap"
)

type fakePublicRepo struct {
	totalPV          int64
	total, reg, anon int64
	pages            []model.AnalyticsPageDaily
	lastLimit        int
}

func (f *fakePublicRepo) QueryTotals(context.Context) (int64, int64, error) {
	return f.totalPV, 0, nil
}
func (f *fakePublicRepo) QueryTotalsSegmented(context.Context) (int64, int64, int64, error) {
	return f.total, f.reg, f.anon, nil
}
func (f *fakePublicRepo) QueryTopPagesPublic(_ context.Context, _, _ string, limit int) ([]model.AnalyticsPageDaily, error) {
	f.lastLimit = limit
	return f.pages, nil
}

func TestPublicSummary_NilRedis(t *testing.T) {
	r := &fakePublicRepo{totalPV: 100, total: 40, reg: 10, anon: 30}
	rt := &fakeRealtime{today: svc.TodayStat{PV: 5, UV: 3}, online: 2}
	s := svc.NewPublicService(r, rt, nil, time.Minute, zap.NewNop())
	out, err := s.Summary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(5), out.TodayPV)
	assert.Equal(t, int64(2), out.Online)
	assert.Equal(t, int64(100), out.TotalPV)
	assert.Equal(t, int64(40), out.TotalUV)
	assert.Equal(t, int64(10), out.RegisteredUV)
	assert.Equal(t, int64(30), out.AnonymousUV)
}

func TestPublicPopular_MapsAndDelegates(t *testing.T) {
	r := &fakePublicRepo{pages: []model.AnalyticsPageDaily{{Path: "/a", Title: "A", PV: 9, UV: 4}}}
	s := svc.NewPublicService(r, &fakeRealtime{}, nil, time.Minute, zap.NewNop())
	out, err := s.Popular(context.Background(), 7)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "/a", out[0].Path)
	assert.Equal(t, 9, out[0].PV)
	assert.Equal(t, 7, r.lastLimit)
}
```

> `fakeRealtime` already exists in `query_test.go` (same `analytics_test` package) — reuse it; do NOT redefine.

- [ ] **Step 7: Run service tests (red → green)**

Run: `go test ./internal/service/analytics/ -run 'Public' -v` (FAIL before Step 5 impl, PASS after). Then full `go test ./internal/service/analytics/ -v`.

- [ ] **Step 8: Public handler + swagger (new public.go)**

```go
package analytics

import (
	"strconv"

	"github.com/gin-gonic/gin"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"github.com/vpt/blog-backend/pkg/response"
)

const (
	publicDefaultPopular = 10
	publicMaxPopular     = 20
)

// PublicHandler 前台公开统计入口（无需登录，仅聚合数字）。
type PublicHandler struct {
	svc svc.PublicService
}

func NewPublicHandler(s svc.PublicService) *PublicHandler { return &PublicHandler{svc: s} }

// Summary 前台公开总览。
// @Summary  前台站点统计总览（公开）
// @Tags     analytics
// @Produce  json
// @Success  200 {object} response.Response{data=dto.PublicSummary} "统一响应；code=0 成功"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /analytics/public/summary [get]
func (h *PublicHandler) Summary(c *gin.Context) {
	data, err := h.svc.Summary(c.Request.Context())
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}

// Popular 前台热门页面榜（公开，排除 /admin/*）。
// @Summary  前台热门页面（公开）
// @Tags     analytics
// @Produce  json
// @Param    limit query int false "返回条数，默认 10，上限 20"
// @Success  200 {object} response.Response{data=[]dto.PublicPageStat} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /analytics/public/popular [get]
func (h *PublicHandler) Popular(c *gin.Context) {
	limit := publicDefaultPopular
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.Fail(c, response.CodeBadRequest, "limit 必须是大于 0 的整数")
			return
		}
		limit = n
	}
	if limit > publicMaxPopular {
		limit = publicMaxPopular
	}
	data, err := h.svc.Popular(c.Request.Context(), limit)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, data)
}
```

- [ ] **Step 9: Public handler test (new public_test.go)**

New `internal/handler/analytics/public_test.go` (own fake service + own router; reuse `doGET`/`decode` from `admin_test.go`, same `analytics_test` package):

```go
package analytics_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	hdl "github.com/vpt/blog-backend/internal/handler/analytics"
	"github.com/vpt/blog-backend/pkg/response"
)

type fakePublic struct{ lastLimit int }

func (f *fakePublic) Summary(context.Context) (dto.PublicSummary, error) {
	return dto.PublicSummary{TodayPV: 5, Online: 2}, nil
}
func (f *fakePublic) Popular(_ context.Context, limit int) ([]dto.PublicPageStat, error) {
	f.lastLimit = limit
	return []dto.PublicPageStat{{Path: "/a", PV: 9}}, nil
}

func newPublicRouter(h *hdl.PublicHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/analytics/public/summary", h.Summary)
	r.GET("/analytics/public/popular", h.Popular)
	return r
}

func TestPublicSummaryHandler(t *testing.T) {
	r := newPublicRouter(hdl.NewPublicHandler(&fakePublic{}))
	w := doGET(r, "/analytics/public/summary")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeOK, decode(t, w).Code)
	assert.Contains(t, w.Body.String(), "today_pv")
}

func TestPublicPopular_LimitCap(t *testing.T) {
	fp := &fakePublic{}
	r := newPublicRouter(hdl.NewPublicHandler(fp))
	w := doGET(r, "/analytics/public/popular?limit=500")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 20, fp.lastLimit)
}

func TestPublicPopular_InvalidLimit(t *testing.T) {
	r := newPublicRouter(hdl.NewPublicHandler(&fakePublic{}))
	w := doGET(r, "/analytics/public/popular?limit=abc")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}
```

- [ ] **Step 10: Wire services/handlers + routes**

In `router.go`:
- Add field `analyticsPublic *analyticshandler.PublicHandler` to `routeHandlers`.
- In `newAnalyticsCollectHandler`, after `adminHandler`, add and EXTEND the return tuple to include the public handler:

```go
	publicSvc := analyticsservice.NewPublicService(analyticsRepo, realtime, redisClient, analyticsCfg.PublicCacheTTL, log)
	publicHandler := analyticshandler.NewPublicHandler(publicSvc)
```

Update the function signature's return types and the `return ...` statement, and the caller (`newRouteHandlers`) to thread `analyticsPublic: publicHandler`. Read the existing tuple wiring (the function returns `(*CollectHandler, *AdminHandler, AnalyticsRuntime)` — add `*PublicHandler`) and update the single call site accordingly.

- In `registerPublicRoutes`, add (rate-limited, NO auth):

```go
	r.GET("/analytics/public/summary", middleware.RateLimitNormal(redisClient), handlers.analyticsPublic.Summary)
	r.GET("/analytics/public/popular", middleware.RateLimitNormal(redisClient), handlers.analyticsPublic.Popular)
```

- [ ] **Step 11: Build + test + commit**

Run: `go build ./... && go test ./internal/repository/analytics/... ./internal/service/analytics/... ./internal/handler/analytics/...`

```bash
git add internal/dto/analytics internal/repository/analytics internal/service/analytics/public.go internal/service/analytics/public_test.go internal/handler/analytics/public.go internal/handler/analytics/public_test.go internal/router/router.go
git commit -m "feat(analytics): 新增前台公开总览与热门页面接口"
```

---

## Task 5: Admin backfill endpoint

**Files:** new `internal/service/analytics/backfill.go`, `internal/dto/analytics/analytics.go`, `internal/handler/analytics/admin.go`, `internal/router/router.go`; tests new `internal/service/analytics/backfill_test.go`, `internal/handler/analytics/admin_test.go`.

**Interfaces:** `BackfillService.Backfill(ctx, from, to string) (days int, err error)`; `AdminHandler` gains a `backfill svc.BackfillService` field; `NewAdminHandler(svc.QueryService, svc.BackfillService)`; route `POST /admin/analytics/backfill?from=&to=` (admin only).

> **Cross-task note:** `NewAdminHandler` gets a SECOND parameter here. You MUST update every existing `hdl.NewAdminHandler(...)` call in `admin_test.go` (and the Task 3 dimensions test) to pass a `&fakeBackfill{}` as the second arg, and the real wiring in router.go.

- [ ] **Step 1: Add DTO**

```go
// BackfillResult 回填结果：成功重算的天数与区间。
type BackfillResult struct {
	From string `json:"from"`
	To   string `json:"to"`
	Days int    `json:"days"`
}
```

- [ ] **Step 2: Failing service test**

New `internal/service/analytics/backfill_test.go`:

```go
package analytics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestBackfill_InclusiveLoop(t *testing.T) {
	var got []string
	s := svc.NewBackfillService(func(_ context.Context, date string) error {
		got = append(got, date)
		return nil
	})
	days, err := s.Backfill(context.Background(), "2026-06-01", "2026-06-03")
	require.NoError(t, err)
	assert.Equal(t, 3, days)
	require.Len(t, got, 3)
	assert.Equal(t, "2026-06-01", got[0])
	assert.Equal(t, "2026-06-03", got[2])
}

func TestBackfill_StopsOnError(t *testing.T) {
	calls := 0
	s := svc.NewBackfillService(func(_ context.Context, _ string) error {
		calls++
		return errors.New("boom")
	})
	_, err := s.Backfill(context.Background(), "2026-06-01", "2026-06-03")
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}
```

- [ ] **Step 3: Run to verify fail**

Run: `go test ./internal/service/analytics/ -run TestBackfill -v` → FAIL (undefined).

- [ ] **Step 4: Implement BackfillService (new backfill.go)**

```go
package analytics

import (
	"context"
	"fmt"
	"time"
)

// BackfillService 对指定日期区间逐日重算聚合（幂等），用于改规则后重刷历史或补漏天。
type BackfillService interface {
	Backfill(ctx context.Context, from, to string) (days int, err error)
}

type backfillService struct {
	rollupDay func(ctx context.Context, date string) error
}

// NewBackfillService 注入单日聚合函数（由 worker.Rollup.RollupDay 提供），
// 以函数值注入避免 service→worker 的循环依赖。
func NewBackfillService(rollupDay func(ctx context.Context, date string) error) BackfillService {
	return &backfillService{rollupDay: rollupDay}
}

const backfillLayout = "2006-01-02"

// Backfill 逐日（含端点）调用 rollupDay；遇错即停并返回已完成天数。
func (s *backfillService) Backfill(ctx context.Context, from, to string) (int, error) {
	tz := shanghaiTZ() // 复用 public.go 的东八区
	start, err := time.ParseInLocation(backfillLayout, from, tz)
	if err != nil {
		return 0, fmt.Errorf("解析 from 失败: %w", err)
	}
	end, err := time.ParseInLocation(backfillLayout, to, tz)
	if err != nil {
		return 0, fmt.Errorf("解析 to 失败: %w", err)
	}
	days := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if err := s.rollupDay(ctx, d.Format(backfillLayout)); err != nil {
			return days, fmt.Errorf("回填 %s 失败: %w", d.Format(backfillLayout), err)
		}
		days++
	}
	return days, nil
}
```

> `shanghaiTZ()` is defined in `public.go` (Task 4), same package. If executing Task 5 before Task 4, define `shanghaiTZ()` here and remove it from public.go later — keep EXACTLY ONE definition in the package.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./internal/service/analytics/ -run TestBackfill -v` → PASS.

- [ ] **Step 6: Handler + range cap + swagger + AdminHandler change**

In `admin.go`:
- Add `const maxBackfillDays = 92`.
- Change `AdminHandler` struct to add `backfill svc.BackfillService` and update `NewAdminHandler`:

```go
type AdminHandler struct {
	svc      svc.QueryService
	backfill svc.BackfillService
}

func NewAdminHandler(s svc.QueryService, b svc.BackfillService) *AdminHandler {
	return &AdminHandler{svc: s, backfill: b}
}
```

- Add the handler + a required-range parser:

```go
// Backfill 重算指定区间的日聚合（管理员，幂等）。
// @Summary  回填日聚合
// @Tags     analytics
// @Produce  json
// @Param    from query string true "起始日期 YYYY-MM-DD"
// @Param    to   query string true "结束日期 YYYY-MM-DD"
// @Success  200 {object} response.Response{data=dto.BackfillResult} "统一响应；code=0 成功，code=400 参数错误"
// @Failure  401 {object} response.Response "未登录或 token 已过期"
// @Failure  403 {object} response.Response "权限不足"
// @Failure  500 {object} response.Response "服务器内部错误"
// @Router   /admin/analytics/backfill [post]
func (h *AdminHandler) Backfill(c *gin.Context) {
	from, to, ok := parseRequiredRange(c)
	if !ok {
		return
	}
	days, err := h.backfill.Backfill(c.Request.Context(), from, to)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.Success(c, dto.BackfillResult{From: from, To: to, Days: days})
}

// parseRequiredRange 解析必填 from/to，校验格式、顺序与回填跨度上限。
func parseRequiredRange(c *gin.Context) (from, to string, ok bool) {
	fromRaw, toRaw := c.Query("from"), c.Query("to")
	if fromRaw == "" || toRaw == "" {
		response.Fail(c, response.CodeBadRequest, "from 和 to 均为必填 YYYY-MM-DD")
		return "", "", false
	}
	fromDate, err := time.ParseInLocation(dateLayout, fromRaw, adminTZ)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "from 必须是 YYYY-MM-DD 格式")
		return "", "", false
	}
	toDate, err := time.ParseInLocation(dateLayout, toRaw, adminTZ)
	if err != nil {
		response.Fail(c, response.CodeBadRequest, "to 必须是 YYYY-MM-DD 格式")
		return "", "", false
	}
	if toDate.Before(fromDate) {
		response.Fail(c, response.CodeBadRequest, "to 不能早于 from")
		return "", "", false
	}
	if toDate.Sub(fromDate) > time.Duration(maxBackfillDays)*24*time.Hour {
		response.Fail(c, response.CodeBadRequest, "回填跨度不能超过 92 天")
		return "", "", false
	}
	return fromRaw, toRaw, true
}
```

- [ ] **Step 7: Handler test + update ALL NewAdminHandler call sites**

In `admin_test.go`:
- Add a fake:

```go
type fakeBackfill struct{ lastFrom, lastTo string }

func (f *fakeBackfill) Backfill(_ context.Context, from, to string) (int, error) {
	f.lastFrom, f.lastTo = from, to
	return 3, nil
}
```

- Update `newRouter` to take both and register the route:

```go
func newRouter(h *hdl.AdminHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin/analytics/overview", h.Overview)
	r.GET("/admin/analytics/trend", h.Trend)
	r.GET("/admin/analytics/pages", h.Pages)
	r.GET("/admin/analytics/dimensions", h.Dimensions)
	r.POST("/admin/analytics/backfill", h.Backfill)
	return r
}
```

- Replace EVERY `hdl.NewAdminHandler(<fq>)` with `hdl.NewAdminHandler(<fq>, &fakeBackfill{})` across the file (and the Task 3 dimensions tests).
- Add a POST helper + tests:

```go
func doPOST(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAdminBackfill_HappyPath(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))
	w := doPOST(r, "/admin/analytics/backfill?from=2026-06-01&to=2026-06-03")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeOK, decode(t, w).Code)
	assert.Contains(t, w.Body.String(), "days")
}

func TestAdminBackfill_MissingParams(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))
	w := doPOST(r, "/admin/analytics/backfill")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}

func TestAdminBackfill_RangeTooLarge(t *testing.T) {
	r := newRouter(hdl.NewAdminHandler(&fakeQuery{}, &fakeBackfill{}))
	w := doPOST(r, "/admin/analytics/backfill?from=2026-01-01&to=2026-06-01")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, response.CodeBadRequest, decode(t, w).Code)
}
```

- [ ] **Step 8: Wire router — construct a dedicated backfill Rollup**

In `newAnalyticsCollectHandler` (router.go), before building `adminHandler`:

```go
	// 专用的回填 Rollup：RollupDay 幂等且无状态，可与调度器的实例分开（区别于唯一的 ingestor）。
	backfillRollup := analyticsworker.NewRollup(analyticsRepo, analyticsRepo, log)
	backfillSvc := analyticsservice.NewBackfillService(backfillRollup.RollupDay)
	adminHandler := analyticshandler.NewAdminHandler(querySvc, backfillSvc)
```

Register the route in `registerAdminRoutes`:

```go
	admin.POST("/analytics/backfill", handlers.analyticsAdmin.Backfill)
```

- [ ] **Step 9: Build + test + commit**

Run: `go build ./... && go test ./internal/service/analytics/... ./internal/handler/analytics/...`

```bash
git add internal/dto/analytics internal/service/analytics/backfill.go internal/service/analytics/backfill_test.go internal/handler/analytics/admin.go internal/handler/analytics/admin_test.go internal/router/router.go
git commit -m "feat(analytics): 新增后台日聚合回填接口"
```

---

## Task 6: Regenerate Swagger

- [ ] **Step 1: Regenerate**

Read the `Makefile` `swag` target first. Run `make swag` (if `swag` is not installed in the environment, note it and skip — the annotations are still committed; a maintainer regenerates later). Expected output: docs include `dimensions`, `public/summary`, `public/popular`, `backfill` with the four new `dto.*` schemas.

- [ ] **Step 2: Build + full suite**

Run: `go build ./... && go test ./...` → PASS.

- [ ] **Step 3: Commit (only if docs changed)**

```bash
git add docs/
git commit -m "docs(analytics): 重新生成 Phase 2 接口 Swagger"
```

---

## Self-Review

**Spec coverage (handoff §6):** R2 → Task 1; R3 → Task 2; dimensions → Task 3; public summary/popular (no-auth, aggregate-only, exclude /admin/*, Redis TTL cache, RateLimitNormal) → Task 4; backfill admin endpoint → Task 5; swagger → Task 6.

**Testing approach matches the repo:** hand-written fakes (`fakeQuery`/`fakeRealtime`/`fakeQueryRepo` style), `newRepo(t)` + sqlmock `regexp` matchers, gin TestMode + `doGET`/`decode`. NO gomock, NO mock generation.

**Cross-cutting risks called out:** (1) `NewAdminHandler` gains a 2nd arg in Task 5 — all `admin_test.go` call sites + router wiring must update; (2) `QueryReader` gains `QueryDimRange` in Task 3 — `fakeQueryRepo` must add it; (3) `shanghaiTZ()` single definition; (4) `AggregateDay` adds an 8th query — the existing `TestAggregateDay` must append the session expectation; (5) sqlmock regex patterns may need adjustment to the actual rendered SQL shown in the red run.

**Layering:** no service→worker import (backfill via injected func); handlers return `dto.*` only; constructors inject infra; zap for logging.

---

## Execution Handoff

Recommended: superpowers:subagent-driven-development — fresh implementer + task reviewer per task; final whole-branch review at the end. Branch `feat/analytics-phase2` already cut from `dev`.
