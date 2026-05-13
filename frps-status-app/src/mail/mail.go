package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"frps-status-app.local/status/src/logger"
	"frps-status-app.local/status/src/model"
)

func Send(settings model.PublicSettings, authCode, subject, body string) error {
	return SendTo(settings, authCode, settings.SMTPTo, subject, body)
}

func SendTo(settings model.PublicSettings, authCode, to, subject, body string) error {
	addr := net.JoinHostPort(settings.SMTPHost, strconv.Itoa(settings.SMTPPort))
	auth := smtp.PlainAuth("", settings.SMTPFrom, authCode, settings.SMTPHost)
	sentAt := time.Now()
	body = withSentTime(body, sentAt)
	msg := "From: " + formatFrom(settings) + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Date: " + sentAt.Format(time.RFC1123Z) + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	toList := splitCSV(to)
	logger.Info("准备发送邮件 主机=%s 端口=%d 发件人=%s 收件人数=%d 主题=%s", settings.SMTPHost, settings.SMTPPort, settings.SMTPFrom, len(toList), subject)
	if settings.SMTPPort == 465 {
		return sendTLS(addr, settings.SMTPHost, auth, settings.SMTPFrom, toList, msg)
	}
	if err := smtp.SendMail(addr, auth, settings.SMTPFrom, toList, []byte(msg)); err != nil {
		logger.Error("SMTP 发送失败 地址=%s 发件人=%s 收件人=%v 错误=%v", addr, settings.SMTPFrom, toList, err)
		return err
	}
	logger.Info("SMTP 发送完成 地址=%s 发件人=%s 收件人=%v", addr, settings.SMTPFrom, toList)
	return nil
}

func withSentTime(body string, sentAt time.Time) string {
	line := "发送时间：" + sentAt.Format("2006-01-02 15:04:05 -0700 MST")
	body = strings.TrimLeft(body, "\r\n")
	if body == "" {
		return line
	}
	return line + "\n\n" + body
}

func formatFrom(settings model.PublicSettings) string {
	name := strings.TrimSpace(settings.SMTPUser)
	if name == "" {
		return settings.SMTPFrom
	}
	return (&mail.Address{Name: name, Address: settings.SMTPFrom}).String()
}

func HumanBytes(v uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f := float64(v)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", f, units[i])
}

func GBToBytes(gb float64) uint64 {
	return uint64(gb * 1024 * 1024 * 1024)
}

func sendTLS(addr, host string, auth smtp.Auth, from string, to []string, msg string) error {
	logger.Info("正在连接 SMTP 服务器 %s，准备 TLS 发送邮件 发件人=%s 收件人=%v", addr, from, to)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		logger.Error("SMTP TLS 连接失败 地址=%s 错误=%v", addr, err)
		return err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		logger.Error("SMTP 客户端创建失败 主机=%s 错误=%v", host, err)
		return err
	}
	defer client.Close()
	if err := client.Auth(auth); err != nil {
		logger.Error("SMTP 认证失败 主机=%s 错误=%v", host, err)
		return err
	}
	if err := client.Mail(from); err != nil {
		logger.Error("SMTP MAIL FROM 命令失败 发件人=%s 错误=%v", from, err)
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			logger.Error("SMTP RCPT TO 命令失败 收件人=%s 错误=%v", rcpt, err)
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		logger.Error("SMTP DATA 命令失败 错误=%v", err)
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		logger.Error("SMTP 写入邮件内容失败 错误=%v", err)
		return err
	}
	if err := w.Close(); err != nil {
		logger.Error("SMTP 关闭数据写入器失败 错误=%v", err)
		return err
	}
	logger.Info("SMTP TLS 发送完成 地址=%s 发件人=%s 收件人=%v", addr, from, to)
	return nil
}

func splitCSV(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
