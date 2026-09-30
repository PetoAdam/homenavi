package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type stubRedisClient struct {
	redis.UniversalClient
	values map[string]string
	ttls   map[string]time.Duration
}

func newStubRedisClient() *stubRedisClient {
	return &stubRedisClient{values: map[string]string{}, ttls: map[string]time.Duration{}}
}

func (s *stubRedisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	cmd := redis.NewCmd(ctx)
	if len(keys) != 1 || len(args) < 3 {
		cmd.SetVal(int64(0))
		return cmd
	}
	key := keys[0]
	cutoff, _ := args[0].(int64)
	updatedAt, _ := args[1].(int64)
	ttlSeconds, _ := args[2].(int)
	if current, ok := s.values[key]; ok {
		var currentNum int64
		_, _ = fmt.Sscan(current, &currentNum)
		if currentNum > cutoff {
			cmd.SetVal(int64(0))
			return cmd
		}
	}
	s.values[key] = fmt.Sprintf("%d", updatedAt)
	s.ttls[key] = time.Duration(ttlSeconds) * time.Second
	cmd.SetVal(int64(1))
	return cmd
}

func TestJWTAuthMiddlewareTracksDemoActivity(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, &Claims{
		Role: "resident",
		Name: "Demo Visitor",
		Demo: true,
		RegisteredClaims: jwt.RegisteredClaims{Subject: "demo-user"},
	})
	tokenStr, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	redisClient := newStubRedisClient()
	tracker := NewDemoActivityTracker(redisClient, 60*time.Second, 15*time.Minute)
	tracker.now = func() time.Time { return time.Unix(1_000, 0) }
	h := JWTAuthMiddlewareRS256(&privateKey.PublicKey, tracker)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if got := redisClient.values["demo:last-active:demo-user"]; got != "1000" {
		t.Fatalf("expected tracked timestamp, got %q", got)
	}
	if got := redisClient.ttls["demo:last-active:demo-user"]; got != 15*time.Minute {
		t.Fatalf("expected TTL to be 15m, got %v", got)
	}
}

func TestDemoActivityTrackerDebouncesWrites(t *testing.T) {
	redisClient := newStubRedisClient()
	tracker := NewDemoActivityTracker(redisClient, 60*time.Second, 15*time.Minute)
	tracker.now = func() time.Time { return time.Unix(1_000, 0) }
	claims := &Claims{Demo: true, RegisteredClaims: jwt.RegisteredClaims{Subject: "demo-user"}}

	tracker.Track(context.Background(), claims)
	tracker.now = func() time.Time { return time.Unix(1_030, 0) }
	tracker.Track(context.Background(), claims)

	if got := redisClient.values["demo:last-active:demo-user"]; got != "1000" {
		t.Fatalf("expected second write to be debounced, got %q", got)
	}

	tracker.now = func() time.Time { return time.Unix(1_061, 0) }
	tracker.Track(context.Background(), claims)
	if got := redisClient.values["demo:last-active:demo-user"]; got != "1061" {
		t.Fatalf("expected write after debounce window, got %q", got)
	}
}