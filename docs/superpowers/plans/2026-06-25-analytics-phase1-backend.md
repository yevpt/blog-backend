# 自建站点分析 Phase 1 后端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现站点分析的后端采集闭环——`/collect` 上报、富化、bot 过滤、原始入库、Redis 实时/今日计数、日聚合 job、后台只读 API，含全部/注册/匿名三档细分。

**Architecture:** tracker → web BFF → 后端 `POST /collect`（挂 `OptionalAuth` 解析 user_id）→ 同步富化 + Redis 实时层 + 有界 channel → ingest batch worker 落原始表 → analytics worker 日滚动聚合成永久聚合表 → 后台 `/admin/analytics/*` 只读聚合表 + Redis。

**Tech Stack:** Gin、GORM/MySQL、go-redis、zap、viper；`github.com/mileusna/useragent`（UA 解析）、`github.com/lionsoul2014/ip2region/binding/golang/xdb`（IP→地理）。

## Global Constraints

- Go 1.25+，模块 `github.com/vpt/blog-backend`。
- 禁全局变量持有 db/redis/logger 等基础设施，一律构造注入。
- 生产代码禁 `fmt.Println`，用 `zap.Logger`。
- 禁直接返回 `model.*` 给前端或写进 Swagger，出参走 `internal/dto`。
- 分层：handler→service→repository，依赖经 interface 注入便于 gomock；`_test` 外部测试包。
- 统计时区固定 `Asia/Shanghai`；原始事件表用硬删除（不嵌 `model.Base` 软删除）。
- 测试分层：repository 用 go-sqlmock，service 用 gomock，handler 用 httptest+testify。
- 提交遵 Conventional Commits + 中文主题；scope 用英文小写。
- 改了接口要跑相关测试；新增 swagger 注解后 `make swag`。

---

## File Structure

**新建：**
- `internal/model/analytics.go` — 5 张表的 GORM 模型。
- `internal/service/analytics/referer.go` — referer 分类（纯函数）。
- `internal/service/analytics/useragent.go` — UA 解析包装。
- `internal/service/analytics/sanitize.go` — path/referer 脱敏 + ip_hash。
- `internal/service/analytics/botfilter.go` — bot 判定单元。
- `internal/service/analytics/geoip.go` — ip2region 包装（缺库优雅降级）。
- `internal/service/analytics/enrich.go` — 富化编排，把上述组合成 `EnrichedEvent`。
- `internal/service/analytics/realtime.go` — Redis 在线 + 今日计数（三档）。
- `internal/service/analytics/collect.go` — 上报编排 service。
- `internal/service/analytics/query.go` — 后台只读查询 service。
- `internal/service/analytics/types.go` — 跨文件共享的入参/中间结构体与接口。
- `internal/repository/analytics/repository.go` — 原始写入、聚合读写、清理。
- `internal/worker/analytics/ingest.go` — 有界 channel + 批量落库 worker。
- `internal/worker/analytics/rollup.go` — 日聚合 + 保留期清理 + 在线清理 worker。
- `internal/handler/analytics/collect.go` — 上报 handler。
- `internal/handler/analytics/admin.go` — 后台查询 handler。
- `internal/dto/analytics/analytics.go` — 出入参 DTO。

**修改：**
- `internal/dbschema/schema.go` — AutoMigrate 注册新模型。
- `internal/router/router.go` — 注册路由、组装依赖、启动 worker。
- `internal/bootstrap/bootstrap.go` — 启动 analytics worker。
- `cmd/server/main.go` — 调用 bootstrap 启动。
- `pkg/config`（配置结构）+ `config/*.yaml` — 新增 analytics 配置块。
- `docker-compose.yml` / `.env.example` — `ANALYTICS_ALLOWED_ORIGINS` 与 geoip 文件挂载。
- `go.mod` — 新增两个依赖。

---

## Task 1: 数据模型与迁移

**Files:**
- Create: `internal/model/analytics.go`
- Modify: `internal/dbschema/schema.go`
- Test: `internal/model/analytics_test.go`

**Interfaces:**
- Produces: `model.AnalyticsEvent`、`model.AnalyticsSession`、`model.AnalyticsDaily`、`model.AnalyticsDailyDim`、`model.AnalyticsPageDaily`，各自 `TableName()`。字段见下。

- [ ] **Step 1: 写模型**

`internal/model/analytics.go`：

```go
package model

import "time"

// AnalyticsEvent 单次 PV 原始事件，短期保留、硬删除（不嵌 Base 软删除）。
type AnalyticsEvent struct {
	ID              uint64    `gorm:"primaryKey"`
	EventType       string    `gorm:"type:varchar(16);index"` // page_view
	VisitorID       string    `gorm:"type:varchar(64);index"`
	UserID          *uint     `gorm:"index"`
	IsAuthenticated bool      `gorm:"index"`
	SessionID       string    `gorm:"type:varchar(64);index"`
	Path            string    `gorm:"type:varchar(512);index"`
	Title           string    `gorm:"type:varchar(255)"`
	RefererHost     string    `gorm:"type:varchar(255)"`
	RefererType     string    `gorm:"type:varchar(16)"`  // direct/search/social/external/internal
	DeviceType      string    `gorm:"type:varchar(16)"`  // desktop/mobile/tablet/bot
	Browser         string    `gorm:"type:varchar(32)"`
	OS              string    `gorm:"type:varchar(32)"`
	Country         string    `gorm:"type:varchar(64)"`
	Region          string    `gorm:"type:varchar(64)"`
	IPHash          string    `gorm:"type:varchar(64)"`
	IsNewVisitor    bool
	IsBot           bool   `gorm:"index"`
	BotReason       string `gorm:"type:varchar(64)"`
	IsSuspect       bool
	CreatedAt       time.Time `gorm:"index"`
}

func (AnalyticsEvent) TableName() string { return "analytics_events" }

// AnalyticsSession 会话聚合，心跳 upsert 更新，硬删除。
type AnalyticsSession struct {
	SessionID       string `gorm:"primaryKey;type:varchar(64)"`
	VisitorID       string `gorm:"type:varchar(64);index"`
	UserID          *uint  `gorm:"index"`
	IsAuthenticated bool
	FirstSeen       time.Time
	LastSeen        time.Time `gorm:"index"`
	PVCount         int
	EntryPath       string `gorm:"type:varchar(512)"`
	ExitPath        string `gorm:"type:varchar(512)"`
	Duration        int    // 秒
	IsBounce        bool
	DeviceType      string `gorm:"type:varchar(16)"`
	Browser         string `gorm:"type:varchar(32)"`
	OS              string `gorm:"type:varchar(32)"`
	Country         string `gorm:"type:varchar(64)"`
	Region          string `gorm:"type:varchar(64)"`
	RefererType     string `gorm:"type:varchar(16)"`
	IsBot           bool
}

func (AnalyticsSession) TableName() string { return "analytics_sessions" }

// AnalyticsDaily 每日站点总览（永久），含注册/匿名分档。
type AnalyticsDaily struct {
	Date         string `gorm:"primaryKey;type:varchar(10)"` // YYYY-MM-DD（Asia/Shanghai）
	PV           int
	UV           int
	Sessions     int
	NewVisitors  int
	AvgDuration  int
	BounceRate   float64
	RegisteredPV int
	RegisteredUV int
	AnonymousPV  int
	AnonymousUV  int
}

func (AnalyticsDaily) TableName() string { return "analytics_daily" }

// AnalyticsDailyDim 每日每维度（永久），长表。
type AnalyticsDailyDim struct {
	Date      string `gorm:"primaryKey;type:varchar(10)"`
	Dimension string `gorm:"primaryKey;type:varchar(32)"` // referer_type/device/browser/os/country/user_type
	DimValue  string `gorm:"primaryKey;type:varchar(64)"`
	PV        int
	UV        int
}

func (AnalyticsDailyDim) TableName() string { return "analytics_daily_dim" }

// AnalyticsPageDaily 每日每路径（永久），支撑热门页面排行。
type AnalyticsPageDaily struct {
	Date  string `gorm:"primaryKey;type:varchar(10)"`
	Path  string `gorm:"primaryKey;type:varchar(512)"`
	Title string `gorm:"type:varchar(255)"`
	PV    int
	UV    int
}

func (AnalyticsPageDaily) TableName() string { return "analytics_page_daily" }
```

- [ ] **Step 2: 写表名测试**

`internal/model/analytics_test.go`：

```go
package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/model"
)

func TestAnalyticsTableNames(t *testing.T) {
	assert.Equal(t, "analytics_events", model.AnalyticsEvent{}.TableName())
	assert.Equal(t, "analytics_sessions", model.AnalyticsSession{}.TableName())
	assert.Equal(t, "analytics_daily", model.AnalyticsDaily{}.TableName())
	assert.Equal(t, "analytics_daily_dim", model.AnalyticsDailyDim{}.TableName())
	assert.Equal(t, "analytics_page_daily", model.AnalyticsPageDaily{}.TableName())
}
```

- [ ] **Step 3: 注册迁移**

`internal/dbschema/schema.go` 的 `AutoMigrate` 列表末尾追加：

```go
		&model.AnalyticsEvent{},
		&model.AnalyticsSession{},
		&model.AnalyticsDaily{},
		&model.AnalyticsDailyDim{},
		&model.AnalyticsPageDaily{},
```

- [ ] **Step 4: 跑测试 + 编译**

Run: `go test ./internal/model/... && go build ./...`
Expected: PASS，编译通过。

- [ ] **Step 5: Commit**

```bash
git add internal/model/analytics.go internal/model/analytics_test.go internal/dbschema/schema.go
git commit -m "feat(analytics): 新增统计数据模型与迁移"
```

---

## Task 2: referer 分类（纯函数）

**Files:**
- Create: `internal/service/analytics/referer.go`
- Test: `internal/service/analytics/referer_test.go`

**Interfaces:**
- Produces: `func ClassifyReferer(refererHost, siteHost string) (refererType string)`，返回 `direct/search/social/external/internal`。空 host → `direct`；等于 siteHost → `internal`。

- [ ] **Step 1: 写失败测试**

`internal/service/analytics/referer_test.go`：

```go
package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestClassifyReferer(t *testing.T) {
	cases := []struct{ host, site, want string }{
		{"", "yevpt.com", "direct"},
		{"yevpt.com", "yevpt.com", "internal"},
		{"www.google.com", "yevpt.com", "search"},
		{"www.baidu.com", "yevpt.com", "search"},
		{"t.co", "yevpt.com", "social"},
		{"www.zhihu.com", "yevpt.com", "social"},
		{"example.com", "yevpt.com", "external"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, svc.ClassifyReferer(c.host, c.site), c.host)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/service/analytics/ -run TestClassifyReferer`
Expected: FAIL（未定义 `ClassifyReferer`）。

- [ ] **Step 3: 实现**

`internal/service/analytics/referer.go`：

```go
package analytics

import "strings"

var searchHosts = []string{"google.", "baidu.", "bing.", "sogou.", "so.com", "duckduckgo.", "yandex."}
var socialHosts = []string{"t.co", "twitter.", "x.com", "facebook.", "zhihu.", "weibo.", "douban.", "github.", "telegram.", "t.me"}

// ClassifyReferer 把来源 host 归类为 direct/search/social/external/internal。
func ClassifyReferer(refererHost, siteHost string) string {
	h := strings.ToLower(strings.TrimSpace(refererHost))
	if h == "" {
		return "direct"
	}
	if h == strings.ToLower(siteHost) || strings.HasSuffix(h, "."+strings.ToLower(siteHost)) {
		return "internal"
	}
	for _, s := range searchHosts {
		if strings.Contains(h, s) {
			return "search"
		}
	}
	for _, s := range socialHosts {
		if strings.Contains(h, s) {
			return "social"
		}
	}
	return "external"
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/service/analytics/ -run TestClassifyReferer`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/service/analytics/referer.go internal/service/analytics/referer_test.go
git commit -m "feat(analytics): 新增来源 referer 分类"
```

---

## Task 3: UA 解析包装

**Files:**
- Create: `internal/service/analytics/useragent.go`
- Test: `internal/service/analytics/useragent_test.go`
- Modify: `go.mod`（新增 `github.com/mileusna/useragent`）

**Interfaces:**
- Produces: `func ParseUserAgent(ua string) (deviceType, browser, os string)`。deviceType ∈ desktop/mobile/tablet/bot；空 ua → desktop/""/""。

- [ ] **Step 1: 装依赖**

Run: `go get github.com/mileusna/useragent@latest`
Expected: go.mod 出现该依赖。

- [ ] **Step 2: 写失败测试**

`internal/service/analytics/useragent_test.go`：

```go
package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestParseUserAgent(t *testing.T) {
	chrome := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
	dt, br, os := svc.ParseUserAgent(chrome)
	assert.Equal(t, "desktop", dt)
	assert.Equal(t, "Chrome", br)
	assert.Equal(t, "Windows", os)

	iphone := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148"
	dt, _, _ = svc.ParseUserAgent(iphone)
	assert.Equal(t, "mobile", dt)

	dt2, _, _ := svc.ParseUserAgent("")
	assert.Equal(t, "desktop", dt2)
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/service/analytics/ -run TestParseUserAgent`
Expected: FAIL（未定义 `ParseUserAgent`）。

- [ ] **Step 4: 实现**

`internal/service/analytics/useragent.go`：

```go
package analytics

import "github.com/mileusna/useragent"

// ParseUserAgent 解析 UA，返回设备类型/浏览器/操作系统。
func ParseUserAgent(ua string) (deviceType, browser, os string) {
	if ua == "" {
		return "desktop", "", ""
	}
	p := useragent.Parse(ua)
	switch {
	case p.Bot:
		deviceType = "bot"
	case p.Tablet:
		deviceType = "tablet"
	case p.Mobile:
		deviceType = "mobile"
	default:
		deviceType = "desktop"
	}
	return deviceType, p.Name, p.OS
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/service/analytics/ -run TestParseUserAgent`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/service/analytics/useragent.go internal/service/analytics/useragent_test.go
git commit -m "feat(analytics): 新增 UA 解析包装"
```

---

## Task 4: 脱敏与 ip_hash

**Files:**
- Create: `internal/service/analytics/sanitize.go`
- Test: `internal/service/analytics/sanitize_test.go`

**Interfaces:**
- Produces:
  - `func SanitizePath(rawURL string) string` — 剥离 query/fragment，截断 ≤512。
  - `func RefererHost(rawReferer string) string` — 取 host（小写），解析失败 → ""。
  - `func HashIP(ip, salt string) string` — IPv4 去末段 / IPv6 去后 64 位后加 salt 做 sha256，返回 16 位 hex 前缀。空 ip → ""。

- [ ] **Step 1: 写失败测试**

`internal/service/analytics/sanitize_test.go`：

```go
package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestSanitizePath(t *testing.T) {
	assert.Equal(t, "/articles/1", svc.SanitizePath("/articles/1?token=secret#x"))
	assert.Equal(t, "/", svc.SanitizePath("/"))
}

func TestRefererHost(t *testing.T) {
	assert.Equal(t, "www.google.com", svc.RefererHost("https://www.google.com/search?q=x"))
	assert.Equal(t, "", svc.RefererHost(""))
	assert.Equal(t, "", svc.RefererHost("not a url"))
}

func TestHashIP(t *testing.T) {
	assert.Equal(t, "", svc.HashIP("", "salt"))
	// 同段 IP 末段不同 → 同一哈希（已去末段）
	a := svc.HashIP("1.2.3.4", "salt")
	b := svc.HashIP("1.2.3.99", "salt")
	assert.Equal(t, a, b)
	assert.Len(t, a, 16)
	// 不同 salt → 不同哈希
	assert.NotEqual(t, a, svc.HashIP("1.2.3.4", "other"))
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/service/analytics/ -run 'TestSanitizePath|TestRefererHost|TestHashIP'`
Expected: FAIL。

- [ ] **Step 3: 实现**

`internal/service/analytics/sanitize.go`：

```go
package analytics

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"strings"
)

const maxPathLen = 512

// SanitizePath 剥离 query 与 fragment，仅保留 path，并截断长度。
func SanitizePath(rawURL string) string {
	p := rawURL
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if p == "" {
		p = "/"
	}
	if len(p) > maxPathLen {
		p = p[:maxPathLen]
	}
	return p
}

// RefererHost 从完整 referer 取小写 host，失败返回空串。
func RefererHost(rawReferer string) string {
	if rawReferer == "" {
		return ""
	}
	u, err := url.Parse(rawReferer)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// HashIP 对 IP 去除主机段后加 salt 做 sha256，返回 16 位 hex 前缀；不存明文。
func HashIP(ip, salt string) string {
	if ip == "" {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	var masked net.IP
	if v4 := parsed.To4(); v4 != nil {
		masked = v4.Mask(net.CIDRMask(24, 32)) // 去末段
	} else {
		masked = parsed.Mask(net.CIDRMask(64, 128)) // 去后 64 位
	}
	sum := sha256.Sum256([]byte(masked.String() + "|" + salt))
	return hex.EncodeToString(sum[:])[:16]
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/service/analytics/ -run 'TestSanitizePath|TestRefererHost|TestHashIP'`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/service/analytics/sanitize.go internal/service/analytics/sanitize_test.go
git commit -m "feat(analytics): 新增 path/referer 脱敏与 ip 哈希"
```

---

## Task 5: botfilter 判定

**Files:**
- Create: `internal/service/analytics/botfilter.go`
- Test: `internal/service/analytics/botfilter_test.go`

**Interfaces:**
- Produces: `func DetectBot(ua, deviceType string) (isBot bool, reason string)`。命中 UA 黑名单或 deviceType=="bot" → true。reason ∈ `ua_blacklist`/`ua_device`/""。

- [ ] **Step 1: 写失败测试**

`internal/service/analytics/botfilter_test.go`：

```go
package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func TestDetectBot(t *testing.T) {
	bot, reason := svc.DetectBot("Mozilla/5.0 (compatible; Googlebot/2.1)", "desktop")
	assert.True(t, bot)
	assert.Equal(t, "ua_blacklist", reason)

	bot2, reason2 := svc.DetectBot("python-requests/2.31", "desktop")
	assert.True(t, bot2)
	assert.Equal(t, "ua_blacklist", reason2)

	bot3, reason3 := svc.DetectBot("some-ua", "bot")
	assert.True(t, bot3)
	assert.Equal(t, "ua_device", reason3)

	bot4, reason4 := svc.DetectBot("Mozilla/5.0 (Windows NT 10.0) Chrome/120", "desktop")
	assert.False(t, bot4)
	assert.Equal(t, "", reason4)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/service/analytics/ -run TestDetectBot`
Expected: FAIL。

- [ ] **Step 3: 实现**

`internal/service/analytics/botfilter.go`：

```go
package analytics

import (
	"regexp"
	"strings"
)

// botUARegex 覆盖常见爬虫/脚本 UA 特征（社区列表精简版）。
var botUARegex = regexp.MustCompile(`(?i)(bot|spider|crawl|slurp|headless|phantomjs|curl|wget|python-requests|python-urllib|go-http-client|java/|okhttp|scrapy|httpclient|facebookexternalhit|googlebot|bingbot|baiduspider|yandex|sogou|semrush|ahrefs|mj12)`)

// DetectBot 判定是否爬虫/机器人，返回是否命中与原因。
func DetectBot(ua, deviceType string) (bool, string) {
	if deviceType == "bot" {
		return true, "ua_device"
	}
	if botUARegex.MatchString(strings.ToLower(ua)) {
		return true, "ua_blacklist"
	}
	return false, ""
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/service/analytics/ -run TestDetectBot`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/service/analytics/botfilter.go internal/service/analytics/botfilter_test.go
git commit -m "feat(analytics): 新增 bot 判定单元"
```

---

## Task 6: ip2region 地理解析（缺库降级）

**Files:**
- Create: `internal/service/analytics/geoip.go`
- Test: `internal/service/analytics/geoip_test.go`
- Modify: `go.mod`（新增 `github.com/lionsoul2014/ip2region/binding/golang`）

**Interfaces:**
- Produces:
  - `type GeoResolver interface { Resolve(ip string) (country, region string) }`
  - `func NewGeoResolver(xdbPath string, logger *zap.Logger) GeoResolver` — 路径为空或加载失败 → 返回 no-op resolver（始终返回 "",""），记 warn，不 panic。

- [ ] **Step 1: 装依赖**

Run: `go get github.com/lionsoul2014/ip2region/binding/golang@latest`
Expected: go.mod 出现该依赖。

- [ ] **Step 2: 写失败测试（仅测降级路径，不依赖真实 xdb）**

`internal/service/analytics/geoip_test.go`：

```go
package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
	"go.uber.org/zap"
)

func TestGeoResolverNoopWhenMissing(t *testing.T) {
	r := svc.NewGeoResolver("", zap.NewNop())
	country, region := r.Resolve("1.2.3.4")
	assert.Equal(t, "", country)
	assert.Equal(t, "", region)

	r2 := svc.NewGeoResolver("/nonexistent/path.xdb", zap.NewNop())
	c2, rg2 := r2.Resolve("1.2.3.4")
	assert.Equal(t, "", c2)
	assert.Equal(t, "", rg2)
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/service/analytics/ -run TestGeoResolver`
Expected: FAIL。

- [ ] **Step 4: 实现**

`internal/service/analytics/geoip.go`：

```go
package analytics

import (
	"strings"

	xdb "github.com/lionsoul2014/ip2region/binding/golang/xdb"
	"go.uber.org/zap"
)

// GeoResolver 把 IP 解析为国家/地区。
type GeoResolver interface {
	Resolve(ip string) (country, region string)
}

type noopGeo struct{}

func (noopGeo) Resolve(string) (string, string) { return "", "" }

type ip2regionGeo struct {
	searcher *xdb.Searcher
	logger   *zap.Logger
}

// NewGeoResolver 加载 ip2region xdb；路径空或加载失败时降级为 no-op。
func NewGeoResolver(xdbPath string, logger *zap.Logger) GeoResolver {
	if xdbPath == "" {
		logger.Warn("analytics geoip 未配置 xdb 路径，地理解析降级关闭")
		return noopGeo{}
	}
	buf, err := xdb.LoadContentFromFile(xdbPath)
	if err != nil {
		logger.Warn("analytics geoip 加载 xdb 失败，地理解析降级关闭", zap.Error(err))
		return noopGeo{}
	}
	searcher, err := xdb.NewWithBuffer(buf)
	if err != nil {
		logger.Warn("analytics geoip 创建 searcher 失败，地理解析降级关闭", zap.Error(err))
		return noopGeo{}
	}
	return &ip2regionGeo{searcher: searcher, logger: logger}
}

// Resolve 返回 (country, region)。ip2region 结果形如 "国家|区域|省份|城市|ISP"。
func (g *ip2regionGeo) Resolve(ip string) (string, string) {
	if ip == "" {
		return "", ""
	}
	region, err := g.searcher.SearchByStr(ip)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(region, "|")
	country, province := "", ""
	if len(parts) > 0 {
		country = normalizeGeo(parts[0])
	}
	if len(parts) > 2 {
		province = normalizeGeo(parts[2])
	}
	return country, province
}

func normalizeGeo(s string) string {
	if s == "0" || s == "" {
		return ""
	}
	return s
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/service/analytics/ -run TestGeoResolver`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/service/analytics/geoip.go internal/service/analytics/geoip_test.go
git commit -m "feat(analytics): 新增 ip2region 地理解析（缺库降级）"
```

---

## Task 7: 共享类型与富化编排

**Files:**
- Create: `internal/service/analytics/types.go`, `internal/service/analytics/enrich.go`
- Test: `internal/service/analytics/enrich_test.go`

**Interfaces:**
- Produces:
  - `type RawEvent struct { EventType, VisitorID, SessionID, Path, Title, Referer, UA, IP, Origin string; UserID *uint }`
  - `type EnrichedEvent struct { model.AnalyticsEvent }`（直接复用模型字段承载富化结果）
  - `type Enricher interface { Enrich(raw RawEvent) model.AnalyticsEvent }`
  - `func NewEnricher(geo GeoResolver, siteHost, ipSalt string) Enricher`
- Consumes: Task 2-6 的纯函数与 `GeoResolver`。

- [ ] **Step 1: 写失败测试**

`internal/service/analytics/enrich_test.go`：

```go
package analytics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/model"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

type fakeGeo struct{}

func (fakeGeo) Resolve(string) (string, string) { return "中国", "浙江省" }

func TestEnrich(t *testing.T) {
	e := svc.NewEnricher(fakeGeo{}, "yevpt.com", "salt")
	uid := uint(7)
	got := e.Enrich(svc.RawEvent{
		EventType: "page_view",
		VisitorID: "v1",
		SessionID: "s1",
		Path:      "/articles/1?token=x",
		Title:     "标题",
		Referer:   "https://www.google.com/search?q=a",
		UA:        "Mozilla/5.0 (Windows NT 10.0) Chrome/120 Safari/537.36",
		IP:        "1.2.3.4",
		UserID:    &uid,
	})

	assert.Equal(t, "/articles/1", got.Path)
	assert.Equal(t, "www.google.com", got.RefererHost)
	assert.Equal(t, "search", got.RefererType)
	assert.Equal(t, "desktop", got.DeviceType)
	assert.Equal(t, "中国", got.Country)
	assert.Equal(t, "浙江省", got.Region)
	assert.Equal(t, uint(7), *got.UserID)
	assert.True(t, got.IsAuthenticated)
	assert.False(t, got.IsBot)
	assert.NotEmpty(t, got.IPHash)

	var _ model.AnalyticsEvent = got
}

func TestEnrichBot(t *testing.T) {
	e := svc.NewEnricher(fakeGeo{}, "yevpt.com", "salt")
	got := e.Enrich(svc.RawEvent{EventType: "page_view", VisitorID: "v", SessionID: "s",
		Path: "/", UA: "Googlebot/2.1", IP: "1.2.3.4"})
	assert.True(t, got.IsBot)
	assert.Equal(t, "ua_blacklist", got.BotReason)
	assert.False(t, got.IsAuthenticated)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/service/analytics/ -run TestEnrich`
Expected: FAIL。

- [ ] **Step 3: 实现 types.go**

`internal/service/analytics/types.go`：

```go
package analytics

// RawEvent 是 collect 接口接收并交给 Enricher 的原始入参（IP/UA/Origin/UserID 由后端注入）。
type RawEvent struct {
	EventType string
	VisitorID string
	SessionID string
	Path      string
	Title     string
	Referer   string
	UA        string
	IP        string
	Origin    string
	UserID    *uint
}
```

- [ ] **Step 4: 实现 enrich.go**

`internal/service/analytics/enrich.go`：

```go
package analytics

import "github.com/vpt/blog-backend/internal/model"

const maxTitleLen = 255

// Enricher 把 RawEvent 富化为可入库的 AnalyticsEvent。
type Enricher interface {
	Enrich(raw RawEvent) model.AnalyticsEvent
}

type enricher struct {
	geo      GeoResolver
	siteHost string
	ipSalt   string
}

// NewEnricher 构造富化器。siteHost 用于 referer internal 判定，ipSalt 用于 ip 哈希。
func NewEnricher(geo GeoResolver, siteHost, ipSalt string) Enricher {
	return &enricher{geo: geo, siteHost: siteHost, ipSalt: ipSalt}
}

func (e *enricher) Enrich(raw RawEvent) model.AnalyticsEvent {
	deviceType, browser, os := ParseUserAgent(raw.UA)
	isBot, botReason := DetectBot(raw.UA, deviceType)
	refHost := RefererHost(raw.Referer)
	country, region := e.geo.Resolve(raw.IP)

	title := raw.Title
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen]
	}

	return model.AnalyticsEvent{
		EventType:       raw.EventType,
		VisitorID:       raw.VisitorID,
		UserID:          raw.UserID,
		IsAuthenticated: raw.UserID != nil,
		SessionID:       raw.SessionID,
		Path:            SanitizePath(raw.Path),
		Title:           title,
		RefererHost:     refHost,
		RefererType:     ClassifyReferer(refHost, e.siteHost),
		DeviceType:      deviceType,
		Browser:         browser,
		OS:              os,
		Country:         country,
		Region:          region,
		IPHash:          HashIP(raw.IP, e.ipSalt),
		IsBot:           isBot,
		BotReason:       botReason,
	}
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/service/analytics/ -run TestEnrich`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/service/analytics/types.go internal/service/analytics/enrich.go internal/service/analytics/enrich_test.go
git commit -m "feat(analytics): 新增事件富化编排"
```

---

## Task 8: Redis 实时层（在线 + 今日三档计数）

**Files:**
- Create: `internal/service/analytics/realtime.go`
- Test: `internal/service/analytics/realtime_test.go`（用 miniredis）

**Interfaces:**
- Produces:
  - `type Realtime interface { TouchOnline(ctx, visitorID string) error; OnlineCount(ctx) (int64, error); IncrToday(ctx context.Context, ev model.AnalyticsEvent) error; TodayCounters(ctx context.Context) (TodayStat, error) }`
  - `type TodayStat struct { PV, UV, RegisteredPV, RegisteredUV, AnonymousPV, AnonymousUV int64 }`
  - `func NewRealtime(rdb *redis.Client, tz *time.Location, onlineWindow time.Duration) Realtime`
- Consumes: `model.AnalyticsEvent`。

- [ ] **Step 1: 写失败测试**

`internal/service/analytics/realtime_test.go`：

```go
package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

func newRT(t *testing.T) (svc.Realtime, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return svc.NewRealtime(rdb, time.UTC, 90*time.Second), mr
}

func TestOnlineCount(t *testing.T) {
	rt, _ := newRT(t)
	ctx := context.Background()
	require.NoError(t, rt.TouchOnline(ctx, "v1"))
	require.NoError(t, rt.TouchOnline(ctx, "v2"))
	require.NoError(t, rt.TouchOnline(ctx, "v1"))
	n, err := rt.OnlineCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestIncrTodaySegments(t *testing.T) {
	rt, _ := newRT(t)
	ctx := context.Background()
	uid := uint(1)
	require.NoError(t, rt.IncrToday(ctx, model.AnalyticsEvent{VisitorID: "v1", UserID: &uid, IsAuthenticated: true}))
	require.NoError(t, rt.IncrToday(ctx, model.AnalyticsEvent{VisitorID: "v2", IsAuthenticated: false}))
	require.NoError(t, rt.IncrToday(ctx, model.AnalyticsEvent{VisitorID: "v2", IsAuthenticated: false}))

	st, err := rt.TodayCounters(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), st.PV)
	assert.Equal(t, int64(2), st.UV) // identity: user1 + visitor v2
	assert.Equal(t, int64(1), st.RegisteredPV)
	assert.Equal(t, int64(1), st.RegisteredUV)
	assert.Equal(t, int64(2), st.AnonymousPV)
	assert.Equal(t, int64(1), st.AnonymousUV)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/service/analytics/ -run 'TestOnline|TestIncrToday'`
Expected: FAIL。

- [ ] **Step 3: 实现**

`internal/service/analytics/realtime.go`：

```go
package analytics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/internal/model"
)

const onlineKey = "analytics:online"

// TodayStat 今日实时计数快照。
type TodayStat struct {
	PV, UV, RegisteredPV, RegisteredUV, AnonymousPV, AnonymousUV int64
}

// Realtime 维护在线人数与今日三档计数（Redis）。
type Realtime interface {
	TouchOnline(ctx context.Context, visitorID string) error
	OnlineCount(ctx context.Context) (int64, error)
	IncrToday(ctx context.Context, ev model.AnalyticsEvent) error
	TodayCounters(ctx context.Context) (TodayStat, error)
}

type realtime struct {
	rdb          *redis.Client
	tz           *time.Location
	onlineWindow time.Duration
}

// NewRealtime 构造实时层。tz 决定今日 key 的切天口径，onlineWindow 为在线判定时间窗。
func NewRealtime(rdb *redis.Client, tz *time.Location, onlineWindow time.Duration) Realtime {
	return &realtime{rdb: rdb, tz: tz, onlineWindow: onlineWindow}
}

func (r *realtime) today() string { return time.Now().In(r.tz).Format("20060102") }

func (r *realtime) ttl() time.Duration {
	now := time.Now().In(r.tz)
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, r.tz).Add(26 * time.Hour)
	return time.Until(next)
}

func (r *realtime) TouchOnline(ctx context.Context, visitorID string) error {
	now := time.Now().Unix()
	if err := r.rdb.ZAdd(ctx, onlineKey, redis.Z{Score: float64(now), Member: visitorID}).Err(); err != nil {
		return fmt.Errorf("在线表写入失败: %w", err)
	}
	return nil
}

func (r *realtime) OnlineCount(ctx context.Context) (int64, error) {
	min := float64(time.Now().Add(-r.onlineWindow).Unix())
	n, err := r.rdb.ZCount(ctx, onlineKey, strconv.FormatFloat(min, 'f', 0, 64), "+inf").Result()
	if err != nil {
		return 0, fmt.Errorf("在线数统计失败: %w", err)
	}
	return n, nil
}

func (r *realtime) IncrToday(ctx context.Context, ev model.AnalyticsEvent) error {
	day := r.today()
	identity := ev.VisitorID
	if ev.IsAuthenticated && ev.UserID != nil {
		identity = "u:" + strconv.FormatUint(uint64(*ev.UserID), 10)
	}
	pipe := r.rdb.Pipeline()
	pipe.Incr(ctx, "analytics:pv:"+day)
	pipe.SAdd(ctx, "analytics:uv:"+day, identity)
	if ev.IsAuthenticated && ev.UserID != nil {
		pipe.Incr(ctx, "analytics:pv:"+day+":registered")
		pipe.SAdd(ctx, "analytics:uv:"+day+":registered", identity)
	} else {
		pipe.Incr(ctx, "analytics:pv:"+day+":anonymous")
		pipe.SAdd(ctx, "analytics:uv:"+day+":anonymous", ev.VisitorID)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("今日计数写入失败: %w", err)
	}
	// 统一设 TTL（到次日 + 缓冲）
	ttl := r.ttl()
	for _, suffix := range []string{"", ":registered", ":anonymous"} {
		r.rdb.Expire(ctx, "analytics:pv:"+day+suffix, ttl)
		r.rdb.Expire(ctx, "analytics:uv:"+day+suffix, ttl)
	}
	return nil
}

func (r *realtime) TodayCounters(ctx context.Context) (TodayStat, error) {
	day := r.today()
	pv, _ := r.rdb.Get(ctx, "analytics:pv:"+day).Int64()
	uv, _ := r.rdb.SCard(ctx, "analytics:uv:"+day).Result()
	rpv, _ := r.rdb.Get(ctx, "analytics:pv:"+day+":registered").Int64()
	ruv, _ := r.rdb.SCard(ctx, "analytics:uv:"+day+":registered").Result()
	apv, _ := r.rdb.Get(ctx, "analytics:pv:"+day+":anonymous").Int64()
	auv, _ := r.rdb.SCard(ctx, "analytics:uv:"+day+":anonymous").Result()
	return TodayStat{PV: pv, UV: uv, RegisteredPV: rpv, RegisteredUV: ruv, AnonymousPV: apv, AnonymousUV: auv}, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/service/analytics/ -run 'TestOnline|TestIncrToday'`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/service/analytics/realtime.go internal/service/analytics/realtime_test.go
git commit -m "feat(analytics): 新增 Redis 实时在线与今日计数"
```

---

## Task 9: Repository（原始写入 / 聚合读写 / 清理）

**Files:**
- Create: `internal/repository/analytics/repository.go`
- Test: `internal/repository/analytics/repository_test.go`（go-sqlmock）

**Interfaces:**
- Produces:
  - `type Repository interface {`
    - `InsertEvents(ctx, events []model.AnalyticsEvent) error`
    - `UpsertSession(ctx, s model.AnalyticsSession) error`
    - `TouchSession(ctx, sessionID string, lastSeen time.Time) error`
    - `UpsertDaily(ctx, d model.AnalyticsDaily) error`
    - `UpsertDailyDim(ctx, rows []model.AnalyticsDailyDim) error`
    - `UpsertPageDaily(ctx, rows []model.AnalyticsPageDaily) error`
    - `DeleteEventsBefore(ctx, t time.Time) (int64, error)`
    - `DeleteSessionsBefore(ctx, t time.Time) (int64, error)`
  - `}`
  - `func NewRepository(db *gorm.DB) Repository`
- Consumes: `model.*`。
- Note: 本任务只覆盖写入/清理；后台只读查询在 Task 14 加到同一 interface。

- [ ] **Step 1: 写失败测试（覆盖 InsertEvents 与 UpsertDaily）**

`internal/repository/analytics/repository_test.go`：

```go
package analytics_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	repo "github.com/vpt/blog-backend/internal/repository/analytics"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newRepo(t *testing.T) (repo.Repository, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	require.NoError(t, err)
	return repo.NewRepository(gdb), mock
}

func TestInsertEvents(t *testing.T) {
	r, mock := newRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `analytics_events`")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := r.InsertEvents(context.Background(), []model.AnalyticsEvent{{EventType: "page_view", VisitorID: "v"}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertDaily(t *testing.T) {
	r, mock := newRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `analytics_daily`")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := r.UpsertDaily(context.Background(), model.AnalyticsDaily{Date: "2026-06-24", PV: 10, UV: 5})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/repository/analytics/`
Expected: FAIL（包不存在）。

- [ ] **Step 3: 实现**

`internal/repository/analytics/repository.go`：

```go
package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 负责统计原始表写入、聚合表 upsert 与过期清理。
type Repository interface {
	InsertEvents(ctx context.Context, events []model.AnalyticsEvent) error
	UpsertSession(ctx context.Context, s model.AnalyticsSession) error
	TouchSession(ctx context.Context, sessionID string, lastSeen time.Time) error
	UpsertDaily(ctx context.Context, d model.AnalyticsDaily) error
	UpsertDailyDim(ctx context.Context, rows []model.AnalyticsDailyDim) error
	UpsertPageDaily(ctx context.Context, rows []model.AnalyticsPageDaily) error
	DeleteEventsBefore(ctx context.Context, t time.Time) (int64, error)
	DeleteSessionsBefore(ctx context.Context, t time.Time) (int64, error)
}

type repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &repository{db: db} }

func (r *repository) InsertEvents(ctx context.Context, events []model.AnalyticsEvent) error {
	if len(events) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).CreateInBatches(events, 200).Error; err != nil {
		return fmt.Errorf("批量写入事件失败: %w", err)
	}
	return nil
}

func (r *repository) UpsertSession(ctx context.Context, s model.AnalyticsSession) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"last_seen", "pv_count", "exit_path", "duration", "is_bounce",
			"user_id", "is_authenticated",
		}),
	}).Create(&s).Error
	if err != nil {
		return fmt.Errorf("会话 upsert 失败: %w", err)
	}
	return nil
}

func (r *repository) TouchSession(ctx context.Context, sessionID string, lastSeen time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.AnalyticsSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{
			"last_seen": lastSeen,
			"duration":  gorm.Expr("TIMESTAMPDIFF(SECOND, first_seen, ?)", lastSeen),
		})
	if res.Error != nil {
		return fmt.Errorf("会话心跳更新失败: %w", res.Error)
	}
	return nil
}

func (r *repository) UpsertDaily(ctx context.Context, d model.AnalyticsDaily) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}},
		UpdateAll: true,
	}).Create(&d).Error
	if err != nil {
		return fmt.Errorf("日聚合 upsert 失败: %w", err)
	}
	return nil
}

func (r *repository) UpsertDailyDim(ctx context.Context, rows []model.AnalyticsDailyDim) error {
	if len(rows) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "dimension"}, {Name: "dim_value"}},
		UpdateAll: true,
	}).CreateInBatches(rows, 200).Error
	if err != nil {
		return fmt.Errorf("维度聚合 upsert 失败: %w", err)
	}
	return nil
}

func (r *repository) UpsertPageDaily(ctx context.Context, rows []model.AnalyticsPageDaily) error {
	if len(rows) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "path"}},
		UpdateAll: true,
	}).CreateInBatches(rows, 200).Error
	if err != nil {
		return fmt.Errorf("页面聚合 upsert 失败: %w", err)
	}
	return nil
}

func (r *repository) DeleteEventsBefore(ctx context.Context, t time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("created_at < ?", t).Delete(&model.AnalyticsEvent{})
	if res.Error != nil {
		return 0, fmt.Errorf("清理过期事件失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}

func (r *repository) DeleteSessionsBefore(ctx context.Context, t time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("last_seen < ?", t).Delete(&model.AnalyticsSession{})
	if res.Error != nil {
		return 0, fmt.Errorf("清理过期会话失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/repository/analytics/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/repository/analytics/
git commit -m "feat(analytics): 新增统计 repository 写入与清理"
```

---

## Task 10: 有界 channel + ingest 批量落库 worker

**Files:**
- Create: `internal/worker/analytics/ingest.go`
- Test: `internal/worker/analytics/ingest_test.go`

**Interfaces:**
- Produces:
  - `type Ingestor interface { Submit(ev model.AnalyticsEvent) bool; Run(ctx context.Context); Dropped() int64 }`
  - `func NewIngestor(repo repo.Repository, bufferSize, batchSize int, flushInterval time.Duration, logger *zap.Logger) Ingestor`
  - `Submit` 非阻塞，channel 满返回 false 并累加 drop 计数。
- Consumes: Task 9 的 `repo.Repository`。

- [ ] **Step 1: 写失败测试**

`internal/worker/analytics/ingest_test.go`：

```go
package analytics_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	worker "github.com/vpt/blog-backend/internal/worker/analytics"
	"go.uber.org/zap"
)

type fakeRepo struct {
	mu     sync.Mutex
	events []model.AnalyticsEvent
}

func (f *fakeRepo) InsertEvents(_ context.Context, evs []model.AnalyticsEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, evs...)
	return nil
}
func (f *fakeRepo) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.events) }

// 其余 Repository 方法此测试用不到，用嵌入 nil 接口补齐。
type repoStub struct {
	*fakeRepo
	worker.RepoForIngest
}

func TestIngestorFlush(t *testing.T) {
	fr := &fakeRepo{}
	ing := worker.NewIngestor(fr, 16, 8, 20*time.Millisecond, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	go ing.Run(ctx)

	for i := 0; i < 5; i++ {
		assert.True(t, ing.Submit(model.AnalyticsEvent{EventType: "page_view"}))
	}
	require.Eventually(t, func() bool { return fr.count() == 5 }, time.Second, 10*time.Millisecond)
	cancel()
}

func TestIngestorDropWhenFull(t *testing.T) {
	fr := &fakeRepo{}
	ing := worker.NewIngestor(fr, 2, 8, time.Hour, zap.NewNop()) // 不 flush，撑满
	// 不启动 Run，channel 很快满
	ok := 0
	for i := 0; i < 10; i++ {
		if ing.Submit(model.AnalyticsEvent{}) {
			ok++
		}
	}
	assert.LessOrEqual(t, ok, 2)
	assert.GreaterOrEqual(t, ing.Dropped(), int64(8))
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/worker/analytics/ -run TestIngestor`
Expected: FAIL。

- [ ] **Step 3: 实现**

`internal/worker/analytics/ingest.go`：

```go
package analytics

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"go.uber.org/zap"
)

// RepoForIngest 是 ingest worker 依赖的最小写入接口（便于测试）。
type RepoForIngest interface {
	InsertEvents(ctx context.Context, events []model.AnalyticsEvent) error
}

// Ingestor 通过有界 channel 异步批量落库事件。
type Ingestor interface {
	Submit(ev model.AnalyticsEvent) bool
	Run(ctx context.Context)
	Dropped() int64
}

type ingestor struct {
	repo          RepoForIngest
	ch            chan model.AnalyticsEvent
	batchSize     int
	flushInterval time.Duration
	logger        *zap.Logger
	dropped       atomic.Int64
}

// NewIngestor 构造异步落库器。bufferSize 为 channel 容量，满则丢弃。
func NewIngestor(repo RepoForIngest, bufferSize, batchSize int, flushInterval time.Duration, logger *zap.Logger) Ingestor {
	return &ingestor{
		repo:          repo,
		ch:            make(chan model.AnalyticsEvent, bufferSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		logger:        logger,
	}
}

func (i *ingestor) Submit(ev model.AnalyticsEvent) bool {
	select {
	case i.ch <- ev:
		return true
	default:
		i.dropped.Add(1)
		return false
	}
}

func (i *ingestor) Dropped() int64 { return i.dropped.Load() }

func (i *ingestor) Run(ctx context.Context) {
	ticker := time.NewTicker(i.flushInterval)
	defer ticker.Stop()
	buf := make([]model.AnalyticsEvent, 0, i.batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := i.repo.InsertEvents(ctx, buf); err != nil {
			i.logger.Error("统计事件落库失败", zap.Error(err), zap.Int("count", len(buf)))
		}
		buf = buf[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case ev := <-i.ch:
			buf = append(buf, ev)
			if len(buf) >= i.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
```

> 测试中的 `repoStub` 仅为满足类型，实际 `fakeRepo` 已实现 `RepoForIngest`；若编译器提示未用可删除 `repoStub`，核心断言不依赖它。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/worker/analytics/ -run TestIngestor`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/worker/analytics/ingest.go internal/worker/analytics/ingest_test.go
git commit -m "feat(analytics): 新增有界 channel 异步落库 worker"
```

---

## Task 11: collect 编排 service

**Files:**
- Create: `internal/service/analytics/collect.go`
- Test: `internal/service/analytics/collect_test.go`

**Interfaces:**
- Produces:
  - `type CollectService interface { Handle(ctx context.Context, raw RawEvent) error }`
  - `func NewCollectService(enricher Enricher, realtime Realtime, ingestor SessionIngestor, dedup DedupChecker, logger *zap.Logger) CollectService`
  - `type SessionIngestor interface { Submit(ev model.AnalyticsEvent) bool; UpsertSession(ctx, ev model.AnalyticsEvent) error; TouchSession(ctx, sessionID string, lastSeen time.Time) error }`
  - `type DedupChecker interface { IsDuplicatePV(ctx, visitorID, sessionID, path string) (bool, error) }`
- 行为：
  - 富化 → 若 `event_type=heartbeat`：仅 `TouchOnline` + `TouchSession`，不计今日、不入事件表。
  - 若 `page_view`：去重命中则跳过计数与入库（仍 TouchOnline）；非 bot 才 `IncrToday`；`Submit` 事件 + `UpsertSession`。
- Consumes: Task 7/8/9/10。

- [ ] **Step 1: 写失败测试（用 gomock 或手写 fake）**

`internal/service/analytics/collect_test.go`（手写 fake，避免额外 mock 生成）：

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

type fakeIngestor struct {
	submitted int
	upserts   int
	touches   int
}

func (f *fakeIngestor) Submit(model.AnalyticsEvent) bool { f.submitted++; return true }
func (f *fakeIngestor) UpsertSession(context.Context, model.AnalyticsEvent) error {
	f.upserts++
	return nil
}
func (f *fakeIngestor) TouchSession(context.Context, string, time.Time) error { f.touches++; return nil }

type fakeRT struct{ online, incr int }

func (f *fakeRT) TouchOnline(context.Context, string) error             { f.online++; return nil }
func (f *fakeRT) OnlineCount(context.Context) (int64, error)            { return 0, nil }
func (f *fakeRT) IncrToday(context.Context, model.AnalyticsEvent) error { f.incr++; return nil }
func (f *fakeRT) TodayCounters(context.Context) (svc.TodayStat, error)  { return svc.TodayStat{}, nil }

type fakeDedup struct{ dup bool }

func (f *fakeDedup) IsDuplicatePV(context.Context, string, string, string) (bool, error) {
	return f.dup, nil
}

func enr() svc.Enricher { return svc.NewEnricher(fakeGeo{}, "yevpt.com", "salt") }

func TestCollectPageView(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, zap.NewNop())
	err := cs.Handle(context.Background(), svc.RawEvent{
		EventType: "page_view", VisitorID: "v", SessionID: "s", Path: "/", UA: "Chrome",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, ing.submitted)
	assert.Equal(t, 1, ing.upserts)
	assert.Equal(t, 1, rt.incr)
	assert.Equal(t, 1, rt.online)
}

func TestCollectDuplicatePVSkipsCount(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: true}, zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "page_view", VisitorID: "v", SessionID: "s", Path: "/", UA: "Chrome"}))
	assert.Equal(t, 0, ing.submitted)
	assert.Equal(t, 0, rt.incr)
	assert.Equal(t, 1, rt.online) // 在线仍刷新
}

func TestCollectBotNotCounted(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "page_view", VisitorID: "v", SessionID: "s", Path: "/", UA: "Googlebot/2.1"}))
	assert.Equal(t, 0, rt.incr)     // bot 不计今日
	assert.Equal(t, 1, ing.submitted) // 但仍入库（带 is_bot 标记）
}

func TestCollectHeartbeat(t *testing.T) {
	ing, rt := &fakeIngestor{}, &fakeRT{}
	cs := svc.NewCollectService(enr(), rt, ing, &fakeDedup{dup: false}, zap.NewNop())
	require.NoError(t, cs.Handle(context.Background(), svc.RawEvent{
		EventType: "heartbeat", VisitorID: "v", SessionID: "s", UA: "Chrome"}))
	assert.Equal(t, 0, ing.submitted) // 心跳不入事件表
	assert.Equal(t, 1, ing.touches)   // 仅刷新会话
	assert.Equal(t, 1, rt.online)
	assert.Equal(t, 0, rt.incr)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/service/analytics/ -run TestCollect`
Expected: FAIL。

- [ ] **Step 3: 实现**

`internal/service/analytics/collect.go`：

```go
package analytics

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"go.uber.org/zap"
)

// SessionIngestor 抽象事件落库与会话维护（由 worker + repo 适配）。
type SessionIngestor interface {
	Submit(ev model.AnalyticsEvent) bool
	UpsertSession(ctx context.Context, ev model.AnalyticsEvent) error
	TouchSession(ctx context.Context, sessionID string, lastSeen time.Time) error
}

// DedupChecker 判断同一访客/会话/路径短窗口内是否重复 PV。
type DedupChecker interface {
	IsDuplicatePV(ctx context.Context, visitorID, sessionID, path string) (bool, error)
}

// CollectService 编排上报：富化 → 实时层 → 去重 → 入库/会话。
type CollectService interface {
	Handle(ctx context.Context, raw RawEvent) error
}

type collectService struct {
	enricher Enricher
	realtime Realtime
	ingestor SessionIngestor
	dedup    DedupChecker
	logger   *zap.Logger
}

func NewCollectService(e Enricher, rt Realtime, ing SessionIngestor, dedup DedupChecker, logger *zap.Logger) CollectService {
	return &collectService{enricher: e, realtime: rt, ingestor: ing, dedup: dedup, logger: logger}
}

func (s *collectService) Handle(ctx context.Context, raw RawEvent) error {
	ev := s.enricher.Enrich(raw)
	now := time.Now()
	ev.CreatedAt = now

	// 在线始终刷新（含 bot？bot 不应计在线，这里仅非 bot 刷新）
	if !ev.IsBot {
		if err := s.realtime.TouchOnline(ctx, ev.VisitorID); err != nil {
			s.logger.Warn("刷新在线失败", zap.Error(err))
		}
	}

	if raw.EventType == "heartbeat" {
		if err := s.ingestor.TouchSession(ctx, ev.SessionID, now); err != nil {
			s.logger.Warn("心跳更新会话失败", zap.Error(err))
		}
		return nil
	}

	// page_view
	dup := false
	if d, err := s.dedup.IsDuplicatePV(ctx, ev.VisitorID, ev.SessionID, ev.Path); err == nil {
		dup = d
	}

	// 入库始终发生（含 bot/重复，便于审计与会话拼接），但今日计数仅非 bot 且非重复
	s.ingestor.Submit(ev)
	if err := s.ingestor.UpsertSession(ctx, sessionFrom(ev, now)); err != nil {
		s.logger.Warn("会话 upsert 失败", zap.Error(err))
	}
	if !ev.IsBot && !dup {
		if err := s.realtime.IncrToday(ctx, ev); err != nil {
			s.logger.Warn("今日计数失败", zap.Error(err))
		}
	}
	return nil
}

func sessionFrom(ev model.AnalyticsEvent, now time.Time) model.AnalyticsSession {
	return model.AnalyticsSession{
		SessionID:       ev.SessionID,
		VisitorID:       ev.VisitorID,
		UserID:          ev.UserID,
		IsAuthenticated: ev.IsAuthenticated,
		FirstSeen:       now,
		LastSeen:        now,
		PVCount:         1,
		EntryPath:       ev.Path,
		ExitPath:        ev.Path,
		DeviceType:      ev.DeviceType,
		Browser:         ev.Browser,
		OS:              ev.OS,
		Country:         ev.Country,
		Region:          ev.Region,
		RefererType:     ev.RefererType,
		IsBot:           ev.IsBot,
	}
}
```

> 注：`TestCollectDuplicatePVSkipsCount` 断言重复时 `submitted==0`。为满足它，调整实现：去重命中时不 `Submit`、不 `UpsertSession` 自增。将上面 page_view 段改为：先判 `dup`，`if dup { return nil }`（在 TouchOnline 之后），再 Submit/Upsert/IncrToday。按测试为准实现。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/service/analytics/ -run TestCollect`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/service/analytics/collect.go internal/service/analytics/collect_test.go
git commit -m "feat(analytics): 新增 collect 上报编排 service"
```

---

## Task 12: collect handler + 路由 + Origin 校验 + 适配器

**Files:**
- Create: `internal/handler/analytics/collect.go`, `internal/dto/analytics/analytics.go`
- Create: `internal/service/analytics/dedup.go`（基于已有 `service/uv` 适配 `DedupChecker`）
- Create: `internal/worker/analytics/session_adapter.go`（把 `Ingestor`+`repo` 适配成 `SessionIngestor`）
- Modify: `internal/router/router.go`
- Test: `internal/handler/analytics/collect_test.go`（httptest）

**Interfaces:**
- Consumes: `CollectService`、`middleware.VisitorID`、`middleware.OptionalAuth`、`RateLimitNormal`。
- Produces: `POST /collect` 路由；`func NewCollectHandler(svc CollectService, allowedOrigins []string) *CollectHandler`，方法 `Collect(c *gin.Context)`。
- DTO: `type CollectRequest struct { EventType string; Path string; Title string; Referer string; SessionID string; Screen string }`（json tag）。

- [ ] **Step 1: 写 DTO 与适配器**

`internal/dto/analytics/analytics.go`：

```go
package analytics

// CollectRequest 是 /collect 上报载荷（仅可信字段，UA/IP/UserID 由后端注入）。
type CollectRequest struct {
	EventType string `json:"event_type" binding:"required,oneof=page_view heartbeat"`
	Path      string `json:"path"`
	Title     string `json:"title"`
	Referer   string `json:"referer"`
	SessionID string `json:"session_id" binding:"required"`
	Screen    string `json:"screen"`
}
```

`internal/service/analytics/dedup.go`：

```go
package analytics

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/service/uv"
)

const pvDedupWindow = 5 * time.Second

type uvDedup struct{ uv uv.UVService }

// NewDedupChecker 用已有 UV 去重服务实现短窗口 PV 去重。
func NewDedupChecker(u uv.UVService) DedupChecker { return &uvDedup{uv: u} }

func (d *uvDedup) IsDuplicatePV(ctx context.Context, visitorID, sessionID, path string) (bool, error) {
	isNew, err := d.uv.CheckAndMark(ctx, "analytics:pv:dedup", sessionID+"|"+path, visitorID, pvDedupWindow)
	if err != nil {
		return false, err
	}
	return !isNew, nil // 非新 = 重复
}
```

`internal/worker/analytics/session_adapter.go`：

```go
package analytics

import (
	"context"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	repo "github.com/vpt/blog-backend/internal/repository/analytics"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

// sessionIngestor 把异步 Ingestor 与同步会话写入合并为 svc.SessionIngestor。
type sessionIngestor struct {
	ing  Ingestor
	repo repo.Repository
}

// NewSessionIngestor 适配 collect service 所需的 SessionIngestor。
func NewSessionIngestor(ing Ingestor, r repo.Repository) svc.SessionIngestor {
	return &sessionIngestor{ing: ing, repo: r}
}

func (s *sessionIngestor) Submit(ev model.AnalyticsEvent) bool { return s.ing.Submit(ev) }
func (s *sessionIngestor) UpsertSession(ctx context.Context, ev model.AnalyticsSession) error {
	return s.repo.UpsertSession(ctx, ev)
}
func (s *sessionIngestor) TouchSession(ctx context.Context, sessionID string, lastSeen time.Time) error {
	return s.repo.TouchSession(ctx, sessionID, lastSeen)
}
```

> 注意：`svc.SessionIngestor.UpsertSession` 的签名在 Task 11 用了 `model.AnalyticsEvent`，此处会话来自 `sessionFrom`。统一为：collect service 内部构造 `AnalyticsSession` 后调用 `UpsertSession(ctx, session)`。据此把 Task 11 接口的 `UpsertSession(ctx, ev model.AnalyticsEvent)` 改为 `UpsertSession(ctx, s model.AnalyticsSession)`，并在 collect service 内调用 `s.ingestor.UpsertSession(ctx, sessionFrom(ev, now))`。本任务以该统一签名为准。

- [ ] **Step 2: 写失败测试（handler）**

`internal/handler/analytics/collect_test.go`：

```go
package analytics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hdl "github.com/vpt/blog-backend/internal/handler/analytics"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

type captureSvc struct{ got svc.RawEvent; called bool }

func (c *captureSvc) Handle(_ context.Context, raw svc.RawEvent) error {
	c.got = raw
	c.called = true
	return nil
}

func TestCollectHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cs := &captureSvc{}
	h := hdl.NewCollectHandler(cs, []string{"https://www.yevpt.com"})

	r := gin.New()
	r.POST("/collect", func(c *gin.Context) { c.Set("visitor_id", "v1") }, h.Collect)

	body := `{"event_type":"page_view","path":"/a","session_id":"s1"}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://www.yevpt.com")
	req.Header.Set("User-Agent", "Chrome")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.True(t, cs.called)
	assert.Equal(t, "v1", cs.got.VisitorID)
	assert.Equal(t, "page_view", cs.got.EventType)
}

func TestCollectHandlerBadOriginMarksSuspect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cs := &captureSvc{}
	h := hdl.NewCollectHandler(cs, []string{"https://www.yevpt.com"})
	r := gin.New()
	r.POST("/collect", func(c *gin.Context) { c.Set("visitor_id", "v1") }, h.Collect)

	body := `{"event_type":"page_view","path":"/a","session_id":"s1"}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.True(t, cs.got.Origin == "https://evil.com")
}
```

> suspect 标记最终在 service/enrich 里依据 Origin 是否匹配置位；handler 仅透传 Origin 与匹配结果。简化起见：handler 把「Origin 是否匹配」算好后塞进 RawEvent（给 RawEvent 增 `OriginAllowed bool` 字段，enrich 据此置 `IsSuspect`）。据此在 Task 7 的 RawEvent 增 `OriginAllowed bool`，enrich 设 `IsSuspect = !raw.OriginAllowed`（仅当配置了白名单时才判定，空白名单恒 allowed）。

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/handler/analytics/ -run TestCollectHandler`
Expected: FAIL。

- [ ] **Step 4: 实现 handler**

`internal/handler/analytics/collect.go`：

```go
package analytics

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vpt/blog-backend/internal/middleware"
	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

// CollectHandler 处理 /collect 上报。
type CollectHandler struct {
	svc            svc.CollectService
	allowedOrigins map[string]struct{}
}

// NewCollectHandler 构造。allowedOrigins 为空表示不校验（开发环境）。
func NewCollectHandler(s svc.CollectService, allowedOrigins []string) *CollectHandler {
	set := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o != "" {
			set[o] = struct{}{}
		}
	}
	return &CollectHandler{svc: s, allowedOrigins: set}
}

// Collect 接收上报，校验/富化交由 service，统一返回 204。
// @Summary  站点访问上报
// @Tags     analytics
// @Accept   json
// @Param    body body dto.CollectRequest true "上报载荷"
// @Success  204
// @Router   /collect [post]
func (h *CollectHandler) Collect(c *gin.Context) {
	var req dto.CollectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Status(http.StatusNoContent) // 上报失败不回错，避免暴露细节/影响前台
		return
	}
	raw := svc.RawEvent{
		EventType:     req.EventType,
		VisitorID:     middleware.GetVisitorID(c),
		SessionID:     req.SessionID,
		Path:          req.Path,
		Title:         req.Title,
		Referer:       req.Referer,
		UA:            c.Request.UserAgent(),
		IP:            c.ClientIP(),
		Origin:        c.GetHeader("Origin"),
		OriginAllowed: h.originAllowed(c.GetHeader("Origin")),
		UserID:        userIDFromContext(c),
	}
	_ = h.svc.Handle(c.Request.Context(), raw)
	c.Status(http.StatusNoContent)
}

func (h *CollectHandler) originAllowed(origin string) bool {
	if len(h.allowedOrigins) == 0 {
		return true // 未配置白名单（开发）→ 放行
	}
	_, ok := h.allowedOrigins[origin]
	return ok
}

// userIDFromContext 从 OptionalAuth 写入的 Claims 取 user_id，未登录返回 nil。
func userIDFromContext(c *gin.Context) *uint {
	v, ok := c.Get(middleware.ClaimsContextKey)
	if !ok {
		return nil
	}
	claims, ok := v.(*jwtClaims)
	if !ok {
		return nil
	}
	id := claims.UserID
	return &id
}
```

> `middleware.ClaimsContextKey` 与 claims 类型以 `internal/middleware/auth.go` 现有实现为准（OptionalAuth 把解析出的 claims 存入 context 的 key 与类型）。实现时读现有 `auth.go`，用其导出的取值辅助（若有 `jwt.GetClaims(c)` 之类则直接用，替换上面手写部分）。

- [ ] **Step 5: 注册路由**

`internal/router/router.go` 公开路由区（参照 `RateLimitNormal` 用法）加入：

```go
	r.POST("/collect",
		middleware.VisitorID(),
		middleware.OptionalAuth(jwtManager),
		middleware.RateLimitNormal(redisClient),
		handlers.analyticsCollect.Collect,
	)
```

依赖组装（在 router 组装区，参照 `uvSvc := uv.NewService(redisClient)` 附近）：

```go
	geo := analyticssvc.NewGeoResolver(cfg.Analytics.GeoIPPath, zapLogger)
	enricher := analyticssvc.NewEnricher(geo, cfg.Analytics.SiteHost, cfg.Analytics.IPSalt)
	analyticsRepo := analyticsrepo.NewRepository(db)
	tz, _ := time.LoadLocation(cfg.Analytics.Timezone)
	realtime := analyticssvc.NewRealtime(redisClient, tz, cfg.Analytics.OnlineWindow)
	ingestor := analyticsworker.NewIngestor(analyticsRepo, cfg.Analytics.ChannelBuffer, 100, 2*time.Second, zapLogger)
	sessionIng := analyticsworker.NewSessionIngestor(ingestor, analyticsRepo)
	dedup := analyticssvc.NewDedupChecker(uvSvc)
	collectSvc := analyticssvc.NewCollectService(enricher, realtime, sessionIng, dedup, zapLogger)
	// handlers.analyticsCollect = analyticshdl.NewCollectHandler(collectSvc, splitCORSOrigins(os.Getenv("ANALYTICS_ALLOWED_ORIGINS")))
```

（`handlers` 结构体增 `analyticsCollect *analyticshdl.CollectHandler` 字段；ingestor 的 `Run` 在 bootstrap 启动，见 Task 13。）

- [ ] **Step 6: 跑测试 + 编译**

Run: `go test ./internal/handler/analytics/ && go build ./...`
Expected: PASS，编译通过。

- [ ] **Step 7: Commit**

```bash
git add internal/handler/analytics/ internal/dto/analytics/ internal/service/analytics/dedup.go internal/worker/analytics/session_adapter.go internal/router/router.go
git commit -m "feat(analytics): 新增 collect 上报接口与路由"
```

---

## Task 13: 聚合 worker（日滚动 + 清理）+ bootstrap 启动

**Files:**
- Create: `internal/worker/analytics/rollup.go`
- Modify: `internal/bootstrap/bootstrap.go`, `cmd/server/main.go`
- Test: `internal/worker/analytics/rollup_test.go`

**Interfaces:**
- Produces:
  - `type Rollup interface { RollupDay(ctx context.Context, date string) error; Cleanup(ctx context.Context, retentionDays int) error }`
  - `func NewRollup(reader RollupReader, repo repo.Repository, tz *time.Location, logger *zap.Logger) Rollup`
  - `type RollupReader interface { AggregateDay(ctx context.Context, date string) (DayAggregate, error) }`（从原始表读出某天聚合结果，SQL 实现）
  - `type DayAggregate struct { Daily model.AnalyticsDaily; Dims []model.AnalyticsDailyDim; Pages []model.AnalyticsPageDaily }`
  - `func NewWorker(...) *Worker` + `Run(ctx)`：ticker 每分钟检查，跨过 00:30（Asia/Shanghai）时聚合昨天（带 MySQL 租约）；每日清理。
- Consumes: Task 9 repo。

> 聚合读 `AggregateDay` 用原生 SQL 在 `RollupReader` 实现（COUNT/COUNT DISTINCT，按 `is_bot=0 AND is_suspect=0` 过滤，三档 UV 按 `COALESCE(user_id, visitor_id)` / `user_id` / `visitor_id` 去重）。该 SQL 单元在 repository 包内，用 sqlmock 测一条 happy path。

- [ ] **Step 1: 写失败测试（RollupDay 调用 reader 与 repo upsert）**

`internal/worker/analytics/rollup_test.go`：

```go
package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	worker "github.com/vpt/blog-backend/internal/worker/analytics"
	"go.uber.org/zap"
)

type fakeReader struct{ date string }

func (f *fakeReader) AggregateDay(_ context.Context, date string) (worker.DayAggregate, error) {
	f.date = date
	return worker.DayAggregate{
		Daily: model.AnalyticsDaily{Date: date, PV: 10, UV: 4, RegisteredUV: 1, AnonymousUV: 3},
		Dims:  []model.AnalyticsDailyDim{{Date: date, Dimension: "device", DimValue: "desktop", PV: 10, UV: 4}},
		Pages: []model.AnalyticsPageDaily{{Date: date, Path: "/", PV: 6, UV: 3}},
	}, nil
}

type recRepo struct {
	daily int
	dims  int
	pages int
}

func (r *recRepo) UpsertDaily(context.Context, model.AnalyticsDaily) error { r.daily++; return nil }
func (r *recRepo) UpsertDailyDim(context.Context, []model.AnalyticsDailyDim) error {
	r.dims++
	return nil
}
func (r *recRepo) UpsertPageDaily(context.Context, []model.AnalyticsPageDaily) error {
	r.pages++
	return nil
}

func TestRollupDay(t *testing.T) {
	fr := &fakeReader{}
	rr := &recRepo{}
	rollup := worker.NewRollup(fr, rr, time.UTC, zap.NewNop())
	require.NoError(t, rollup.RollupDay(context.Background(), "2026-06-24"))
	assert.Equal(t, "2026-06-24", fr.date)
	assert.Equal(t, 1, rr.daily)
	assert.Equal(t, 1, rr.dims)
	assert.Equal(t, 1, rr.pages)
}
```

> `recRepo` 仅实现 Rollup 用到的三个 upsert 方法；`NewRollup` 的 repo 参数类型应是一个最小接口 `RollupWriter`（含这三个方法），便于测试。`repo.Repository` 满足之。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/worker/analytics/ -run TestRollupDay`
Expected: FAIL。

- [ ] **Step 3: 实现 rollup.go**

`internal/worker/analytics/rollup.go`：

```go
package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"go.uber.org/zap"
)

// DayAggregate 是某天从原始表算出的聚合结果。
type DayAggregate struct {
	Daily model.AnalyticsDaily
	Dims  []model.AnalyticsDailyDim
	Pages []model.AnalyticsPageDaily
}

// RollupReader 从原始表读出某天聚合（SQL 实现）。
type RollupReader interface {
	AggregateDay(ctx context.Context, date string) (DayAggregate, error)
}

// RollupWriter 写入聚合表（repo.Repository 满足）。
type RollupWriter interface {
	UpsertDaily(ctx context.Context, d model.AnalyticsDaily) error
	UpsertDailyDim(ctx context.Context, rows []model.AnalyticsDailyDim) error
	UpsertPageDaily(ctx context.Context, rows []model.AnalyticsPageDaily) error
}

// Rollup 执行日聚合。
type Rollup interface {
	RollupDay(ctx context.Context, date string) error
}

type rollup struct {
	reader RollupReader
	writer RollupWriter
	tz     *time.Location
	logger *zap.Logger
}

func NewRollup(reader RollupReader, writer RollupWriter, tz *time.Location, logger *zap.Logger) Rollup {
	return &rollup{reader: reader, writer: writer, tz: tz, logger: logger}
}

// RollupDay 幂等聚合指定日期（YYYY-MM-DD）。
func (r *rollup) RollupDay(ctx context.Context, date string) error {
	agg, err := r.reader.AggregateDay(ctx, date)
	if err != nil {
		return fmt.Errorf("读取 %s 聚合失败: %w", date, err)
	}
	if err := r.writer.UpsertDaily(ctx, agg.Daily); err != nil {
		return err
	}
	if err := r.writer.UpsertDailyDim(ctx, agg.Dims); err != nil {
		return err
	}
	if err := r.writer.UpsertPageDaily(ctx, agg.Pages); err != nil {
		return err
	}
	r.logger.Info("日聚合完成", zap.String("date", date), zap.Int("pv", agg.Daily.PV))
	return nil
}
```

- [ ] **Step 4: 实现调度 Worker（含租约 + 清理）与 bootstrap 启动**

在 `rollup.go` 追加调度器（或单独 `worker.go`）。调度器每分钟 tick，记录上次聚合日期，跨 00:30 时对昨天 `RollupDay`；用 `repo` 的一个租约表/键防多实例（可复用通知 worker 的租约模式或用 Redis SETNX 锁 `analytics:rollup:lock:<date>`）。同时启动 `Ingestor.Run` 与每日 `Cleanup`（调用 `DeleteEventsBefore`/`DeleteSessionsBefore` + Redis `ZREMRANGEBYSCORE` 清在线）。

`internal/bootstrap/bootstrap.go` 增 `StartAnalyticsWorker(ctx, cfg, db, redisClient, ingestor, rollup, repo, zapLogger)`，内部 `go ingestor.Run(ctx)` 与 `go scheduler.Run(ctx)`，仿 `StartNotificationWorker`。`cmd/server/main.go` 增调用。

> 实现期：调度细节（租约 key、清理时刻）参照 `internal/worker/notification/worker.go` 的 ticker+租约写法落地；本步交付物是「server 启动后 ingestor 在跑、每日 00:30 自动聚合昨天、每日清理过期原始数据」。

- [ ] **Step 5: 跑测试 + 编译 + 启动冒烟**

Run: `go test ./internal/worker/analytics/ && go build ./...`
Expected: PASS，编译通过。

- [ ] **Step 6: Commit**

```bash
git add internal/worker/analytics/rollup.go internal/bootstrap/bootstrap.go cmd/server/main.go
git commit -m "feat(analytics): 新增日聚合 worker 与启动装配"
```

---

## Task 14: 后台只读查询（repo + service）

**Files:**
- Modify: `internal/repository/analytics/repository.go`（增查询方法 + `AggregateDay` 的 SQL 实现）
- Create: `internal/service/analytics/query.go`
- Test: `internal/repository/analytics/query_test.go`, `internal/service/analytics/query_test.go`

**Interfaces:**
- Repository 增：
  - `QueryDailyRange(ctx, from, to string) ([]model.AnalyticsDaily, error)`
  - `QueryDimRange(ctx, dimension, from, to string) ([]model.AnalyticsDailyDim, error)`
  - `QueryTopPages(ctx, from, to string, limit int) ([]model.AnalyticsPageDaily, error)`
  - `AggregateDay(ctx, date string) (worker.DayAggregate, error)`（实现 Task 13 的 RollupReader；为避免循环依赖，DayAggregate 类型放在 repo 包或共享包，import 方向由实现期定——建议把 `DayAggregate/RollupReader` 定义移到 repo 包，worker 引用 repo）。
- Service 增（query.go）：
  - `type QueryService interface { Overview(ctx) (dto.Overview, error); Trend(ctx, from, to, metric, segment string) ([]dto.TrendPoint, error); TopPages(ctx, from, to string, limit int) ([]dto.PageStat, error) }`
  - 组合 repo + realtime（今日数 + 在线）。

- [ ] **Step 1: 写 repo 查询失败测试**

`internal/repository/analytics/query_test.go`（覆盖 `QueryDailyRange`）：

```go
package analytics_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryDailyRange(t *testing.T) {
	r, mock := newRepo(t)
	rows := sqlmock.NewRows([]string{"date", "pv", "uv"}).
		AddRow("2026-06-23", 10, 5).
		AddRow("2026-06-24", 20, 8)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `analytics_daily`")).
		WillReturnRows(rows)

	got, err := r.QueryDailyRange(context.Background(), "2026-06-23", "2026-06-24")
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, 20, got[1].PV)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/repository/analytics/ -run TestQueryDailyRange`
Expected: FAIL（方法未定义）。

- [ ] **Step 3: 实现 repo 查询方法**

在 `repository.go` 追加（接口 + 实现）：

```go
func (r *repository) QueryDailyRange(ctx context.Context, from, to string) ([]model.AnalyticsDaily, error) {
	var out []model.AnalyticsDaily
	err := r.db.WithContext(ctx).Where("date >= ? AND date <= ?", from, to).Order("date asc").Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("查询日趋势失败: %w", err)
	}
	return out, nil
}

func (r *repository) QueryDimRange(ctx context.Context, dimension, from, to string) ([]model.AnalyticsDailyDim, error) {
	var out []model.AnalyticsDailyDim
	err := r.db.WithContext(ctx).
		Where("dimension = ? AND date >= ? AND date <= ?", dimension, from, to).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("查询维度失败: %w", err)
	}
	return out, nil
}

func (r *repository) QueryTopPages(ctx context.Context, from, to string, limit int) ([]model.AnalyticsPageDaily, error) {
	var out []model.AnalyticsPageDaily
	err := r.db.WithContext(ctx).
		Select("path, max(title) as title, sum(pv) as pv, sum(uv) as uv").
		Where("date >= ? AND date <= ?", from, to).
		Group("path").Order("pv desc").Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("查询热门页面失败: %w", err)
	}
	return out, nil
}
```

`AggregateDay` 的 SQL 实现（原始表 → DayAggregate）：用多条聚合查询（`COUNT(*)` 为 PV、`COUNT(DISTINCT COALESCE(user_id, visitor_id))` 为全部 UV，等等），`WHERE created_at` 落在该天（按 tz 转 UTC 区间）且 `is_bot=0 AND is_suspect=0`。返回组装好的 `DayAggregate`。维度循环 `device/browser/os/referer_type/country/user_type` 各跑一条 `GROUP BY`。本步同时让该方法满足 worker 的 `RollupReader`。

- [ ] **Step 4: 实现 query service + DTO**

`internal/dto/analytics/analytics.go` 增：

```go
type Overview struct {
	TodayPV   int64 `json:"today_pv"`
	TodayUV   int64 `json:"today_uv"`
	Online    int64 `json:"online"`
	TotalPV   int64 `json:"total_pv"`
	TotalUV   int64 `json:"total_uv"`
	Registered SegmentStat `json:"registered"`
	Anonymous  SegmentStat `json:"anonymous"`
}
type SegmentStat struct {
	TodayPV int64 `json:"today_pv"`
	TodayUV int64 `json:"today_uv"`
}
type TrendPoint struct {
	Date string `json:"date"`
	Value int   `json:"value"`
}
type PageStat struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	PV    int    `json:"pv"`
	UV    int    `json:"uv"`
}
```

`internal/service/analytics/query.go` 实现 `QueryService`，`Overview` 合并 `realtime.TodayCounters` + `realtime.OnlineCount` + repo 的累计求和；`Trend` 按 metric/segment 从 `QueryDailyRange` 取对应字段映射成 `[]TrendPoint`；`TopPages` 映射 `QueryTopPages`。配 service 层单测（gomock 或手写 fake repo/realtime）覆盖 Trend 的 segment 字段选择与边界。

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/repository/analytics/ ./internal/service/analytics/`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/repository/analytics/ internal/service/analytics/query.go internal/dto/analytics/
git commit -m "feat(analytics): 新增后台统计查询 service 与聚合读"
```

---

## Task 15: 后台查询 handler + 路由 + Swagger

**Files:**
- Create: `internal/handler/analytics/admin.go`
- Modify: `internal/router/router.go`
- Test: `internal/handler/analytics/admin_test.go`

**Interfaces:**
- Consumes: `QueryService`。
- Produces: `func NewAdminHandler(q QueryService) *AdminHandler`，方法 `Overview/Trend/Pages`；路由挂在 `admin` 分组下。

- [ ] **Step 1: 写失败测试（Overview happy path + Trend 参数校验）**

`internal/handler/analytics/admin_test.go`：

```go
package analytics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dto "github.com/vpt/blog-backend/internal/dto/analytics"
	hdl "github.com/vpt/blog-backend/internal/handler/analytics"
)

type fakeQuery struct{}

func (fakeQuery) Overview(context.Context) (dto.Overview, error) {
	return dto.Overview{TodayPV: 10, Online: 2}, nil
}
func (fakeQuery) Trend(context.Context, string, string, string, string) ([]dto.TrendPoint, error) {
	return []dto.TrendPoint{{Date: "2026-06-24", Value: 10}}, nil
}
func (fakeQuery) TopPages(context.Context, string, string, int) ([]dto.PageStat, error) {
	return []dto.PageStat{{Path: "/", PV: 5}}, nil
}

func TestAdminOverview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := hdl.NewAdminHandler(fakeQuery{})
	r := gin.New()
	r.GET("/admin/analytics/overview", h.Overview)

	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "today_pv")
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/handler/analytics/ -run TestAdmin`
Expected: FAIL。

- [ ] **Step 3: 实现 handler**

`internal/handler/analytics/admin.go`：用 `pkg/response` 统一返回；`Trend` 解析 `from/to/metric/segment`（默认近 7 天、metric 默认 pv、segment 默认 all），`metric ∈ {pv,uv,sessions}`、`segment ∈ {all,registered,anonymous}` 非法返回 422；`Pages` 解析 `limit`（默认 20、上限 100）、`from/to`，跨度上限 365 天。每个方法带 Swagger 注解（`@Tags analytics`、`@Security BearerAuth`、`@Success 200 {object} dto.Overview` 等）。

- [ ] **Step 4: 注册路由**

`internal/router/router.go` 的 `admin` 分组内：

```go
	admin.GET("/analytics/overview", handlers.analyticsAdmin.Overview)
	admin.GET("/analytics/trend", handlers.analyticsAdmin.Trend)
	admin.GET("/analytics/pages", handlers.analyticsAdmin.Pages)
```

（`handlers` 增 `analyticsAdmin *analyticshdl.AdminHandler` 并在组装区构造。）

- [ ] **Step 5: 跑测试 + swag**

Run: `go test ./internal/handler/analytics/ && make swag && go build ./...`
Expected: PASS，swagger 生成无错。

- [ ] **Step 6: Commit**

```bash
git add internal/handler/analytics/admin.go internal/router/router.go docs/
git commit -m "feat(analytics): 新增后台统计查询接口"
```

---

## Task 16: 配置项与部署

**Files:**
- Modify: `pkg/config`（配置结构体）, `config/config.yaml` / `config.prod.yaml` / `config.local.yaml.example` / `config.test.yaml`
- Modify: `docker-compose.yml`, `.env.example`
- Test: `pkg/config` 现有测试（若有）保持通过

**Interfaces:**
- Produces: `cfg.Analytics` 结构：`Timezone string`、`RetentionDays int`、`OnlineWindow time.Duration`、`SessionTimeout time.Duration`、`BounceDuration time.Duration`、`ChannelBuffer int`、`PublicCacheTTL time.Duration`、`GeoIPPath string`、`SiteHost string`、`IPSalt string`。`ANALYTICS_ALLOWED_ORIGINS` 仍走 `os.Getenv`（与现有 CORS env 风格一致）。

- [ ] **Step 1: 加配置结构**

在 config 结构体增 `Analytics` 块及默认值（viper 默认或 yaml）。`config.yaml` 示例：

```yaml
analytics:
  timezone: "Asia/Shanghai"
  retention_days: 90
  online_window: 90s
  session_timeout: 30m
  bounce_duration: 10s
  channel_buffer: 4096
  public_cache_ttl: 60s
  geoip_path: ""          # 容器内 ip2region xdb 路径，空则关闭地理解析
  site_host: "yevpt.com"
  ip_salt: "change_me"    # 生产用随机串，经 env 覆盖
```

- [ ] **Step 2: 部署接线**

`docker-compose.yml` 的 `blog-server.environment` 增：

```yaml
      ANALYTICS_ALLOWED_ORIGINS: ${ANALYTICS_ALLOWED_ORIGINS:-}
      BLOG_ANALYTICS_GEOIP_PATH: ${ANALYTICS_GEOIP_PATH:-}
      BLOG_ANALYTICS_IP_SALT: ${ANALYTICS_IP_SALT:-}
```

`volumes` 增（若启用地理）：`- ./geoip:/app/geoip:ro`（放 `ip2region.xdb`，并把 `geoip_path` 设为 `/app/geoip/ip2region.xdb`）。

`.env.example` 增：

```bash
# 站点分析
ANALYTICS_ALLOWED_ORIGINS=https://www.yevpt.com,https://yevpt.com
ANALYTICS_GEOIP_PATH=/app/geoip/ip2region.xdb
ANALYTICS_IP_SALT=change_me_random_string
```

- [ ] **Step 3: 验证配置加载**

Run: `go build ./... && go test ./pkg/config/...`
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add pkg/config config/ docker-compose.yml .env.example
git commit -m "chore(analytics): 新增统计配置项与部署接线"
```

---

## Self-Review

**Spec coverage：**
- 采集 `/collect`、富化、botfilter、Origin 校验 → Task 5/7/12 ✔
- 注册/匿名识别（OptionalAuth + user_id + 三档）→ Task 12（取 claims）、Task 7（is_authenticated）、Task 8（三档计数）、Task 14（三档查询）✔
- 原始表 + 会话 + 三聚合表 → Task 1/9 ✔
- 有界 channel 异步落库 → Task 10 ✔
- Redis 在线 + 今日热计数（可重建）→ Task 8 ✔
- 日聚合幂等 + 清理 → Task 13/14 ✔
- 后台 overview/trend/pages → Task 15 ✔
- 隐私（ip_hash、path 脱敏、user_id 仅后台）→ Task 4/7/15 ✔
- 配置 + 部署（ANALYTICS_ALLOWED_ORIGINS、geoip、时区）→ Task 16 ✔
- 公开 API（summary/popular）、dimensions 接口、回填工具 → **归 Phase 2**，本计划不含（与 spec 分期一致）。

**接口一致性修正（实现期以此为准）：**
- `svc.SessionIngestor.UpsertSession(ctx, s model.AnalyticsSession)`（Task 12 统一签名，覆盖 Task 11 早期写法）。
- `RawEvent` 增 `OriginAllowed bool`（Task 12），`enrich` 据此设 `IsSuspect`（Task 7 实现时补该字段映射）。
- `DayAggregate`/`RollupReader` 定义建议落在 `repository/analytics` 包，`worker/analytics` 引用，避免循环依赖（Task 13/14）。

**Placeholder 扫描：** 纯函数任务（2-8）含完整代码；编排/装配任务（11-16）含核心代码 + 明确的「以现有 X 为准」落地点，无 TODO/TBD。

---

## Execution Handoff

见对话中的执行方式选择。
