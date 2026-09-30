package password

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

func TestHandleChangePasswordBlocksDemoUserInDemoMode(t *testing.T) {
	userService := newPasswordTestUserService(t, passwordTestUser{
		id:       "00000000-0000-0000-0000-000000000001",
		userName: "demo_abc123",
		email:    "demo+abc123@example.com",
	})
	t.Cleanup(userService.Close)

	authService := newPasswordTestAuthService(t)
	h := NewChangeHandler(authService, userService.Client(), true)
	body, _ := json.Marshal(map[string]string{
		"current_password": "current-password",
		"new_password":     "DemoValid9!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password/change", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+passwordTestAccessToken(t, authService, "00000000-0000-0000-0000-000000000001"))
	rr := httptest.NewRecorder()

	h.HandleChangePassword(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["error"] != "This action is disabled in the public demo" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
	if len(userService.updateCalls) != 0 {
		t.Fatalf("expected no password update, got %d calls", len(userService.updateCalls))
	}
	if len(userService.validateCalls) != 0 {
		t.Fatalf("expected no credential validation, got %d calls", len(userService.validateCalls))
	}
}

func TestHandleChangePasswordAllowsNonDemoUserWhenDemoModeEnabled(t *testing.T) {
	userService := newPasswordTestUserService(t, passwordTestUser{
		id:       "00000000-0000-0000-0000-000000000002",
		userName: "resident",
		email:    "resident@example.com",
	})
	t.Cleanup(userService.Close)

	authService := newPasswordTestAuthService(t)
	h := NewChangeHandler(authService, userService.Client(), true)
	body, _ := json.Marshal(map[string]string{
		"current_password": "current-password",
		"new_password":     "DemoValid9!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password/change", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+passwordTestAccessToken(t, authService, "00000000-0000-0000-0000-000000000002"))
	rr := httptest.NewRecorder()

	h.HandleChangePassword(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(userService.validateCalls) != 1 {
		t.Fatalf("expected credential validation, got %d", len(userService.validateCalls))
	}
	if len(userService.updateCalls) != 1 || userService.updateCalls[0] != "00000000-0000-0000-0000-000000000002" {
		t.Fatalf("expected password update for resident-user, got %#v", userService.updateCalls)
	}
}

type passwordTestUser struct {
	id       string
	userName string
	email    string
	first    string
	last     string
	role     string
	updated  map[string]any
}

type passwordTestUserService struct {
	server        *httptest.Server
	users         map[string]passwordTestUser
	validateCalls []string
	updateCalls   []string
	privateKey    *rsa.PrivateKey
}

func newPasswordTestUserService(t *testing.T, user passwordTestUser) *passwordTestUserService {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	svc := &passwordTestUserService{
		users:      map[string]passwordTestUser{user.id: user},
		privateKey: key,
	}
	svc.server = httptest.NewServer(http.HandlerFunc(svc.handle))
	return svc
}

func (s *passwordTestUserService) Close() {
	s.server.Close()
}

func (s *passwordTestUserService) Client() *clientsinfra.UserClient {
	return clientsinfra.NewUserClient(clientsinfra.UserConfig{BaseURL: s.server.URL, JWTPrivateKey: s.privateKey})
}

func (s *passwordTestUserService) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/users/00000000-0000-0000-0000-000000000001":
		fallthrough
	case r.Method == http.MethodGet && r.URL.Path == "/users/00000000-0000-0000-0000-000000000002":
		user := s.users[r.URL.Path[len("/users/"):]]
		_ = json.NewEncoder(w).Encode(clientsinfra.User{ID: user.id, UserName: user.userName, Email: user.email, FirstName: user.first, LastName: user.last, Role: "resident"})
	case r.Method == http.MethodPost && r.URL.Path == "/users/validate":
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		s.validateCalls = append(s.validateCalls, payload["email"])
		_ = json.NewEncoder(w).Encode(clientsinfra.User{ID: "validated-user", Email: payload["email"]})
	case r.Method == http.MethodPatch && (r.URL.Path == "/users/00000000-0000-0000-0000-000000000001" || r.URL.Path == "/users/00000000-0000-0000-0000-000000000002"):
		s.updateCalls = append(s.updateCalls, r.URL.Path[len("/users/"):])
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func newPasswordTestAuthService(t *testing.T) *authdomain.Service {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate auth rsa key: %v", err)
	}
	return authdomain.NewService(authdomain.Config{
		JWTPrivateKey:        key,
		AccessTokenTTL:       time.Hour,
		RefreshTokenTTL:      time.Hour,
		TwoFactorTTL:         time.Minute,
		PasswordResetTTL:     time.Minute,
		EmailVerificationTTL: time.Minute,
	}, noopPasswordCacheStore{})
}

func passwordTestAccessToken(t *testing.T, authService *authdomain.Service, userID string) string {
	t.Helper()
	token, err := authService.IssueAccessToken(&clientsinfra.User{ID: userID, Role: "resident", FirstName: "Demo", LastName: "User"})
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}
	return token
}

type noopPasswordCacheStore struct{}

func (noopPasswordCacheStore) Get(_ context.Context, _ string) (string, error)       { return "", nil }
func (noopPasswordCacheStore) GetDelete(_ context.Context, _ string) (string, error) { return "", nil }
func (noopPasswordCacheStore) Set(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}
func (noopPasswordCacheStore) Delete(_ context.Context, _ ...string) error               { return nil }
func (noopPasswordCacheStore) Increment(_ context.Context, _ string) (int64, error)      { return 0, nil }
func (noopPasswordCacheStore) Expire(_ context.Context, _ string, _ time.Duration) error { return nil }
func (noopPasswordCacheStore) TTL(_ context.Context, _ string) (time.Duration, error)    { return 0, nil }
func (noopPasswordCacheStore) Close() error                                              { return nil }

var _ cacheinfra.Store = noopPasswordCacheStore{}
