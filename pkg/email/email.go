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
