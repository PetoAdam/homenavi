package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
)

type failingRevocationStore struct{}

type unavailableReadStore struct{ failingRevocationStore }

func (unavailableReadStore) Get(context.Context, string) (string, error) {
	return "", errors.New("redis unavailable")
}

func (failingRevocationStore) Set(context.Context, string, string, time.Duration) error { return nil }
func (failingRevocationStore) Get(context.Context, string) (string, error) {
	return `{"user_id":"user-1","family_id":"family-1"}`, nil
}
func (failingRevocationStore) Delete(context.Context, ...string) error {
	return errors.New("redis unavailable")
}
func (failingRevocationStore) TTL(context.Context, string) (time.Duration, error) {
	return time.Hour, nil
}
func (failingRevocationStore) Increment(context.Context, string) (int64, error)    { return 0, nil }
func (failingRevocationStore) Expire(context.Context, string, time.Duration) error { return nil }
func (failingRevocationStore) GetDelete(context.Context, string) (string, error) {
	return "", cacheinfra.ErrNotFound
}
func (failingRevocationStore) RotateRefreshToken(context.Context, string, string, string, string) (string, cacheinfra.RefreshTokenRotationStatus, error) {
	return "", cacheinfra.RefreshTokenMissing, nil
}
func (failingRevocationStore) Close() error { return nil }

func TestLogoutReturnsServiceUnavailableWhenRevocationFails(t *testing.T) {
	service := authdomain.NewService(authdomain.Config{}, failingRevocationStore{})
	handler := NewLogoutHandler(service)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: "refresh-token"})
	response := httptest.NewRecorder()

	handler.HandleLogout(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestLogoutReturnsServiceUnavailableWhenRefreshTokenLookupFails(t *testing.T) {
	service := authdomain.NewService(authdomain.Config{}, unavailableReadStore{})
	handler := NewLogoutHandler(service)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: "refresh-token"})
	response := httptest.NewRecorder()

	handler.HandleLogout(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
