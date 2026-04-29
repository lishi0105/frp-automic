package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"frps-status-app.local/status/src/model"
)

func Send(settings model.PublicSettings, authCode, subject, body string) error {
	addr := net.JoinHostPort(settings.SMTPHost, strconv.Itoa(settings.SMTPPort))
	auth := smtp.PlainAuth("", settings.SMTPFrom, authCode, settings.SMTPHost)
	msg := "From: " + settings.SMTPFrom + "\r\n" +
		"To: " + settings.SMTPTo + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	if settings.SMTPPort == 465 {
		return sendTLS(addr, settings.SMTPHost, auth, settings.SMTPFrom, splitCSV(settings.SMTPTo), msg)
	}
	return smtp.SendMail(addr, auth, settings.SMTPFrom, splitCSV(settings.SMTPTo), []byte(msg))
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
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	return w.Close()
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
