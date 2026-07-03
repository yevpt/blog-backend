# 邮件通知模板重新设计

## 1. 背景与目标

当前三处邮件发送场景的 HTML 正文都是最朴素的手写标签，没有统一视觉风格：

- 验证码邮件（[pkg/email/email.go:46](../../../pkg/email/email.go)）：一段 `<p>` + `<strong>` 大字号验证码，无卡片、无品牌标识。
- 通知摘要邮件（[internal/service/notification/email_template.go](../../../internal/service/notification/email_template.go)）：`<div><h2>...</h2><ul>...</ul></div>`，逐条纯文本列出评论/回复/点赞/留言互动。
- 审核提醒邮件（[internal/service/moderationemail/template.go](../../../internal/service/moderationemail/template.go)）：一张无样式的裸 `<table>`。

三处邮件也没有统一的品牌名/站点地址来源：通知摘要和审核提醒依赖 `email.site_url` 配置项拼接跳转链接，为空时直接不渲染链接；验证码邮件完全没有品牌信息。

目标：

1. 三类邮件统一采用极简卡片风视觉：浅灰底、白色圆角卡片、蓝紫色（`#6366F1`）强调色、顶部可跳转的文字品牌 Logo、底部版权行。
2. 验证码邮件按注册/重置密码/绑定邮箱三种场景区分提示文案。
3. 品牌名、站点地址、审核后台地址均可通过配置项覆盖，留空时使用合理默认值。
4. 样式通过共享包复用，避免三处模板各自维护一份 CSS 造成视觉漂移。

本次不引入除现有 3 类场景外的新邮件类型，不改变发送节流/聚合/重试等既有机制。

## 2. 整体架构：共享 Layout 包

新增 `pkg/email/layout` 包，纯展示层工具，不含业务逻辑，供 `pkg/email`、`internal/service/notification`、`internal/service/moderationemail` 共同引用。

```go
package layout

const (
    DefaultBrandName = "YEVPT"
    DefaultSiteURL   = "https://www.yevpt.com"
)

// Brand 是邮件顶部品牌标识与跳转目标。
type Brand struct {
    Name    string
    SiteURL string
}

// ResolveBrand 对品牌名和站点地址应用兜底默认值，供三处渲染入口统一调用。
func ResolveBrand(name, siteURL string) Brand

// Options 是邮件外壳渲染参数。
type Options struct {
    Brand    Brand
    Title    string // 卡片正文标题
    BodyHTML string // 已受信任/已转义的正文 HTML 片段
    CTAText  string // 可选，底部按钮文案
    CTAURL   string // 可选，底部按钮跳转地址；CTAText 非空时才渲染按钮
}

// Wrap 渲染统一邮件外壳：灰底居中卡片、顶部品牌 Logo、正文、可选 CTA 按钮、底部版权行。
func Wrap(opts Options) string
```

实现方式：包内用 `html/template` 定义一个固定布局模板（行内 `style`，不使用 `<style>` 块，兼容国内主流邮箱客户端），`BodyHTML` 以 `template.HTML` 类型注入（调用方必须保证内容已转义或来自受信任的固定字符串）。

视觉规范：

- 页面背景 `#F4F5F7`，卡片 `#FFFFFF`，圆角 8px，`box-shadow: 0 1px 3px rgba(0,0,0,0.08)`，最大宽度 480px 居中。
- 顶部品牌区：`<a href="{SiteURL}">{Name}</a>`，颜色 `#6366F1`，字号 20px，加粗。
- 字体栈：`-apple-system,'PingFang SC','Microsoft YaHei',sans-serif`。
- CTA 按钮：`#6366F1` 底、白字、圆角 20px 胶囊按钮。
- 底部版权行：灰色 12px，"© 2026 {Name} · 这是一封自动发送的邮件，请勿直接回复"。

三处调用方各自只负责生成 `BodyHTML`（卡片正文片段）与可选 CTA，壳体外观由 `layout.Wrap` 统一保证。

## 3. 验证码邮件

现状确认（避免误解业务场景）：`SendCode` 用于**注册**（[auth.go:122](../../../internal/service/auth/auth.go)），`SendPasswordResetCode` 用于**重置密码**（同文件 180 行），`SendEmailCode` 用于**绑定/更换邮箱**（[account_security.go:38](../../../internal/service/user/account_security.go)）。三处目前调用同一个 `SendVerificationCode(to, code string)`，文案完全相同。

接口改动：

```go
// pkg/email/email.go
type Purpose string

const (
    PurposeRegister      Purpose = "register"
    PurposePasswordReset Purpose = "password_reset"
    PurposeEmailBind     Purpose = "email_bind"
)

type MailSender interface {
    SendVerificationCode(to, code string, purpose Purpose) error
    SendHTML(to string, subject string, htmlBody string, messageID string) error
}
```

按 `purpose` 决定标题与说明文案（品牌名来自 `layout.ResolveBrand` 后的 `Brand.Name`）：

| purpose | 标题 | 说明行 |
| --- | --- | --- |
| `register` | 注册验证码 | 你正在注册 {Brand} 账号，验证码用于确认是你本人操作： |
| `password_reset` | 重置密码验证码 | 你正在重置 {Brand} 账号密码，验证码用于确认是你本人操作： |
| `email_bind` | 绑定邮箱验证码 | 你正在绑定/更换 {Brand} 账号邮箱，验证码用于确认是你本人操作： |

验证码本体样式不变（沿用 mockup）：浅蓝紫底框，32px 加粗等宽字体，8px 字间距；下方补一行"验证码 5 分钟内有效，请勿泄露给他人。如非本人操作，请忽略此邮件。"

`email.Config` 新增 `BrandName`、`SiteURL` 两个字段（供 `Mailer` 渲染顶部品牌区），由 `bootstrap.InitMailer` 从 `cfg.Email.BrandName`/`cfg.Email.SiteURL` 透传。

影响面（签名改动的连锁调用点）：

- `pkg/email/email.go`：`Mailer.SendVerificationCode` 实现改造。
- `internal/service/auth/auth.go`：`SendCode`（177 行）传 `email.PurposeRegister`，`SendPasswordResetCode`（222 行）传 `email.PurposePasswordReset`。
- `internal/service/user/account_security.go`：`SendEmailCode`（66 行）传 `email.PurposeEmailBind`。
- 测试 stub 同步改签名：`auth_test.go`、`user_test.go`、`notification/email_sender_test.go`、`moderationemail/sender_test.go` 中的 4 处手写 `SendVerificationCode` 方法。

## 4. 通知摘要邮件

`internal/service/notification/email_template.go` 的 `renderDigestHTML` 改为：每条通知渲染成一张小卡片（浅灰边框、圆角 6px），卡片左上角是类型徽章（浅色底 + 深色字），下方是互动摘要正文；文章类根对象摘要仍保留可点击跳转链接。所有卡片拼接后作为 `BodyHTML`，连同标题（"你有 N 条新的互动通知"）和 CTA（"查看全部通知" → `Brand.SiteURL`）一起传给 `layout.Wrap`。

邮件聚合队列当前只允许 `comment_created`、`reply_created`、`guestbook_created` 三种事件类型进入邮件（[dispatcher.go:21](../../../internal/service/notification/dispatcher.go)），因此徽章只需覆盖这三类，其余类型沿用原有兜底文案（不单独设计徽章配色）：

| 事件类型 | 徽章文案 | 徽章配色 |
| --- | --- | --- |
| `comment_created` | 评论 | 底 `#EEEDFE` / 字 `#534AB7`（靛紫） |
| `reply_created` | 回复 | 底 `#E1F5EE` / 字 `#0F6E56`（青绿） |
| `guestbook_created` | 留言 | 底 `#FAECE7` / 字 `#993C1D`（珊瑚） |

原有的 `renderEventLine`/`renderCommentCreatedLine`/`renderReplyCreatedLine` 等场景文案拼接逻辑保留，只是外层包装从裸 `<li>` 换成带徽章的卡片 `<div>`；`renderFooter` 函数删除，footer 由 `layout.Wrap` 统一渲染。

`NewEmailSender` 构造函数新增 `brandName string` 参数（紧邻现有 `siteURL string`），`bootstrap.StartNotificationWorker` 透传 `cfg.Email.BrandName`。

## 5. 审核提醒邮件

`internal/service/moderationemail/template.go` 的 `Render` 同样改为卡片列表而非裸 `<table>`：每条待审内容一张卡片，徽章统一用中性靛紫色（不像通知摘要那样按类型区分多种颜色，审核提醒是管理员内部工具，无需强区分），卡片内容为"类型徽章 + 内容摘要 + 元信息行（`#{ItemID} · 作者 #{AuthorID} · {CreatedAt}`）"。选用卡片列表而非表格，是因为表格在部分移动端邮件客户端容易横向溢出，卡片列表天然纵向堆叠、兼容性更好，也与通知摘要邮件视觉语言保持一致。

CTA 按钮"打开审核后台"跳转到新配置项 `admin_url`，**直接作为最终地址使用，不拼接路径**。原 `adminURL(siteURL string) string`（由 `siteURL + "/admin/moderation"` 拼接）函数删除。

配置：`ModerationReviewEmailConfig` 新增 `AdminURL string`（`mapstructure:"admin_url"`），默认值 `https://admin.yevpt.com`。默认值回退不放进通用的 `layout` 包（那里只管品牌名/站点地址这两个跨场景字段），而是在 `moderationemail` 包内新增一个包私有常量 `defaultAdminURL = "https://admin.yevpt.com"` 和函数 `resolveAdminURL(adminURL string) string`（空值兜底），与 `Render` 放在同一文件。

`moderationemailservice.NewSender` 构造函数新增 `brandName string` 参数（用于品牌 Logo），`adminURL` 替代原来通过 `siteURL` 派生的用法；`bootstrap.StartModerationReviewEmailWorker` 透传 `cfg.Email.BrandName`、`cfg.Moderation.ReviewEmail.AdminURL`。

## 6. 配置改动汇总

- `pkg/config/config.go` 的 `EmailConfig` 新增：
  ```go
  BrandName string `mapstructure:"brand_name"` // 邮件品牌名，留空默认 "YEVPT"
  ```
  （`SiteURL` 字段已存在，语义从"可选，留空则邮件里的链接不渲染"改为"留空则用默认站点地址 https://www.yevpt.com，Logo 与链接始终可跳转"。）
- `pkg/config/moderation.go` 的 `ModerationReviewEmailConfig` 新增：
  ```go
  AdminURL string `mapstructure:"admin_url"` // 审核后台地址，留空默认 "https://admin.yevpt.com"，直接作为最终跳转地址使用
  ```
- `config/config.yaml`、`config.prod.yaml`、`config.local.yaml`、`config.test.yaml` 补充上述两个新字段（留空即用默认值，无需强制每份配置文件都显式填写）。
- `internal/bootstrap/bootstrap.go`：
  - `InitMailer` 新增传入 `BrandName`、`SiteURL` 到 `email.Config`。
  - `StartNotificationWorker` 里 `NewEmailSender(...)` 新增 `cfg.Email.BrandName` 参数。
  - `StartModerationReviewEmailWorker` 里 `NewSender(...)` 新增 `cfg.Email.BrandName` 参数，并把 `cfg.Moderation.ReviewEmail.AdminURL` 传入渲染路径。

## 7. 测试设计

- `pkg/email`：新增/调整测试覆盖三种 `Purpose` 分别渲染出不同标题与说明文案；`layout.ResolveBrand` 的空值兜底分支。
- `pkg/email/layout`：`Wrap` 在 `CTAText` 为空时不渲染按钮；`Title`/`BodyHTML` 转义边界（确保 `BodyHTML` 按 `template.HTML` 直接输出，不重复转义）。
- `internal/service/notification`：`email_template_test.go`（如无则新建）覆盖三种事件类型的徽章文案与配色类名是否符合预期；`email_sender_test.go` 中的 `fakeMailer.SendVerificationCode` 签名同步更新。
- `internal/service/moderationemail`：`template_test.go` 覆盖卡片列表渲染与空 `AdminURL` 时的默认地址回退；`sender_test.go` 中的 `reviewMailerStub.SendVerificationCode` 签名同步更新。
- `internal/service/auth`、`internal/service/user`：确认三处调用点传入了正确的 `Purpose` 常量。

沿用项目现有分层测试约定：repository 层不涉及本次改动；service 层用 gomock/手写 fake；渲染类纯函数直接单元测试断言字符串包含关键片段（如徽章文案、按钮文案、Logo 链接），不做全量 HTML 快照比对。

## 8. 验收条件与风险

完成条件：

- 三类邮件视觉统一为卡片风格，`go test ./...` 通过。
- 验证码邮件按场景显示不同标题/说明文案。
- `brand_name`、`site_url`、`moderation.review_email.admin_url` 均可通过配置覆盖，留空时分别兜底为 `YEVPT`、`https://www.yevpt.com`、`https://admin.yevpt.com`。
- `make swag` 无需改动（三处邮件渲染均不经过 HTTP handler，不涉及 Swagger）。

风险：

- `SendVerificationCode` 签名改动是破坏性变更，需要在同一次改动中完成 3 个调用点 + 4 个测试 stub 的同步修改，遗漏会导致编译失败（而非运行时静默错误），风险可控。
- `SiteURL` 语义变化（空值不再表示"禁用链接"而是"用默认域名"）如果生产环境曾依赖"留空以关闭跳转链接"这一副作用，会变成始终跳转到默认域名；需要在发布说明中提醒，但当前代码库未发现此依赖。
