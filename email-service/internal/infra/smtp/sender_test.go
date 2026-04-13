package smtp

import "testing"

func TestSendRejectsInvalidPort(t *testing.T) {
	sender := NewSender(Config{Host: "smtp.example.com", Port: "invalid", FromName: "Homenavi", FromEmail: "noreply@example.com"})
	if err := sender.Send("user@example.com", "Subject", []byte("<html></html>")); err == nil {
		t.Fatal("expected invalid port error")
	}
}
