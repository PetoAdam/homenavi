package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/golang-jwt/jwt/v5"
)

type stubCacheStore struct {
	values map[string]string
	ttls   map[string]time.Duration
}

func newStubCacheStore() *stubCacheStore {
	return &stubCacheStore{values: map[string]string{}, ttls: map[string]time.Duration{}}
}

func (s *stubCacheStore) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	s.values[key] = value
	s.ttls[key] = ttl
	return nil
}

func (s *stubCacheStore) Get(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", cacheinfra.ErrNotFound
	}
	return value, nil
}

func (s *stubCacheStore) Delete(_ context.Context, keys ...string) error {
	for _, key := range keys {
		delete(s.values, key)
		delete(s.ttls, key)
	}
	return nil
}

func (s *stubCacheStore) TTL(_ context.Context, key string) (time.Duration, error) {
	ttl, ok := s.ttls[key]
	if !ok {
		return 0, cacheinfra.ErrNotFound
	}
	return ttl, nil
}

func (s *stubCacheStore) Increment(_ context.Context, key string) (int64, error) {
	return 0, nil
}

func (s *stubCacheStore) Expire(_ context.Context, key string, ttl time.Duration) error {
	s.ttls[key] = ttl
	return nil
}

func (s *stubCacheStore) GetDelete(_ context.Context, key string) (string, error) {
	value, err := s.Get(context.Background(), key)
	if err != nil {
		return "", err
	}
	_ = s.Delete(context.Background(), key)
	return value, nil
}

func (s *stubCacheStore) Close() error {
	return nil
}

type stubUserProvider struct {
	user *clientsinfra.User
}

func (s *stubUserProvider) GetUser(userID string) (*clientsinfra.User, error) {
	if s.user != nil && s.user.ID == userID {
		return s.user, nil
	}
	return nil, nil
}

func TestIssueAccessTokenIncludesDemoClaimForDemoUser(t *testing.T) {
	service := newTestAuthService(t)
	demoUser := &clientsinfra.User{ID: "demo-user", Role: "resident", UserName: "demo_abc123", Email: "demo+abc123@example.com", FirstName: "Demo", LastName: "Visitor"}

	token, err := service.IssueAccessToken(demoUser)
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	parsed, err := jwt.Parse(token, func(token *jwt.Token) (any, error) {
		return &service.config.JWTPrivateKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("expected map claims, got %T", parsed.Claims)
	}
	if claims["demo"] != true {
		t.Fatalf("expected demo claim to be true, got %#v", claims["demo"])
	}
}

func TestRefreshSessionPreservesDemoMarker(t *testing.T) {
	service := newTestAuthService(t)
	demoUser := &clientsinfra.User{ID: "demo-user", Role: "resident", UserName: "demo_abc123", Email: "demo+abc123@example.com", FirstName: "Demo", LastName: "Visitor"}

	pair, err := service.IssueTokenPair(demoUser)
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}
	if !service.isDemoRefreshToken(pair.RefreshToken) {
		t.Fatalf("expected refresh token to be marked as demo")
	}

	refreshed, err := service.RefreshSession(pair.RefreshToken, &stubUserProvider{user: demoUser})
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if !service.isDemoRefreshToken(refreshed.RefreshToken) {
		t.Fatalf("expected refreshed token to keep demo marker")
	}

	parsed, err := jwt.Parse(refreshed.AccessToken, func(token *jwt.Token) (any, error) {
		return &service.config.JWTPrivateKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("parse refreshed access token: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("expected map claims, got %T", parsed.Claims)
	}
	if claims["demo"] != true {
		t.Fatalf("expected refreshed access token demo claim to be true, got %#v", claims["demo"])
	}
}

func TestRefreshSessionPreservesDemoMarkerWithoutHeuristicMatch(t *testing.T) {
	service := newTestAuthService(t)
	demoUser := &clientsinfra.User{ID: "demo-user", Role: "resident", UserName: "demo_abc123", Email: "demo+abc123@example.com", FirstName: "Demo", LastName: "Visitor"}

	pair, err := service.IssueTokenPair(demoUser)
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}

	reloadedUser := &clientsinfra.User{ID: "demo-user", Role: "resident", UserName: "resident_renamed", Email: "resident@example.com", FirstName: "Demo", LastName: "Visitor"}
	refreshed, err := service.RefreshSession(pair.RefreshToken, &stubUserProvider{user: reloadedUser})
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}

	parsed, err := jwt.Parse(refreshed.AccessToken, func(token *jwt.Token) (any, error) {
		return &service.config.JWTPrivateKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("parse refreshed access token: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("expected map claims, got %T", parsed.Claims)
	}
	if claims["demo"] != true {
		t.Fatalf("expected refreshed access token demo claim to stay true, got %#v", claims["demo"])
	}
}

func newTestAuthService(t *testing.T) *Service {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	return NewService(Config{
		JWTPrivateKey:   privateKey,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: time.Hour,
	}, newStubCacheStore())
}