package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
)

type fakeEmailService struct {
	verificationErr error
	notifyErr       error
	verification    []string
	notify          []string
}

func (f *fakeEmailService) SendVerificationEmail(to, userName, code string) error {
	f.verification = []string{to, userName, code}
	return f.verificationErr
}
func (f *fakeEmailService) SendPasswordResetEmail(to, userName, code string) error { return nil }
func (f *fakeEmailService) Send2FAEmail(to, userName, code string) error           { return nil }
func (f *fakeEmailService) SendNotifyEmail(to, userName, subject, message string) error {
	f.notify = []string{to, userName, subject, message}
	return f.notifyErr
}

func TestSendVerificationEmailBadRequest(t *testing.T) {
	h := NewHandler(&fakeEmailService{})
	req := httptest.NewRequest(http.MethodPost, "/send/verification", strings.NewReader(`{"to":""}`))
	rr := httptest.NewRecorder()

	h.SendVerificationEmail(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSendVerificationEmailSuccess(t *testing.T) {
	svc := &fakeEmailService{}
	h := NewHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/send/verification", strings.NewReader(`{"to":"a@example.com","user_name":"Alice","code":"123456"}`))
	rr := httptest.NewRecorder()

	h.SendVerificationEmail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if svc.verification[0] != "a@example.com" {
		t.Fatalf("expected service to be called")
	}
}

func TestSendNotifyEmailInternalError(t *testing.T) {
	h := NewHandler(&fakeEmailService{notifyErr: errors.New("boom")})
	req := httptest.NewRequest(http.MethodPost, "/send/notify", strings.NewReader(`{"to":"a@example.com","user_name":"Alice","subject":"Hi","message":"Body"}`))
	rr := httptest.NewRecorder()

	h.SendNotifyEmail(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestRouterHealth(t *testing.T) {
	router := NewRouter(NewHandler(&fakeEmailService{}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), otel.Tracer("test"))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "OK" {
		t.Fatalf("expected OK, got %q", rr.Body.String())
	}
}

func TestWriteJSONError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSONError(rr, http.StatusBadRequest, "bad")
	var payload map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["error"] != "bad" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
