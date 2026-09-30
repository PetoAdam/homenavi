package twofactor

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

func TestHandle2FAEmailRequestBlocksDemoUserInDemoMode(t *testing.T) {
	userService := newTwoFactorTestUserService(t, twoFactorTestUser{
		id:       "00000000-0000-0000-0000-000000000001",
		userName: "demo_abc123",
		email:    "demo+abc123@example.com",
	})
	t.Cleanup(userService.Close)

	authService := newTwoFactorTestAuthService(t)
	h := NewEmailHandler(authService, userService.Client(), &clientsinfra.EmailClient{}, true)
	body, _ := json.Marshal(map[string]string{"user_id": "00000000-0000-0000-0000-000000000001"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/email/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Handle2FAEmailRequest(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if len(userService.updateCalls) != 0 {
		t.Fatalf("expected no user updates, got %d", len(userService.updateCalls))
	}
}

func TestHandle2FASetupBlocksDemoUserInDemoMode(t *testing.T) {
	userService := newTwoFactorTestUserService(t, twoFactorTestUser{
		id:       "00000000-0000-0000-0000-000000000001",
		userName: "demo_abc123",
		email:    "demo+abc123@example.com",
	})
	t.Cleanup(userService.Close)

	authService := newTwoFactorTestAuthService(t)
	h := NewSetupHandler(authService, userService.Client(), true)
	body, _ := json.Marshal(map[string]string{"user_id": "00000000-0000-0000-0000-000000000001"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Handle2FASetup(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if len(userService.updateCalls) != 0 {
		t.Fatalf("expected no user updates, got %d", len(userService.updateCalls))
	}
}

type twoFactorTestUser struct {
	id               string
	userName         string
	email            string
	twoFactorEnabled bool
	twoFactorSecret  string
	twoFactorType    string
}

type twoFactorTestUserService struct {
	server      *httptest.Server
	users       map[string]twoFactorTestUser
	updateCalls []string
	privateKey  *rsa.PrivateKey
}

func newTwoFactorTestUserService(t *testing.T, user twoFactorTestUser) *twoFactorTestUserService {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	svc := &twoFactorTestUserService{users: map[string]twoFactorTestUser{user.id: user}, privateKey: key}
	svc.server = httptest.NewServer(http.HandlerFunc(svc.handle))
	return svc
}

func (s *twoFactorTestUserService) Close() {
	s.server.Close()
}

func (s *twoFactorTestUserService) Client() *clientsinfra.UserClient {
	return clientsinfra.NewUserClient(clientsinfra.UserConfig{BaseURL: s.server.URL, JWTPrivateKey: s.privateKey})
}

func (s *twoFactorTestUserService) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && len(r.URL.Path) > len("/users/") && r.URL.Path[:len("/users/")] == "/users/" {
		id := r.URL.Path[len("/users/"):]
		user, ok := s.users[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(clientsinfra.User{
			ID:               user.id,
			UserName:         user.userName,
			Email:            user.email,
			TwoFactorEnabled: user.twoFactorEnabled,
			TwoFactorSecret:  user.twoFactorSecret,
			TwoFactorType:    user.twoFactorType,
		})
		return
	}
	if r.Method == http.MethodPatch && len(r.URL.Path) > len("/users/") && r.URL.Path[:len("/users/")] == "/users/" {
		s.updateCalls = append(s.updateCalls, r.URL.Path[len("/users/"):])
		w.WriteHeader(http.StatusOK)
		return
	}
	http.NotFound(w, r)
}

func newTwoFactorTestAuthService(t *testing.T) *authdomain.Service {
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
	}, noopTwoFactorCacheStore{})
}

type noopTwoFactorCacheStore struct{}

func (noopTwoFactorCacheStore) Get(_ context.Context, _ string) (string, error)       { return "", nil }
func (noopTwoFactorCacheStore) GetDelete(_ context.Context, _ string) (string, error) { return "", nil }
func (noopTwoFactorCacheStore) Set(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}
func (noopTwoFactorCacheStore) Delete(_ context.Context, _ ...string) error               { return nil }
func (noopTwoFactorCacheStore) Increment(_ context.Context, _ string) (int64, error)      { return 0, nil }
func (noopTwoFactorCacheStore) Expire(_ context.Context, _ string, _ time.Duration) error { return nil }
func (noopTwoFactorCacheStore) TTL(_ context.Context, _ string) (time.Duration, error)    { return 0, nil }
func (noopTwoFactorCacheStore) Close() error                                              { return nil }

var _ cacheinfra.Store = noopTwoFactorCacheStore{}
