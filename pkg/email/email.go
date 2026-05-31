package email

import (
	"crypto/tls"
	"fmt"

	"gopkg.in/gomail.v2"
)

// MailSender 邮件发送接口，便于在测试中 mock
type MailSender interface {
	SendVerificationCode(to, code string) error
}

// Config 邮件服务配置
type Config struct {
	Host     string
	Port     int
	From     string
	Password string
}

// Mailer 是 MailSender 的 SMTP 实现
type Mailer struct {
	cfg *Config
}

func NewMailer(cfg *Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// SendVerificationCode 向指定邮箱发送验证码邮件，有效期在邮件正文中已说明（5分钟）
func (m *Mailer) SendVerificationCode(to, code string) error {
	// 组装邮件头：发件人、收件人、主题
	msg := gomail.NewMessage()
	msg.SetHeader("From", m.cfg.From)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", "【博客】邮箱验证码")
	// 设置 HTML 正文，验证码大字号显示方便用户阅读
	msg.SetBody("text/html", fmt.Sprintf(
		`<p>您的验证码为：<strong style="font-size:24px">%s</strong></p><p>验证码 5 分钟内有效，请勿泄露给他人。</p>`,
		code,
	))

	// 创建 SMTP 拨号器，使用配置中的主机、端口、账号密码
	d := gomail.NewDialer(m.cfg.Host, m.cfg.Port, m.cfg.From, m.cfg.Password)
	// 163/QQ SMTP 要求 SSL（端口 465），不能用 STARTTLS
	d.SSL = true
	d.TLSConfig = &tls.Config{ServerName: m.cfg.Host}
	// 建立连接并发送邮件，发送完毕后自动关闭连接
	return d.DialAndSend(msg)
}
