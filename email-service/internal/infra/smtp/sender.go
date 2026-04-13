package smtp

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strconv"

	"github.com/jordan-wright/email"
)

// Config holds SMTP delivery settings.
type Config struct {
	Host      string
	Port      string
	Username  string
	Password  string
	FromName  string
	FromEmail string
}

// Sender sends rendered HTML emails via SMTP with STARTTLS.
type Sender struct {
	config Config
}

func NewSender(config Config) *Sender {
	return &Sender{config: config}
}

func (s *Sender) Send(to, subject string, html []byte) error {
	e := email.NewEmail()
	e.From = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromEmail)
	e.To = []string{to}
	e.Subject = subject
	e.HTML = html

	port, err := strconv.Atoi(s.config.Port)
	if err != nil || port <= 0 {
		return fmt.Errorf("invalid SMTP_PORT value %q", s.config.Port)
	}
	auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	return e.SendWithStartTLS(
		fmt.Sprintf("%s:%d", s.config.Host, port),
		auth,
		&tls.Config{ServerName: s.config.Host, InsecureSkipVerify: false},
	)
}
