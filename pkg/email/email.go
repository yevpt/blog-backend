package email

import (
	"crypto/tls"
	"fmt"

	"gopkg.in/gomail.v2"
)

// MailSender 邮件发送接口，便于在测试中 mock
type MailSender interface {
	// SendVerificationCode 发送验证码邮件。
	SendVerificationCode(to, code string) error
	// SendHTML 发送通用 HTML 邮件；messageID 非空时写入 Message-ID 头，用于发送侧幂等与追踪。
	SendHTML(to string, subject string, htmlBody string, messageID string) error
}

// Config 邮件服务配置
type Config struct {
	Host     string
	Port     int
	From     string
	Password string
	FromName string // 发件人昵称，非空时格式化为 "昵称 <邮箱>"
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

// SendVerificationCode 向指定邮箱发送验证码邮件，有效期在邮件正文中已说明（5分钟）
func (m *Mailer) SendVerificationCode(to, code string) error {
	// 组装邮件头：发件人、收件人、主题
	msg := gomail.NewMessage()
	msg.SetHeader("From", m.fromHeader(msg))
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", "【博客】邮箱验证码")
	// 设置 HTML 正文，验证码大字号显示方便用户阅读
	msg.SetBody("text/html", fmt.Sprintf(
		`<p>您的验证码为：<strong style="font-size:24px">%s</strong></p><p>验证码 5 分钟内有效，请勿泄露给他人。</p>`,
		code,
	))

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
