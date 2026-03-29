package email

import (
	"context"
	"fmt"
	"net/smtp"
)

// Sender sends emails. Implementations can be swapped (SMTP, SendGrid, mock, etc.).
type Sender interface {
	SendVerificationCode(ctx context.Context, to string, code string) error
}

type smtpSender struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func NewSMTPSender(host, port, username, password, from string) Sender {
	return &smtpSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}
}

func (s *smtpSender) SendVerificationCode(_ context.Context, to string, code string) error {
	subject := "Your verification code"
	body := fmt.Sprintf("Your email verification code is: %s\n\nThis code expires in 15 minutes.", code)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"utf-8\"\r\n\r\n%s",
		s.from, to, subject, body)

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	addr := fmt.Sprintf("%s:%s", s.host, s.port)

	if err := smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("email.SendVerificationCode: %w", err)
	}
	return nil
}
