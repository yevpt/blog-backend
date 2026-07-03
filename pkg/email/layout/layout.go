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
