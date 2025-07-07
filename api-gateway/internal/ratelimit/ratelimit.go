package ratelimit

import (
	"context"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type LimiterConfig struct {
	RPS   int
	Burst int
}

type RateLimiter struct {
	Redis  *redis.Client
	Prefix string
	Config LimiterConfig
}

func New(redis *redis.Client, prefix string, cfg LimiterConfig) *RateLimiter {
	return &RateLimiter{Redis: redis, Prefix: prefix, Config: cfg}
}

func (rl *RateLimiter) Middleware(keyFunc func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rl.Prefix + ":" + keyFunc(r)
			ctx := context.Background()
			allowed, err := rl.allow(ctx, key)
			if err != nil {
				http.Error(w, "Rate limiter error", http.StatusInternalServerError)
				return
			}
			if !allowed {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(ctx context.Context, key string) (bool, error) {
	// Token bucket algorithm using Redis Lua script
	// KEYS[1] = key
	// ARGV[1] = max_tokens (burst)
	// ARGV[2] = refill_rate (tokens per second)
	// ARGV[3] = now (ms)
	// Returns: 1 if allowed, 0 if not
	lua := `
local tokens_key = KEYS[1]
local max_tokens = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local bucket = redis.call('HMGET', tokens_key, 'tokens', 'last')
local tokens = tonumber(bucket[1]) or max_tokens
local last = tonumber(bucket[2]) or now
local delta = math.max(0, now - last) / 1000
local refill = math.floor(delta * refill_rate)
tokens = math.min(max_tokens, tokens + refill)
if tokens > 0 then
  tokens = tokens - 1
  redis.call('HMSET', tokens_key, 'tokens', tokens, 'last', now)
  redis.call('EXPIRE', tokens_key, 2)
  return 1
else
  redis.call('HMSET', tokens_key, 'tokens', tokens, 'last', now)
  redis.call('EXPIRE', tokens_key, 2)
  return 0
end
`
	maxTokens := rl.Config.Burst
	refillRate := rl.Config.RPS
	now := time.Now().UnixNano() / int64(time.Millisecond)
	res, err := rl.Redis.Eval(ctx, lua, []string{key}, maxTokens, refillRate, now).Result()
	if err != nil {
		log.Printf("Redis error for key %s: %v", key, err)
		return false, err
	}
	var allowed int64
	switch v := res.(type) {
	case int64:
		allowed = v
	case string:
		allowed, _ = strconv.ParseInt(v, 10, 64)
	default:
		allowed = 0
	}
	log.Printf("Token bucket key %s: allowed=%d (max=%d, rps=%d)", key, allowed, maxTokens, refillRate)
	return allowed == 1, nil
}

// Key by IP address
func KeyByIP(r *http.Request) string {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// Key by user (JWT sub) if available, else IP
func KeyByUserOrIP(r *http.Request) string {
	if claims, ok := r.Context().Value("claims").(interface{ GetSubject() string }); ok {
		if sub := claims.GetSubject(); sub != "" {
			return sub
		}
	}
	return KeyByIP(r)
}
