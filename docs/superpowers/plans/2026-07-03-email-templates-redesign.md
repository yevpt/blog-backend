# 邮件通知模板重新设计 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把验证码邮件、通知摘要邮件、审核提醒邮件三处朴素 HTML 模板统一改造为极简卡片风视觉，并让品牌名/站点地址/审核后台地址可配置。

**Architecture:** 新增 `pkg/email/layout` 共享包渲染统一的邮件外壳（品牌 Logo、白色卡片、CTA 按钮、版权行），三处现有渲染函数（`pkg/email/email.go`、`internal/service/notification/email_template.go`、`internal/service/moderationemail/template.go`）只负责生成卡片正文片段并调用 `layout.Wrap`。验证码邮件按场景（注册/重置密码/绑定邮箱）区分文案，通过 `email.Purpose` 参数驱动。

**Tech Stack:** Go 1.25+、`html/template`（自动转义）、`gomail.v2`、`testify`（assert/require）、`gomock`、`go-sqlmock`（本次不涉及 repository 层，无需使用）。

## Global Constraints

- 生产代码禁止 `fmt.Println`，日志用 `zap.Logger`（本次改动均为纯函数/构造注入，不新增日志调用）。
- 禁止直接返回 `model.*` 给前端或写进 Swagger（本次不涉及 HTTP handler/DTO）。
- 禁用全局变量保存 db/redis/logger/mailer，一律构造注入（新增的 `brandName`/`adminURL` 参数同样走构造函数注入，不使用全局变量）。
- 测试文件 `xxx_test.go`；默认包名加 `_test` 后缀做黑盒测试，仅在确需访问未导出实现时使用同包内部测试。
- 改动的每个包完成后跑该包测试；全部任务完成后跑一次 `go test ./...`。
- 三处邮件渲染函数已转义的用户内容必须继续通过 `html/template` 或 `html.EscapeString` 转义，不能出现原始用户输入直接拼进 HTML。
- 颜色/尺寸等视觉常量以 spec 文档 [2026-07-03-email-templates-redesign-design.md](../specs/2026-07-03-email-templates-redesign-design.md) 为准，禁止随意改动色值。

---

### Task 1: 共享 Layout 包

**Files:**
- Create: `pkg/email/layout/layout.go`
- Create: `pkg/email/layout/layout_test.go`

**Interfaces:**
- Produces:
  - `const layout.DefaultBrandName = "YEVPT"`
  - `const layout.DefaultSiteURL = "https://www.yevpt.com"`
  - `type layout.Brand struct { Name, SiteURL string }`
  - `func layout.ResolveBrand(name, siteURL string) Brand`
  - `type layout.Options struct { Brand Brand; Title string; BodyHTML template.HTML; CTAText string; CTAURL string }`
  - `func layout.Wrap(opts Options) string`

- [ ] **Step 1: 写失败的测试**

创建 `pkg/email/layout/layout_test.go`：

```go
package layout_test

import (
	"html/template"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/vpt/blog-backend/pkg/email/layout"
)

func TestResolveBrand_FallsBackToDefaultsWhenEmpty(t *testing.T) {
	brand := layout.ResolveBrand("", "")

	assert.Equal(t, layout.DefaultBrandName, brand.Name)
	assert.Equal(t, layout.DefaultSiteURL, brand.SiteURL)
}

func TestResolveBrand_KeepsConfiguredValuesAndTrimsTrailingSlash(t *testing.T) {
	brand := layout.ResolveBrand("  测试站  ", "https://test.example.com/")

	assert.Equal(t, "测试站", brand.Name)
	assert.Equal(t, "https://test.example.com", brand.SiteURL)
}

func TestWrap_RendersBrandTitleAndBody(t *testing.T) {
	html := layout.Wrap(layout.Options{
		Brand:    layout.Brand{Name: "YEVPT", SiteURL: "https://www.yevpt.com"},
		Title:    "注册验证码",
		BodyHTML: template.HTML(`<p>hello</p>`),
	})

	assert.Contains(t, html, `href="https://www.yevpt.com"`)
	assert.Contains(t, html, ">YEVPT<")
	assert.Contains(t, html, "注册验证码")
	assert.Contains(t, html, "<p>hello</p>")
	assert.Contains(t, html, "&copy;")
	assert.Contains(t, html, strconv.Itoa(time.Now().Year()))
	assert.NotContains(t, html, "打开审核后台") // 没有传 CTAText 时不渲染按钮
}

func TestWrap_RendersCTAButtonWhenProvided(t *testing.T) {
	html := layout.Wrap(layout.Options{
		Brand:    layout.Brand{Name: "YEVPT", SiteURL: "https://www.yevpt.com"},
		Title:    "标题",
		BodyHTML: template.HTML(`<p>正文</p>`),
		CTAText:  "查看全部通知",
		CTAURL:   "https://www.yevpt.com/notifications",
	})

	assert.Contains(t, html, "查看全部通知")
	assert.Contains(t, html, `href="https://www.yevpt.com/notifications"`)
}

func TestWrap_EscapesTitleButKeepsBodyHTMLRaw(t *testing.T) {
	html := layout.Wrap(layout.Options{
		Brand:    layout.Brand{Name: "YEVPT", SiteURL: "https://www.yevpt.com"},
		Title:    `<script>alert(1)</script>`,
		BodyHTML: template.HTML(`<p>安全内容</p>`),
	})

	assert.NotContains(t, html, "<script>alert(1)</script>")
	assert.Contains(t, html, "&lt;script&gt;")
	assert.Contains(t, html, "<p>安全内容</p>")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./pkg/email/layout/...`
Expected: 编译失败，报 `no required module provides package .../pkg/email/layout`（包还不存在）。

- [ ] **Step 3: 实现 layout 包**

创建 `pkg/email/layout/layout.go`：

```go
// Package layout 提供邮件正文的统一视觉外壳（品牌 Logo、卡片、CTA 按钮、版权行），
// 供验证码、通知摘要、审核提醒等场景复用，避免各处重复维护一份 CSS。
package layout

import (
	"html/template"
	"strings"
	"time"
)

// DefaultBrandName 和 DefaultSiteURL 是品牌名/站点地址留空时的兜底默认值。
const (
	DefaultBrandName = "YEVPT"
	DefaultSiteURL   = "https://www.yevpt.com"
)

// Brand 是邮件顶部品牌标识与跳转目标。
type Brand struct {
	Name    string
	SiteURL string
}

// ResolveBrand 对品牌名和站点地址应用兜底默认值，并去掉站点地址末尾多余的斜杠。
func ResolveBrand(name, siteURL string) Brand {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultBrandName
	}
	siteURL = strings.TrimRight(strings.TrimSpace(siteURL), "/")
	if siteURL == "" {
		siteURL = DefaultSiteURL
	}
	return Brand{Name: name, SiteURL: siteURL}
}

// Options 是邮件外壳渲染参数。
type Options struct {
	Brand    Brand
	Title    string        // 卡片正文标题
	BodyHTML template.HTML // 已转义/受信任的正文 HTML 片段
	CTAText  string        // 可选，底部按钮文案；为空时不渲染按钮
	CTAURL   string        // CTAText 非空时必须提供跳转地址
}

const shellLayout = `<!doctype html>
<html lang="zh-CN">
<body style="margin:0;padding:0;background:#F4F5F7;">
<div style="background:#F4F5F7;padding:32px 16px;">
<div style="max-width:480px;margin:0 auto;background:#FFFFFF;border-radius:8px;box-shadow:0 1px 3px rgba(0,0,0,0.08);overflow:hidden;font-family:-apple-system,'PingFang SC','Microsoft YaHei',sans-serif;">
<div style="padding:28px 32px 0;text-align:center;">
<a href="{{.Brand.SiteURL}}" style="color:#6366F1;font-size:20px;font-weight:700;text-decoration:none;letter-spacing:0.5px;">{{.Brand.Name}}</a>
</div>
<div style="padding:20px 32px 8px;">
<p style="font-size:18px;font-weight:700;color:#111827;margin:0 0 12px;">{{.Title}}</p>
{{.BodyHTML}}
{{if .CTAText}}<div style="text-align:center;margin:20px 0;">
<a href="{{.CTAURL}}" style="display:inline-block;background:#6366F1;color:#FFFFFF;font-size:14px;font-weight:700;text-decoration:none;padding:10px 28px;border-radius:20px;">{{.CTAText}}</a>
</div>{{end}}
</div>
<div style="border-top:1px solid #F0F0F0;padding:16px 32px;text-align:center;">
<p style="font-size:12px;color:#9CA3AF;margin:0;">&copy; {{.Year}} {{.Brand.Name}} · 这是一封自动发送的邮件，请勿直接回复</p>
</div>
</div>
</div>
</body>
</html>`

var shellTemplate = template.Must(template.New("email-shell").Parse(shellLayout))

// shellData 是模板执行时的完整数据，在 Options 基础上补充渲染时才能确定的年份。
type shellData struct {
	Options
	Year int
}

// Wrap 渲染统一邮件外壳。模板固定且字段均为字符串/受信任 HTML，Execute 实际不会返回错误；
// 忽略错误以保持调用方签名简单，异常情况下退化为只输出正文片段。
func Wrap(opts Options) string {
	data := shellData{Options: opts, Year: time.Now().Year()}

	var b strings.Builder
	if err := shellTemplate.Execute(&b, data); err != nil {
		return string(opts.BodyHTML)
	}
	return b.String()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./pkg/email/layout/... -v`
Expected: 全部 `PASS`。

- [ ] **Step 5: 提交**

```bash
git add pkg/email/layout/layout.go pkg/email/layout/layout_test.go
git commit -m "$(cat <<'EOF'
feat(email): 新增邮件模板共享 layout 包
EOF
)"
```

---

### Task 2: 验证码邮件按场景区分文案

**Files:**
- Modify: `pkg/email/email.go`（全文件替换）
- Create: `pkg/email/email_test.go`

**Interfaces:**
- Consumes: `layout.ResolveBrand`、`layout.Wrap`、`layout.Options`（Task 1）
- Produces:
  - `type email.Purpose string`
  - `const email.PurposeRegister/PurposePasswordReset/PurposeEmailBind`
  - `type email.MailSender interface { SendVerificationCode(to, code string, purpose Purpose) error; SendHTML(...) error }`（签名变更，破坏性）
  - `type email.Config struct { ...; BrandName string; SiteURL string }`（新增两个字段）

- [ ] **Step 1: 写失败的测试**

创建 `pkg/email/email_test.go`（同包内部测试，因为要访问未导出的 `purposeCopy`）：

```go
package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPurposeCopy_ReturnsDistinctTextPerScenario(t *testing.T) {
	cases := []struct {
		purpose      Purpose
		wantTitle    string
		wantContains string
	}{
		{PurposeRegister, "注册验证码", "你正在注册"},
		{PurposePasswordReset, "重置密码验证码", "你正在重置"},
		{PurposeEmailBind, "绑定邮箱验证码", "你正在绑定/更换"},
	}
	for _, c := range cases {
		title, desc := purposeCopy(c.purpose, "YEVPT")
		assert.Equal(t, c.wantTitle, title)
		assert.Contains(t, desc, c.wantContains)
		assert.Contains(t, desc, "YEVPT")
	}
}

func TestPurposeCopy_UnknownPurposeFallsBackToRegisterCopy(t *testing.T) {
	title, _ := purposeCopy(Purpose("unknown"), "YEVPT")
	assert.Equal(t, "注册验证码", title)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./pkg/email/... -run TestPurposeCopy`
Expected: 编译失败，`undefined: Purpose` / `undefined: purposeCopy`（`purposeCopy` 和 `Purpose` 还不存在）。

- [ ] **Step 3: 用完整新内容替换 `pkg/email/email.go`**

```go
package email

import (
	"crypto/tls"
	"fmt"
	"html/template"

	"gopkg.in/gomail.v2"

	"github.com/vpt/blog-backend/pkg/email/layout"
)

// Purpose 标识验证码邮件的具体使用场景，用于渲染不同的提示文案，降低钓鱼风险。
type Purpose string

const (
	PurposeRegister      Purpose = "register"       // 注册
	PurposePasswordReset Purpose = "password_reset" // 重置密码
	PurposeEmailBind     Purpose = "email_bind"      // 绑定/更换邮箱
)

// MailSender 邮件发送接口，便于在测试中 mock
type MailSender interface {
	// SendVerificationCode 按场景发送验证码邮件。
	SendVerificationCode(to, code string, purpose Purpose) error
	// SendHTML 发送通用 HTML 邮件；messageID 非空时写入 Message-ID 头，用于发送侧幂等与追踪。
	SendHTML(to string, subject string, htmlBody string, messageID string) error
}

// Config 邮件服务配置
type Config struct {
	Host      string
	Port      int
	From      string
	Password  string
	FromName  string // 发件人昵称，非空时格式化为 "昵称 <邮箱>"
	BrandName string // 邮件正文品牌名，留空时使用 layout.DefaultBrandName
	SiteURL   string // 站点公网访问前缀，留空时使用 layout.DefaultSiteURL
}

// Mailer 是 MailSender 的 SMTP 实现
type Mailer struct {
	cfg *Config
}

func NewMailer(cfg *Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// fromHeader 返回发件人地址，配置了昵称时格式化为 RFC 5322 的 "Name <addr>"，
// 否则仅返回裸邮箱。
func (m *Mailer) fromHeader(msg *gomail.Message) string {
	if m.cfg.FromName == "" {
		return m.cfg.From
	}
	return msg.FormatAddress(m.cfg.From, m.cfg.FromName)
}

// purposeCopy 返回验证码邮件按场景区分的标题与说明文案；未知场景兜底为注册文案。
func purposeCopy(purpose Purpose, brandName string) (title, description string) {
	switch purpose {
	case PurposePasswordReset:
		return "重置密码验证码", fmt.Sprintf("你正在重置 %s 账号密码，验证码用于确认是你本人操作：", brandName)
	case PurposeEmailBind:
		return "绑定邮箱验证码", fmt.Sprintf("你正在绑定/更换 %s 账号邮箱，验证码用于确认是你本人操作：", brandName)
	default:
		return "注册验证码", fmt.Sprintf("你正在注册 %s 账号，验证码用于确认是你本人操作：", brandName)
	}
}

// SendVerificationCode 向指定邮箱发送验证码邮件，按 purpose 区分场景文案，有效期在邮件正文中已说明（5分钟）
func (m *Mailer) SendVerificationCode(to, code string, purpose Purpose) error {
	brand := layout.ResolveBrand(m.cfg.BrandName, m.cfg.SiteURL)
	title, description := purposeCopy(purpose, brand.Name)

	body := fmt.Sprintf(
		`<p style="font-size:14px;line-height:1.6;color:#374151;margin:0 0 16px;">%s</p>`+
			`<div style="background:#EEF0FE;border-radius:6px;padding:16px;text-align:center;margin:0 0 16px;">`+
			`<span style="font-size:32px;font-weight:700;letter-spacing:8px;color:#6366F1;font-family:'Courier New',monospace;">%s</span>`+
			`</div>`+
			`<p style="font-size:13px;line-height:1.6;color:#6B7280;margin:0;">验证码 5 分钟内有效，请勿泄露给他人。如非本人操作，请忽略此邮件。</p>`,
		description, code,
	)

	htmlBody := layout.Wrap(layout.Options{
		Brand:    brand,
		Title:    title,
		BodyHTML: template.HTML(body),
	})

	msg := gomail.NewMessage()
	msg.SetHeader("From", m.fromHeader(msg))
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", fmt.Sprintf("【%s】%s", brand.Name, title))
	msg.SetBody("text/html", htmlBody)

	return m.dialAndSend(msg)
}

// SendHTML 发送通用 HTML 邮件，供通知摘要等场景使用。
func (m *Mailer) SendHTML(to string, subject string, htmlBody string, messageID string) error {
	msg := gomail.NewMessage()
	msg.SetHeader("From", m.fromHeader(msg))
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	// 写入 Message-ID 便于追踪与发送侧幂等判断。
	if messageID != "" {
		msg.SetHeader("Message-ID", messageID)
	}
	msg.SetBody("text/html", htmlBody)

	return m.dialAndSend(msg)
}

// dialAndSend 用统一的 SSL 拨号器发送邮件并自动关闭连接。
func (m *Mailer) dialAndSend(msg *gomail.Message) error {
	// 创建 SMTP 拨号器，使用配置中的主机、端口、账号密码
	d := gomail.NewDialer(m.cfg.Host, m.cfg.Port, m.cfg.From, m.cfg.Password)
	// 163/QQ/阿里云企业 SMTP 要求 SSL（端口 465），不能用 STARTTLS
	d.SSL = true
	d.TLSConfig = &tls.Config{ServerName: m.cfg.Host}
	return d.DialAndSend(msg)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go build ./pkg/email/... && go test ./pkg/email/... -run TestPurposeCopy -v`
Expected: 全部 `PASS`。注意此时不要跑全仓库的 `go build ./...`：`auth.go`/`account_security.go` 还在调用旧的两参数 `SendVerificationCode`，全仓库构建会报错，这是预期的中间态，下一任务（Task 3）会修完这些调用点。

- [ ] **Step 5: 提交**

```bash
git add pkg/email/email.go pkg/email/email_test.go
git commit -m "$(cat <<'EOF'
feat(email): 验证码邮件按场景区分文案并接入统一卡片视觉
EOF
)"
```

---

### Task 3: 验证码邮件调用点改造

**Files:**
- Modify: `internal/service/auth/auth.go:177`, `internal/service/auth/auth.go:222`
- Modify: `internal/service/user/account_security.go:66`
- Modify: `internal/service/auth/auth_test.go`
- Modify: `internal/service/user/user_test.go`

**Interfaces:**
- Consumes: `email.Purpose`、`email.PurposeRegister`、`email.PurposePasswordReset`、`email.PurposeEmailBind`（Task 2）

- [ ] **Step 1: 修改 `auth.go` 两处调用**

`auth.go:177`（`SendCode` 方法内）：

```go
	// 发送验证码邮件，SMTP 失败时错误直接返回给调用方，不做重试
	return s.mailer.SendVerificationCode(to, code, email.PurposeRegister)
```

`auth.go:222`（`SendPasswordResetCode` 方法内）：

```go
	codeKey := passwordResetCodeKey(emailAddr)
	s.rdb.Set(ctx, codeKey, code, 5*time.Minute)
	return s.mailer.SendVerificationCode(emailAddr, code, email.PurposePasswordReset)
```

- [ ] **Step 2: 修改 `account_security.go:66`**

```go
	ctx := context.Background()
	s.security.Redis.Set(ctx, emailCodeKey(userID, normalized), code, 5*time.Minute)
	return s.security.Mailer.SendVerificationCode(normalized, code, email.PurposeEmailBind)
```

- [ ] **Step 3: 更新 `auth_test.go` 的 mock 与断言**

在文件顶部 import 块新增 `"github.com/vpt/blog-backend/pkg/email"`。

把 `mockMailSender` 改为：

```go
// mockMailSender 测试用邮件发送 mock
type mockMailSender struct {
	err         error
	sentTo      string
	sentCode    string
	sentPurpose email.Purpose
}

func (m *mockMailSender) SendVerificationCode(to, code string, purpose email.Purpose) error {
	m.sentTo = to
	m.sentCode = code
	m.sentPurpose = purpose
	return m.err
}

func (m *mockMailSender) SendHTML(_ string, _ string, _ string, _ string) error {
	return m.err
}
```

在 `TestAuthService_SendCode_Success`（约 87-102 行）的断言末尾新增：

```go
	assert.Equal(t, email.PurposeRegister, mailer.sentPurpose)
```

在 `TestAuthService_SendPasswordResetCode_ExistingEmailWritesScopedCode`（约 143-159 行）的断言末尾新增：

```go
	assert.Equal(t, email.PurposePasswordReset, mailer.sentPurpose)
```

- [ ] **Step 4: 更新 `user_test.go` 的 mock 与断言**

在文件顶部 import 块新增 `"github.com/vpt/blog-backend/pkg/email"`。

把 `securityMailSender` 改为：

```go
type securityMailSender struct {
	sentTo      string
	sentCode    string
	sentPurpose email.Purpose
}

func (m *securityMailSender) SendVerificationCode(to, code string, purpose email.Purpose) error {
	m.sentTo = to
	m.sentCode = code
	m.sentPurpose = purpose
	return nil
}

func (m *securityMailSender) SendHTML(_ string, _ string, _ string, _ string) error {
	return nil
}
```

在 `TestUserService_SendEmailCode_WritesScopedCodeAndSendsMail`（约 104-119 行）的断言末尾新增：

```go
	assert.Equal(t, email.PurposeEmailBind, mailer.sentPurpose)
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go build ./... && go test ./internal/service/auth/... ./internal/service/user/... -v`
Expected: 全部 `PASS`（`go build` 此时应已通过，因为 `pkg/email` 的所有调用点都已同步）。

- [ ] **Step 6: 提交**

```bash
git add internal/service/auth/auth.go internal/service/auth/auth_test.go \
  internal/service/user/account_security.go internal/service/user/user_test.go
git commit -m "$(cat <<'EOF'
feat(auth): 验证码邮件按注册/重置密码/绑定邮箱场景区分文案
EOF
)"
```

---

### Task 4: 通知摘要邮件卡片化

> **执行时修正：** 本任务与下面的 Task 5（通知摘要发送器接线）在执行时合并为一次交付（同一个 implementer 派工、同一次提交）。原因：`renderDigestHTML` 是未导出函数，其测试必须是同包内部测试（`package notification`），而 `email_sender.go` 与本文件同包且直接调用 `renderDigestHTML`；`go test`（不同于 `go build`）会连同包内所有非测试文件一起编译整个测试二进制，所以只改本文件、不同步改 `email_sender.go` 的调用点，会导致 Task 4 自己的验证命令都无法编译通过——这是执行过程中发现的计划缺陷，不是实现者的错误。合并后按 Task 4 的产出内容 + Task 5 的产出内容一起实现、一起验证、一起提交（可以是同一个 commit，也可以是紧邻的两个 commit，只要两者之间没有对外可见的中间状态即可）。

**Files:**
- Modify: `internal/service/notification/email_template.go`（全文件替换）
- Create: `internal/service/notification/email_template_test.go`

**Interfaces:**
- Consumes: `layout.ResolveBrand`、`layout.Wrap`、`layout.Options`（Task 1）
- Produces: `func renderDigestHTML(tasks []model.NotificationEmailTask, events map[uint]model.NotificationEvent, rootLabels map[uint]string, brandName, siteURL string) string`（签名变更：新增 `brandName` 参数，且调整参数顺序为 `brandName, siteURL`）
- 内部新增（供 Task 5 之外无需感知，仅本包内部使用）：`type eventBadge struct{ Label, BgColor, FgColor string }`、`func badgeFor(eventType string) eventBadge`

- [ ] **Step 1: 写失败的测试**

创建 `internal/service/notification/email_template_test.go`（同包内部测试，因为要访问未导出的 `badgeFor`/`renderDigestHTML`）：

```go
package notification

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vpt/blog-backend/internal/model"
)

func TestBadgeFor_ReturnsDistinctLabelsForEligibleEventTypes(t *testing.T) {
	cases := []struct {
		eventType string
		wantLabel string
	}{
		{EventTypeCommentCreated, "评论"},
		{EventTypeReplyCreated, "回复"},
		{EventTypeGuestbookCreated, "留言"},
	}
	for _, c := range cases {
		badge := badgeFor(c.eventType)
		assert.Equal(t, c.wantLabel, badge.Label)
		assert.NotEmpty(t, badge.BgColor)
		assert.NotEmpty(t, badge.FgColor)
	}
}

func TestBadgeFor_FallsBackToNeutralColorForUnknownType(t *testing.T) {
	badge := badgeFor(EventTypeSystemNotice)

	assert.Equal(t, "系统通知", badge.Label)
	assert.Equal(t, "#F1EFE8", badge.BgColor)
}

func TestRenderDigestHTML_WrapsCardsWithBrandAndCTA(t *testing.T) {
	tasks := []model.NotificationEmailTask{
		{Base: model.Base{ID: 1}, EventID: 100},
	}
	events := map[uint]model.NotificationEvent{
		100: {
			Base:           model.Base{ID: 100},
			Type:           EventTypeCommentCreated,
			RootType:       "article",
			RootID:         5,
			ContentExcerpt: "写得很好",
		},
	}
	rootLabels := map[uint]string{100: "如何设计邮件模板"}

	html := renderDigestHTML(tasks, events, rootLabels, "", "https://www.yevpt.com")

	assert.Contains(t, html, "你有 1 条新的互动通知")
	assert.Contains(t, html, "评论")
	assert.Contains(t, html, "写得很好")
	assert.Contains(t, html, "查看全部通知")
	assert.Contains(t, html, `href="https://www.yevpt.com"`)
	assert.Contains(t, html, ">YEVPT<")
	assert.Contains(t, html, `href="https://www.yevpt.com/articles/5"`)
}

func TestRenderDigestHTML_FallsBackToDefaultBrandWhenEmpty(t *testing.T) {
	html := renderDigestHTML(nil, nil, nil, "", "")

	assert.Contains(t, html, ">YEVPT<")
	assert.Contains(t, html, `href="https://www.yevpt.com"`)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/notification/... -run 'TestBadgeFor|TestRenderDigestHTML'`
Expected: 编译失败，`undefined: badgeFor`（`badgeFor` 还不存在，且 `renderDigestHTML` 参数个数不匹配）。

- [ ] **Step 3: 用完整新内容替换 `internal/service/notification/email_template.go`**

```go
package notification

import (
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/pkg/email/layout"
)

// eventTypeLabels 把事件类型映射为中文展示词，用于兜底分支的标题前缀。
var eventTypeLabels = map[string]string{
	EventTypeCommentCreated:   "评论",
	EventTypeReplyCreated:     "回复",
	EventTypeArticleLiked:     "文章点赞",
	EventTypeMomentLiked:      "碎语点赞",
	EventTypeGuestbookCreated: "留言",
	EventTypeGuestbookLiked:   "留言点赞",
	EventTypeSystemNotice:     "系统通知",
	EventTypeLegacyNotice:     "通知",
}

// 根对象中文称谓：邮件正文中指代「你的文章/碎语/留言板」时使用。
const (
	rootLabelArticle   = "文章"
	rootLabelMoment    = "碎语"
	rootLabelGuestbook = "留言板"
)

// rootLabelFor 返回根对象类型的中文称谓，未知类型回退为「内容」。
func rootLabelFor(rootType string) string {
	switch rootType {
	case "article":
		return rootLabelArticle
	case "moment":
		return rootLabelMoment
	case "guestbook":
		return rootLabelGuestbook
	default:
		return "内容"
	}
}

// eventBadge 是通知卡片左上角类型徽章的展示文案与配色。
type eventBadge struct {
	Label   string
	BgColor string
	FgColor string
}

// badgeFor 返回事件类型对应的徽章样式。邮件聚合队列当前只允许评论/回复/留言三种事件类型
// 进入邮件（见 dispatcher.go 的 emailEligibleEventTypes），其余类型统一使用中性配色兜底。
func badgeFor(eventType string) eventBadge {
	switch eventType {
	case EventTypeCommentCreated:
		return eventBadge{Label: "评论", BgColor: "#EEEDFE", FgColor: "#534AB7"}
	case EventTypeReplyCreated:
		return eventBadge{Label: "回复", BgColor: "#E1F5EE", FgColor: "#0F6E56"}
	case EventTypeGuestbookCreated:
		return eventBadge{Label: "留言", BgColor: "#FAECE7", FgColor: "#993C1D"}
	default:
		return eventBadge{Label: eventTypeLabel(eventType), BgColor: "#F1EFE8", FgColor: "#5F5E5A"}
	}
}

// renderDigestHTML 渲染摘要邮件正文：按任务顺序把每条通知渲染成带类型徽章的卡片，
// 文章类根对象附上可点击跳转链接；外层壳体（品牌 Logo、CTA 按钮、版权）由 layout.Wrap 统一渲染。
//
//   - rootLabels 按事件 ID 索引的根对象展示快照（文章标题/碎语摘要），缺失时回退为「ID xx」。
//   - brandName/siteURL 分别是邮件品牌名与站点前缀，留空时由 layout.ResolveBrand 兜底默认值。
func renderDigestHTML(tasks []model.NotificationEmailTask, events map[uint]model.NotificationEvent, rootLabels map[uint]string, brandName, siteURL string) string {
	brand := layout.ResolveBrand(brandName, siteURL)
	var b strings.Builder
	for _, task := range tasks {
		event := events[task.EventID]
		eventType := eventTypeOf(task, event)
		summary := html.EscapeString(eventSummary(event))
		b.WriteString(renderEventCard(eventType, event, rootLabels, brand.SiteURL, summary))
	}

	return layout.Wrap(layout.Options{
		Brand:    brand,
		Title:    fmt.Sprintf("你有 %d 条新的互动通知", len(tasks)),
		BodyHTML: template.HTML(b.String()),
		CTAText:  "查看全部通知",
		CTAURL:   brand.SiteURL,
	})
}

// renderEventCard 渲染单条通知卡片：类型徽章 + 具体场景文案。
func renderEventCard(eventType string, event model.NotificationEvent, rootLabels map[uint]string, siteURL, summary string) string {
	badge := badgeFor(eventType)
	line := renderEventLine(eventType, event, rootLabels, siteURL, summary)
	return fmt.Sprintf(
		`<div style="border:1px solid #EEE;border-radius:6px;padding:12px 14px;margin:0 0 10px;">`+
			`<span style="display:inline-block;font-size:11px;font-weight:700;color:%s;background:%s;border-radius:4px;padding:2px 8px;margin-bottom:6px;">%s</span>`+
			`<p style="font-size:14px;color:#374151;margin:0;line-height:1.5;">%s</p>`+
			`</div>`,
		badge.FgColor, badge.BgColor, html.EscapeString(badge.Label), line,
	)
}

// renderEventLine 渲染单条通知正文，按事件类型与根对象类型组合具体场景文案。
func renderEventLine(eventType string, event model.NotificationEvent, rootLabels map[uint]string, siteURL, summary string) string {
	rootLabel := html.EscapeString(truncateForDisplay(rootLabels[event.ID]))
	switch eventType {
	case EventTypeCommentCreated:
		return renderCommentCreatedLine(event, rootLabel, siteURL, summary)
	case EventTypeReplyCreated:
		return renderReplyCreatedLine(event, rootLabel, siteURL, summary)
	case EventTypeGuestbookCreated:
		return "你的留言板收到了新留言：" + summary
	default:
		// 兜底：未知事件类型沿用旧版「类型：摘要」结构。
		return fmt.Sprintf("<strong>%s</strong>：%s", html.EscapeString(eventTypeLabel(eventType)), summary)
	}
}

// renderCommentCreatedLine 渲染「被评论」通知行：
// 文章根附带跳转链接，碎语/留言根以中文称谓标识；根快照缺失时回退为 ID。
func renderCommentCreatedLine(event model.NotificationEvent, rootLabel, siteURL, summary string) string {
	URL := articleURL(event.RootID, siteURL)
	switch event.RootType {
	case "article":
		if rootLabel == "" {
			rootLabel = "ID " + strconv.FormatUint(uint64(event.RootID), 10)
		}
		if URL != "" {
			return fmt.Sprintf(`你的文章「<a href="%s">%s</a>」收到了新评论：%s`, template.HTMLEscapeString(URL), rootLabel, summary)
		}
		return fmt.Sprintf(`你的文章「%s」收到了新评论：%s`, rootLabel, summary)
	case "moment":
		if rootLabel == "" {
			rootLabel = "ID " + strconv.FormatUint(uint64(event.RootID), 10)
		}
		return fmt.Sprintf(`你的碎语「%s」收到了新评论：%s`, rootLabel, summary)
	case "guestbook":
		return "你的留言板收到了新留言：" + summary
	default:
		return "你的" + rootLabelFor(event.RootType) + "收到了新评论：" + summary
	}
}

// renderReplyCreatedLine 渲染「被回复」通知行：始终以「回复」为语义主体，并用根快照补充上下文。
func renderReplyCreatedLine(event model.NotificationEvent, rootLabel, siteURL, summary string) string {
	URL := articleURL(event.RootID, siteURL)
	switch event.RootType {
	case "article":
		if rootLabel == "" {
			rootLabel = "ID " + strconv.FormatUint(uint64(event.RootID), 10)
		}
		if URL != "" {
			return fmt.Sprintf(`你在文章「<a href="%s">%s</a>」下的评论收到了新回复：%s`, template.HTMLEscapeString(URL), rootLabel, summary)
		}
		return fmt.Sprintf(`你在文章「%s」下的评论收到了新回复：%s`, rootLabel, summary)
	case "moment":
		if rootLabel == "" {
			rootLabel = "ID " + strconv.FormatUint(uint64(event.RootID), 10)
		}
		return fmt.Sprintf(`你在碎语「%s」下的评论收到了新回复：%s`, rootLabel, summary)
	case "guestbook":
		return "你的留言收到了新回复：" + summary
	default:
		return "你的评论收到了新回复：" + summary
	}
}

// articleURL 在 rootID 非零时拼出 /articles/{id} 的可跳转链接（siteURL 由 layout.ResolveBrand 兜底，恒非空）。
func articleURL(rootID uint, siteURL string) string {
	if rootID == 0 {
		return ""
	}
	return siteURL + "/articles/" + strconv.FormatUint(uint64(rootID), 10)
}

// truncateForDisplay 折叠展示快照中的换行与首尾空白，避免邮件正文中出现不可控排版。
// 长度截断由 Directory 在快照写入时完成，此处仅做空规整。
func truncateForDisplay(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

// eventTypeOf 取事件类型，事件快照缺失时回退到任务上的类型快照。
func eventTypeOf(task model.NotificationEmailTask, event model.NotificationEvent) string {
	if event.Type != "" {
		return event.Type
	}
	return task.EventType
}

// eventTypeLabel 返回事件类型的中文展示词，未知类型回退为「通知」。
func eventTypeLabel(eventType string) string {
	if label, ok := eventTypeLabels[eventType]; ok {
		return label
	}
	return "通知"
}

// eventSummary 取事件的展示文案：优先内容摘要，其次标题，均空时给出占位语。
func eventSummary(event model.NotificationEvent) string {
	if event.ContentExcerpt != "" {
		return event.ContentExcerpt
	}
	if event.Title != "" {
		return event.Title
	}
	return "你有一条新通知"
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/notification/... -run 'TestBadgeFor|TestRenderDigestHTML' -v`
Expected: 全部 `PASS`（此时 `go build ./...` 仍会因 `email_sender.go` 调用点未同步而失败，属预期中间态，下一任务修复）。

- [ ] **Step 5: 提交**

```bash
git add internal/service/notification/email_template.go internal/service/notification/email_template_test.go
git commit -m "$(cat <<'EOF'
feat(notification): 通知摘要邮件改为带类型徽章的卡片视觉
EOF
)"
```

---

### Task 5: 通知摘要发送器接线

**Files:**
- Modify: `internal/service/notification/email_sender.go:36-65`（结构体、构造函数）、`internal/service/notification/email_sender.go:120`（调用点）
- Modify: `internal/service/notification/email_sender_test.go`

**Interfaces:**
- Consumes: `renderDigestHTML(tasks, events, rootLabels, brandName, siteURL string) string`（Task 4）
- Produces: `func NewEmailSender(repo senderRepo, quota *QuotaService, roles RoleResolver, roots RootSnapshotResolver, mailer email.MailSender, provider, brandName, siteURL string) *EmailSender`（签名变更：新增 `brandName` 参数，位于 `provider` 和 `siteURL` 之间）

- [ ] **Step 1: 修改 `EmailSender` 结构体与构造函数**

`email_sender.go:36-65` 替换为：

```go
// EmailSender 邮件发送器：领取到点批次，校验额度后渲染发送，并落发送日志与状态。
type EmailSender struct {
	repo         senderRepo
	quota        *QuotaService
	roles        RoleResolver
	roots        RootSnapshotResolver
	mailer       email.MailSender
	provider     string
	brandName    string
	siteURL      string
	leaseSeconds int
	retryDelay   time.Duration
	now          func() time.Time
}

// NewEmailSender 创建邮件发送器。
// roots 用于按事件根对象解析展示快照，可为 nil（此时退化为不展示根对象标题）。
// brandName/siteURL 留空时分别使用 layout 包的默认品牌名/站点地址。
func NewEmailSender(repo senderRepo, quota *QuotaService, roles RoleResolver, roots RootSnapshotResolver, mailer email.MailSender, provider, brandName, siteURL string) *EmailSender {
	return &EmailSender{
		repo:         repo,
		quota:        quota,
		roles:        roles,
		roots:        roots,
		mailer:       mailer,
		provider:     provider,
		brandName:    brandName,
		siteURL:      siteURL,
		leaseSeconds: defaultSenderLeaseSecs,
		retryDelay:   defaultSendRetryDelay,
		now:          time.Now,
	}
}
```

- [ ] **Step 2: 修改渲染调用点**

`email_sender.go:120`：

```go
	htmlBody := renderDigestHTML(tasks, events, rootLabels, s.brandName, s.siteURL)
```

- [ ] **Step 3: 更新 `email_sender_test.go`**

在文件顶部 import 块新增 `"github.com/vpt/blog-backend/pkg/email"`。

把 `fakeMailer.SendVerificationCode` 签名改为：

```go
func (m *fakeMailer) SendVerificationCode(string, string, email.Purpose) error { return nil }
```

把 `newSender` 辅助函数（约 82 行）改为：

```go
func newSender(repo *senderRepoStub, quotaStore *fakeQuotaStore, mailer *fakeMailer) *notificationservice.EmailSender {
	quota := notificationservice.NewQuotaService(quotaStore, cfg())
	return notificationservice.NewEmailSender(repo, quota, fakeRoles{roles: []string{"normal"}}, nil, mailer, "test", "", "")
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go build ./... && go test ./internal/service/notification/... -v`
Expected: 全部 `PASS`（`go build ./...` 此时应通过，`notification` 包内所有调用点均已同步；`bootstrap.go` 的调用点会在 Task 9 统一修复，本任务先只保证 `notification` 包自身可编译可测试——若 `go build ./...` 因 `bootstrap.go` 报错属预期，可改跑 `go build ./internal/service/notification/...` 确认本包通过）。

- [ ] **Step 5: 提交**

```bash
git add internal/service/notification/email_sender.go internal/service/notification/email_sender_test.go
git commit -m "$(cat <<'EOF'
feat(notification): 邮件发送器接入品牌名参数
EOF
)"
```

---

### Task 6: 审核提醒邮件卡片化

> **执行时修正：** 与 Task 4 相同的原因，本任务与下面的 Task 7（审核提醒发送器接线）在执行时合并为一次交付。`Render` 虽是导出函数，但 `sender.go` 与本文件同包并直接调用 `Render`；改了 `Render` 签名而不同步改 `sender.go` 的调用点，会导致本任务自己的 `go test ./internal/service/moderationemail/...` 验证命令编译失败。合并后一起实现、一起验证、一起提交。

**Files:**
- Modify: `internal/service/moderationemail/template.go`（全文件替换）
- Modify: `internal/service/moderationemail/template_test.go`（全文件替换）

**Interfaces:**
- Consumes: `layout.ResolveBrand`、`layout.Wrap`、`layout.Options`（Task 1）
- Produces: `func Render(batch model.ModerationReviewEmailBatch, tasks []moderationemailrepo.PendingTask, siteURL, brandName, adminURL string) (RenderedEmail, error)`（签名变更：新增 `brandName`、`adminURL` 两个参数）

- [ ] **Step 1: 用完整新内容替换 `internal/service/moderationemail/template.go`**

```go
package moderationemail

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	moderationemailrepo "github.com/vpt/blog-backend/internal/repository/moderationemail"
	"github.com/vpt/blog-backend/pkg/email/layout"
)

const (
	maxRenderedRows = 50
	maxExcerptRunes = 120
	// defaultAdminURL 是 admin_url 留空时的兜底审核后台地址，直接作为最终跳转地址使用。
	defaultAdminURL = "https://admin.yevpt.com"
)

// reviewCardsLayout 是审核提醒邮件的正文片段（不含外层品牌壳体）：
// 每条待审内容渲染成一张卡片，超出展示上限时在最前面提示溢出条数。
const reviewCardsLayout = `{{if .OverflowCount}}<p style="font-size:13px;color:#6B7280;margin:0 0 16px;">以下展示前 {{len .Rows}} 条，其余 {{.OverflowCount}} 条请前往后台查看。</p>{{end}}
{{range .Rows}}<div style="border:1px solid #EEE;border-radius:6px;padding:12px 14px;margin:0 0 10px;">
<span style="display:inline-block;font-size:11px;font-weight:700;color:#534AB7;background:#EEEDFE;border-radius:4px;padding:2px 8px;margin-bottom:6px;">{{.TypeLabel}}</span>
<p style="font-size:14px;color:#374151;margin:0 0 4px;line-height:1.5;">{{.Excerpt}}</p>
<p style="font-size:12px;color:#9CA3AF;margin:0;">#{{.ItemID}} · 作者 #{{.AuthorID}} · {{.CreatedAt}}</p>
</div>
{{end}}`

var reviewCardsTemplate = template.Must(template.New("review-cards").Parse(reviewCardsLayout))

// RenderedEmail 是审核摘要邮件渲染结果。
type RenderedEmail struct {
	Subject string
	HTML    string
}

type reviewEmailData struct {
	Rows          []reviewEmailRow
	OverflowCount int
}

type reviewEmailRow struct {
	TypeLabel string
	ItemID    uint64
	AuthorID  uint64
	Excerpt   string
	CreatedAt string
}

// Render 安全渲染审核摘要邮件；用户提交正文只作为模板数据输出，由 html/template 转义。
// brandName/siteURL 用于顶部品牌 Logo，adminURL 留空时使用 defaultAdminURL，非空时直接作为
// 「打开审核后台」按钮的最终跳转地址，不拼接路径。
func Render(batch model.ModerationReviewEmailBatch, tasks []moderationemailrepo.PendingTask, siteURL, brandName, adminURL string) (RenderedEmail, error) {
	// 以批次总数为权威来源，缺失时回退到任务数量。
	total := batch.ItemCount
	if total <= 0 {
		total = len(tasks)
	}

	// 只展示前 50 条，剩余数量通过溢出提示表达。
	displayed := minInt(len(tasks), maxRenderedRows)
	data := reviewEmailData{
		Rows:          renderRows(tasks[:displayed]),
		OverflowCount: maxInt(total-displayed, 0),
	}

	// 执行模板渲染，避免任何手写 HTML 拼接用户正文。
	var body bytes.Buffer
	if err := reviewCardsTemplate.Execute(&body, data); err != nil {
		return RenderedEmail{}, err
	}

	brand := layout.ResolveBrand(brandName, siteURL)
	htmlBody := layout.Wrap(layout.Options{
		Brand:    brand,
		Title:    fmt.Sprintf("共 %d 条待审核内容", total),
		BodyHTML: template.HTML(body.String()),
		CTAText:  "打开审核后台",
		CTAURL:   resolveAdminURL(adminURL),
	})

	return RenderedEmail{
		Subject: fmt.Sprintf("待审核内容提醒（%d 条）", total),
		HTML:    htmlBody,
	}, nil
}

// resolveAdminURL 对审核后台地址应用兜底默认值。
func resolveAdminURL(adminURL string) string {
	adminURL = strings.TrimSpace(adminURL)
	if adminURL == "" {
		return defaultAdminURL
	}
	return adminURL
}

func renderRows(tasks []moderationemailrepo.PendingTask) []reviewEmailRow {
	rows := make([]reviewEmailRow, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, reviewEmailRow{
			TypeLabel: typeLabel(task.ContentType),
			ItemID:    task.ItemID,
			AuthorID:  task.AuthorID,
			Excerpt:   excerpt(task.SubmittedContent, maxExcerptRunes),
			CreatedAt: formatTime(task.CreatedAt),
		})
	}
	return rows
}

func typeLabel(contentType string) string {
	switch contentType {
	case model.ModerationContentMoment:
		return "碎语"
	case model.ModerationContentArticleComment:
		return "文章评论"
	case model.ModerationContentMomentComment:
		return "碎语评论"
	case model.ModerationContentGuestbook:
		return "留言"
	case model.ModerationContentArticleReply:
		return "文章回复"
	case model.ModerationContentMomentReply:
		return "碎语回复"
	case model.ModerationContentGuestbookReply:
		return "留言回复"
	default:
		return contentType
	}
}

func excerpt(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02 15:04")
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
```

- [ ] **Step 2: 用完整新内容替换 `internal/service/moderationemail/template_test.go`**

```go
package moderationemail_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
	moderationemailrepo "github.com/vpt/blog-backend/internal/repository/moderationemail"
	moderationemailservice "github.com/vpt/blog-backend/internal/service/moderationemail"
)

func TestRenderEscapesSubmittedHTMLAndKeepsUnicodeExcerptValid(t *testing.T) {
	batch := reviewBatch(7, 2)
	long := strings.Repeat("中🙂", 80) + `<script>alert("x")</script>`
	tasks := []moderationemailrepo.PendingTask{
		reviewTask(1, model.ModerationContentMoment, `<img src=x onerror=alert(1)>`),
		reviewTask(2, model.ModerationContentArticleComment, long),
	}

	rendered, err := moderationemailservice.Render(batch, tasks, " https://blog.example.com/base/ ", "", "https://admin.example.com/moderation")

	require.NoError(t, err)
	assert.Equal(t, "待审核内容提醒（2 条）", rendered.Subject)
	assert.Contains(t, rendered.HTML, "共 2 条待审核内容")
	assert.Contains(t, rendered.HTML, "碎语")
	assert.Contains(t, rendered.HTML, "文章评论")
	assert.Contains(t, rendered.HTML, "作者 #301")
	assert.Contains(t, rendered.HTML, `href="https://admin.example.com/moderation"`)
	assert.NotContains(t, rendered.HTML, `<img src=x`)
	assert.NotContains(t, rendered.HTML, "<script>")
	assert.Contains(t, rendered.HTML, "&lt;img src=x")
	assert.True(t, utf8.ValidString(rendered.HTML))
}

func TestRenderLimitsRowsAndShowsOverflowAgainstTotalItemCount(t *testing.T) {
	batch := reviewBatch(8, 53)
	tasks := make([]moderationemailrepo.PendingTask, 0, 53)
	for i := 0; i < 53; i++ {
		tasks = append(tasks, reviewTask(uint64(i+1), model.ModerationContentGuestbookReply, "留言回复"))
	}

	rendered, err := moderationemailservice.Render(batch, tasks, "", "", "")

	require.NoError(t, err)
	assert.Equal(t, "待审核内容提醒（53 条）", rendered.Subject)
	assert.Contains(t, rendered.HTML, "共 53 条待审核内容")
	assert.Contains(t, rendered.HTML, "其余 3 条请前往后台查看")
	assert.Equal(t, 50, strings.Count(rendered.HTML, "background:#EEEDFE"))
}

func TestRenderFallsBackToDefaultBrandAndAdminURLWhenEmpty(t *testing.T) {
	batch := reviewBatch(9, 1)
	tasks := []moderationemailrepo.PendingTask{reviewTask(1, model.ModerationContentMoment, "内容")}

	rendered, err := moderationemailservice.Render(batch, tasks, "", "", "")

	require.NoError(t, err)
	assert.Contains(t, rendered.HTML, ">YEVPT<")
	assert.Contains(t, rendered.HTML, `href="https://admin.yevpt.com"`)
}

func reviewBatch(id uint64, itemCount int) model.ModerationReviewEmailBatch {
	return model.ModerationReviewEmailBatch{
		ID:        id,
		ToEmail:   "owner@example.com",
		ItemCount: itemCount,
		Subject:   "legacy subject should be replaced",
	}
}

func reviewTask(id uint64, contentType string, content string) moderationemailrepo.PendingTask {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	return moderationemailrepo.PendingTask{
		ID:               id,
		RevisionID:       id + 100,
		ItemID:           id + 200,
		ContentType:      contentType,
		AuthorID:         id + 300,
		SubmittedContent: content,
		AvailableAt:      now,
		CreatedAt:        now,
	}
}
```

- [ ] **Step 3: 运行测试确认通过**

Run: `go test ./internal/service/moderationemail/... -run TestRender -v`
Expected: 全部 `PASS`。

- [ ] **Step 4: 提交**

```bash
git add internal/service/moderationemail/template.go internal/service/moderationemail/template_test.go
git commit -m "$(cat <<'EOF'
feat(moderationemail): 审核提醒邮件改为卡片视觉并支持独立后台地址
EOF
)"
```

---

### Task 7: 审核提醒发送器接线

**Files:**
- Modify: `internal/service/moderationemail/sender.go:33-54`（结构体、构造函数）、`internal/service/moderationemail/sender.go:103`（调用点）
- Modify: `internal/service/moderationemail/sender_test.go`

**Interfaces:**
- Consumes: `Render(batch, tasks, siteURL, brandName, adminURL string) (RenderedEmail, error)`（Task 6）
- Produces: `func NewSender(repo SenderRepository, mailer MailSender, siteURL, brandName, adminURL string, now func() time.Time) *Sender`（签名变更：新增 `brandName`、`adminURL` 两个参数，位于 `siteURL` 之后、`now` 之前）

- [ ] **Step 1: 修改 `Sender` 结构体与构造函数**

`sender.go:33-54` 替换为：

```go
// Sender 领取待发送审核摘要批次并执行可重试发送。
type Sender struct {
	repo          SenderRepository
	mailer        MailSender
	siteURL       string
	brandName     string
	adminURL      string
	leaseDuration time.Duration
	now           func() time.Time
}

// NewSender 创建审核摘要邮件发送器。
func NewSender(repo SenderRepository, mailer MailSender, siteURL, brandName, adminURL string, now func() time.Time) *Sender {
	if now == nil {
		now = time.Now
	}
	return &Sender{
		repo:          repo,
		mailer:        mailer,
		siteURL:       siteURL,
		brandName:     brandName,
		adminURL:      adminURL,
		leaseDuration: defaultSenderLeaseDuration,
		now:           now,
	}
}
```

- [ ] **Step 2: 修改渲染调用点**

`sender.go:103`：

```go
	rendered, err := Render(renderBatch, tasks, s.siteURL, s.brandName, s.adminURL)
```

- [ ] **Step 3: 更新 `sender_test.go` 的 4 处构造调用**

把以下 4 处（约 30、57、83、106 行）：

```go
sender := moderationemailservice.NewSender(repo, mailer, "https://blog.example.com", func() time.Time { return now })
```
```go
sender := moderationemailservice.NewSender(repo, mailer, "", func() time.Time { return now })
```

分别改为：

```go
sender := moderationemailservice.NewSender(repo, mailer, "https://blog.example.com", "", "", func() time.Time { return now })
```
```go
sender := moderationemailservice.NewSender(repo, mailer, "", "", "", func() time.Time { return now })
```

（`reviewMailerStub` 上遗留的 `SendVerificationCode(string, string) error` 方法不需要改动：`moderationemail.MailSender` 接口只声明了 `SendHTML`，这个方法未被任何接口要求，是历史遗留的死代码，本次不改动它，保持改动最小。）

- [ ] **Step 4: 运行测试确认通过**

Run: `go build ./internal/service/moderationemail/... && go test ./internal/service/moderationemail/... -v`
Expected: 全部 `PASS`。

- [ ] **Step 5: 提交**

```bash
git add internal/service/moderationemail/sender.go internal/service/moderationemail/sender_test.go
git commit -m "$(cat <<'EOF'
feat(moderationemail): 发送器接入品牌名与审核后台地址参数
EOF
)"
```

---

### Task 8: 配置新增字段

**Files:**
- Modify: `pkg/config/config.go:95-113`（`EmailConfig` 结构体）
- Modify: `pkg/config/moderation.go:49-56`（`ModerationReviewEmailConfig` 结构体）
- Modify: `config/config.yaml:54-59`
- Modify: `config/config.local.yaml:36-42`

**Interfaces:**
- Produces:
  - `config.EmailConfig.BrandName string`（`mapstructure:"brand_name"`）
  - `config.ModerationReviewEmailConfig.AdminURL string`（`mapstructure:"admin_url"`）

- [ ] **Step 1: 修改 `pkg/config/config.go` 的 `EmailConfig`**

`config.go:95-101` 替换为：

```go
type EmailConfig struct {
	Host      string `mapstructure:"host"`       // SMTP 主机地址
	Port      int    `mapstructure:"port"`       // SMTP 端口
	From      string `mapstructure:"from"`       // 发件人邮箱
	Password  string `mapstructure:"password"`   // 邮箱授权码或密码
	FromName  string `mapstructure:"from_name"`  // 发件人昵称，如 YEVPT，为空时仅显示邮箱地址
	BrandName string `mapstructure:"brand_name"` // 邮件正文品牌名，留空时默认 "YEVPT"
	SiteURL   string `mapstructure:"site_url"`   // 站点公网访问前缀，用于邮件正文中的跳转链接，留空时默认 "https://www.yevpt.com"
```

（第 102 行起的 `Provider` 等字段保持不动。）

- [ ] **Step 2: 修改 `pkg/config/moderation.go` 的 `ModerationReviewEmailConfig`**

`moderation.go:49-56` 替换为：

```go
// ModerationReviewEmailConfig 定义待审核邮件的接收人和调度秒数。
type ModerationReviewEmailConfig struct {
	Enabled                  bool   `mapstructure:"enabled"`
	RecipientUserID          uint   `mapstructure:"recipient_user_id"`
	AggregationWindowSeconds int    `mapstructure:"aggregation_window_seconds"`
	MinIntervalSeconds       int    `mapstructure:"min_interval_seconds"`
	PollIntervalSeconds      int    `mapstructure:"poll_interval_seconds"`
	AdminURL                 string `mapstructure:"admin_url"` // 审核后台地址，留空时默认 "https://admin.yevpt.com"，直接作为最终跳转地址使用
}
```

- [ ] **Step 3: 更新 `config/config.yaml`**

`config.yaml:54-59` 替换为：

```yaml
email:
  # 发件人昵称，出现在邮件 From 头，如 "YEVPT <noreply@example.com>"；为空时仅显示邮箱
  from_name: ""
  # 邮件正文品牌名（顶部 Logo 文案、版权行），留空时默认 "YEVPT"
  brand_name: ""
  # 站点公网访问前缀，用于通知邮件中的跳转链接，留空时默认 "https://www.yevpt.com"
  site_url: ""
```

`review_email` 块（`config.yaml:85-90`）新增一行：

```yaml
  review_email:
    enabled: true
    recipient_user_id: 1
    aggregation_window_seconds: 60
    min_interval_seconds: 1800
    poll_interval_seconds: 15
    # 审核后台地址，留空时默认 "https://admin.yevpt.com"
    admin_url: ""
```

- [ ] **Step 4: 更新 `config/config.local.yaml`**

`config.local.yaml:36-42` 替换为：

```yaml
email:
  host: smtp.qiye.aliyun.com
  port: 465
  from: vpt@yevpt.com
  password: "j#!Dr&Q&23hRU3"
  from_name: "YEVPT"
  brand_name: "YEVPT"
  site_url: "https://www.yevpt.com"
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go build ./pkg/config/... && go test ./pkg/config/... -v`
Expected: 全部 `PASS`（新增字段不改变现有校验逻辑，`go vet` 无警告）。

- [ ] **Step 6: 提交**

```bash
git add pkg/config/config.go pkg/config/moderation.go config/config.yaml config/config.local.yaml
git commit -m "$(cat <<'EOF'
feat(config): 新增邮件品牌名与审核后台地址配置项
EOF
)"
```

---

### Task 9: bootstrap 装配

**Files:**
- Modify: `internal/bootstrap/bootstrap.go:76-85`（`InitMailer`）
- Modify: `internal/bootstrap/bootstrap.go:116`（`StartNotificationWorker` 内 `NewEmailSender` 调用）
- Modify: `internal/bootstrap/bootstrap.go:161`（`StartModerationReviewEmailWorker` 内 `NewSender` 调用）

**Interfaces:**
- Consumes:
  - `email.Config{ ...; BrandName string; SiteURL string }`（Task 2、8）
  - `notificationservice.NewEmailSender(repo, quota, roles, roots, mailer, provider, brandName, siteURL string)`（Task 5）
  - `moderationemailservice.NewSender(repo, mailer, siteURL, brandName, adminURL string, now func() time.Time)`（Task 7）

- [ ] **Step 1: 修改 `InitMailer`**

`bootstrap.go:76-85` 替换为：

```go
// InitMailer 创建邮件发送器，用于发送注册和登录验证码。
func InitMailer(cfg *config.Config) email.MailSender {
	return email.NewMailer(&email.Config{
		Host:      cfg.Email.Host,
		Port:      cfg.Email.Port,
		From:      cfg.Email.From,
		Password:  cfg.Email.Password,
		FromName:  cfg.Email.FromName,
		BrandName: cfg.Email.BrandName,
		SiteURL:   cfg.Email.SiteURL,
	})
}
```

- [ ] **Step 2: 修改 `StartNotificationWorker` 内的 `NewEmailSender` 调用**

`bootstrap.go:116`：

```go
	sender := notificationservice.NewEmailSender(repo, quota, directory, directory, mailer, cfg.Email.Provider, cfg.Email.BrandName, cfg.Email.SiteURL)
```

- [ ] **Step 3: 修改 `StartModerationReviewEmailWorker` 内的 `NewSender` 调用**

`bootstrap.go:161`：

```go
	sender := moderationemailservice.NewSender(repo, mailer, cfg.Email.SiteURL, cfg.Email.BrandName, reviewEmail.AdminURL, time.Now)
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go build ./... && go vet ./...`
Expected: 全量编译与 vet 通过，无遗留签名不匹配。

- [ ] **Step 5: 提交**

```bash
git add internal/bootstrap/bootstrap.go
git commit -m "$(cat <<'EOF'
feat(bootstrap): 装配邮件品牌名与审核后台地址配置
EOF
)"
```

---

### Task 10: 全量验证

**Files:**
- 无代码改动，仅验证。

- [ ] **Step 1: 全量构建**

Run: `go build ./...`
Expected: 无报错。

- [ ] **Step 2: 全量静态检查**

Run: `go vet ./...`
Expected: 无报错。

- [ ] **Step 3: 全量测试**

Run: `go test ./...`
Expected: 全部 `PASS`（包括本次未直接改动、但依赖 `email.MailSender`/`notificationservice`/`moderationemailservice` 的其余包，如 `internal/worker/...`）。

- [ ] **Step 4: 格式检查**

Run: `gofmt -l pkg/email internal/service/notification internal/service/moderationemail internal/service/auth internal/service/user internal/bootstrap pkg/config`
Expected: 无输出（无未格式化文件）。

- [ ] **Step 5: 若发现问题则现场修复并重新运行 Step 1-4，全部通过后无需提交（本任务不产生代码改动）**
