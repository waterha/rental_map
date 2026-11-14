package atlas

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// CodeMailer deliberately has no development mode that reveals codes to an API or log.
type CodeMailer interface {
	SendCode(context.Context, string, string) error
}

type QQMailer struct {
	Sender   string
	Password string
}

func (m *QQMailer) SendCode(ctx context.Context, recipient, code string) error {
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: "smtp.qq.com", MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", "smtp.qq.com:465")
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(12 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, "smtp.qq.com")
	if err != nil {
		return err
	}
	defer client.Close()
	if err = client.Auth(smtp.PlainAuth("", m.Sender, m.Password, "smtp.qq.com")); err != nil {
		return err
	}
	if err = client.Mail(m.Sender); err != nil {
		return err
	}
	if err = client.Rcpt(recipient); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	// Both addresses have already passed strict ASCII validation; no header input is interpolated otherwise.
	message := fmt.Sprintf("From: Rental Atlas <%s>\r\nTo: <%s>\r\nSubject: Rental Atlas - Email verification code\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\nDate: %s\r\n\r\n租房版图注册验证码：%s\r\n\r\n验证码发送成功后 2 分钟内有效，仅可使用一次。请勿向他人透露。\r\n如果这不是你的操作，请忽略本邮件。\r\n", m.Sender, recipient, time.Now().Format(time.RFC1123Z), code)
	if _, err = writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	// DATA acknowledgement is the acceptance boundary; a failed QUIT must not invalidate accepted mail.
	_ = client.Quit()
	return nil
}

func validQQSender(sender string) bool {
	parts := strings.Split(sender, "@")
	if len(parts) != 2 || parts[1] != "qq.com" || len(parts[0]) < 5 || len(parts[0]) > 12 {
		return false
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return parts[0][0] != '0'
}
