package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
)

type memoryRefreshStore struct {
	values map[string]string
}

func newMemoryRefreshStore() *memoryRefreshStore {
	return &memoryRefreshStore{values: make(map[string]string)}
}

func (s *memoryRefreshStore) Set(_ context.Context, key, value string, _ time.Duration) error {
	s.values[key] = value
	return nil
}

func (s *memoryRefreshStore) Get(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", cacheinfra.ErrNotFound
	}
	return value, nil
}

func (s *memoryRefreshStore) Delete(_ context.Context, keys ...string) error {
	for _, key := range keys {
		delete(s.values, key)
	}
	return nil
}

func (s *memoryRefreshStore) TTL(_ context.Context, key string) (time.Duration, error) {
	if _, ok := s.values[key]; !ok {
		return 0, cacheinfra.ErrNotFound
	}
	return time.Hour, nil
}

func (s *memoryRefreshStore) Increment(_ context.Context, key string) (int64, error) {
	return 0, nil
}

func (s *memoryRefreshStore) Expire(_ context.Context, key string, _ time.Duration) error {
	return nil
}

func (s *memoryRefreshStore) GetDelete(ctx context.Context, key string) (string, error) {
	value, err := s.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return value, s.Delete(ctx, key)
}

func (s *memoryRefreshStore) RotateRefreshToken(_ context.Context, currentKey, consumedKey, replacementKey, familyKeyPrefix string) (string, cacheinfra.RefreshTokenRotationStatus, error) {
	current, ok := s.values[currentKey]
	if !ok {
		if familyID, consumed := s.values[consumedKey]; consumed {
			s.values[familyKeyPrefix+familyID] = refreshTokenFamilyRevoked
			return "", cacheinfra.RefreshTokenReplayed, nil
		}
		return "", cacheinfra.RefreshTokenMissing, nil
	}
	record, err := decodeRefreshTokenRecord(current)
	if err != nil {
		return "", cacheinfra.RefreshTokenMissing, err
	}
	if s.values[familyKeyPrefix+record.FamilyID] != refreshTokenFamilyActive {
		return "", cacheinfra.RefreshTokenFamilyRevoked, nil
	}
	delete(s.values, currentKey)
	s.values[consumedKey] = current
	s.values[replacementKey] = current
	return current, cacheinfra.RefreshTokenRotated, nil
}

func (s *memoryRefreshStore) Close() error {
	return nil
}

type refreshTestUsers struct {
	user *clientsinfra.User
}

func (u refreshTestUsers) GetUser(userID string) (*clientsinfra.User, error) {
	if u.user == nil || u.user.ID != userID {
		return nil, cacheinfra.ErrNotFound
	}
	return u.user, nil
}

func newRefreshTestService(t *testing.T, store *memoryRefreshStore) *Service {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return NewService(Config{
		JWTPrivateKey:   privateKey,
		JWTIssuer:       "https://auth.example.test",
		JWTAPIAudience:  "https://api.example.test",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	}, store)
}

func TestRefreshSessionRotatesHashedTokenAndRejectsReplay(t *testing.T) {
	store := newMemoryRefreshStore()
	service := newRefreshTestService(t, store)
	user := &clientsinfra.User{ID: "user-1", Role: "resident"}
	originalToken, err := service.IssueRefreshToken(user.ID)
	if err != nil {
		t.Fatalf("issue refresh token: %v", err)
	}
	for key := range store.values {
		if strings.Contains(key, originalToken) {
			t.Fatalf("refresh token value was stored in cache key %q", key)
		}
	}

	tokens, err := service.RefreshSession(originalToken, refreshTestUsers{user: user})
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if tokens.RefreshToken == originalToken || tokens.AccessToken == "" {
		t.Fatalf("expected a new refresh token and access token, got %#v", tokens)
	}
	if _, err := service.ValidateRefreshToken(tokens.RefreshToken); err != nil {
		t.Fatalf("validate rotated refresh token: %v", err)
	}

	if _, err := service.RefreshSession(originalToken, refreshTestUsers{user: user}); err == nil {
		t.Fatal("expected replayed refresh token to be rejected")
	}
	if _, err := service.ValidateRefreshToken(tokens.RefreshToken); err == nil {
		t.Fatal("expected replay to revoke the replacement token family")
	}
}

func TestRevokeRefreshTokenRevokesFamily(t *testing.T) {
	store := newMemoryRefreshStore()
	service := newRefreshTestService(t, store)
	token, err := service.IssueRefreshToken("user-1")
	if err != nil {
		t.Fatalf("issue refresh token: %v", err)
	}
	if err := service.RevokeRefreshToken(token); err != nil {
		t.Fatalf("revoke refresh token: %v", err)
	}
	if _, err := service.ValidateRefreshToken(token); err == nil {
		t.Fatal("expected revoked refresh token to be rejected")
	}
}

func TestRefreshReplayRevokesAccessTokenSession(t *testing.T) {
	store := newMemoryRefreshStore()
	service := newRefreshTestService(t, store)
	user := &clientsinfra.User{ID: "user-1", Role: "resident"}
	tokens, err := service.IssueTokenPair(user)
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}
	parsed, _, err := new(jwt.Parser).ParseUnverified(tokens.AccessToken, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	sessionID, _ := claims[authx.ClaimSessionID].(string)
	if sessionID == "" {
		t.Fatal("expected access token session ID")
	}

	if _, err := service.RefreshSession(tokens.RefreshToken, refreshTestUsers{user: user}); err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if _, err := service.RefreshSession(tokens.RefreshToken, refreshTestUsers{user: user}); err == nil {
		t.Fatal("expected refresh replay to fail")
	}
	if got := store.values[authx.SessionStatusKey(sessionID)]; got != authx.SessionStatusRevoked {
		t.Fatalf("session status = %q, want %q", got, authx.SessionStatusRevoked)
	}
}
