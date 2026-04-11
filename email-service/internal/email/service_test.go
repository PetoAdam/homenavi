package email

import (
	"errors"
	"strings"
	"testing"
)

type fakeSender struct {
	to      string
	subject string
	html    []byte
	err     error
	calls   int
}

func (f *fakeSender) Send(to, subject string, html []byte) error {
	f.calls++
	f.to = to
	f.subject = subject
	f.html = html
	return f.err
}

type fakeRenderer struct {
	templateName string
	data         EmailData
	html         []byte
	err          error
	calls        int
}

func (f *fakeRenderer) Render(templateName string, data EmailData) ([]byte, error) {
	f.calls++
	f.templateName = templateName
	f.data = data
	return f.html, f.err
}

func TestSendVerificationEmailRendersAndSends(t *testing.T) {
	sender := &fakeSender{}
	renderer := &fakeRenderer{html: []byte("<html>ok</html>")}
	svc := NewService(Config{AppName: "Homenavi"}, sender, renderer)

	if err := svc.SendVerificationEmail("test@example.com", "Alice", "123456"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if renderer.templateName != "verification" {
		t.Fatalf("expected verification template, got %q", renderer.templateName)
	}
	if renderer.data.UserName != "Alice" || renderer.data.Code != "123456" {
		t.Fatalf("unexpected render data: %#v", renderer.data)
	}
	if sender.to != "test@example.com" || sender.subject != "Verify Your Email Address" {
		t.Fatalf("unexpected send args: to=%q subject=%q", sender.to, sender.subject)
	}
}

func TestSendEmailPropagatesRendererError(t *testing.T) {
	svc := NewService(Config{AppName: "Homenavi"}, &fakeSender{}, &fakeRenderer{err: errors.New("boom")})
	if err := svc.SendNotifyEmail("to@example.com", "Bob", "Subject", "Body"); err == nil || !strings.Contains(err.Error(), "render email") {
		t.Fatalf("expected render email error, got %v", err)
	}
}

func TestTemplateRendererNotifyTemplate(t *testing.T) {
	renderer := NewTemplateRenderer()
	html, err := renderer.Render("notify", EmailData{AppName: "Homenavi", UserName: "Bob", Subject: "Notice", Message: "Hello"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	content := string(html)
	if !strings.Contains(content, "Hello Bob") || !strings.Contains(content, "Notice") {
		t.Fatalf("unexpected html: %s", content)
	}
}
