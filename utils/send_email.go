package utils

import (
	"fmt"
	"net/smtp"
	"os"
)

func SendEmail(to, subject, body string) error {
	from := os.Getenv("SMTP_EMAIL")
	pass := os.Getenv("SMTP_PASSWORD")

	// Headers: hỗ trợ UTF-8 & HTML
	msg := ""
	msg += "MIME-Version: 1.0\r\n"
	msg += "Content-Type: text/html; charset=\"UTF-8\"\r\n"
	msg += fmt.Sprintf("From: %s\r\n", from)
	msg += fmt.Sprintf("To: %s\r\n", to)
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += "\r\n" + body

	// Gửi mail
	err := smtp.SendMail(
		"smtp.gmail.com:587",
		smtp.PlainAuth("", from, pass, "smtp.gmail.com"),
		from,
		[]string{to},
		[]byte(msg),
	)

	if err != nil {
		return fmt.Errorf("gửi email thất bại: %v", err)
	}
	return nil
}
