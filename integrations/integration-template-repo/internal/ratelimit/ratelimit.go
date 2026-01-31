package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	last   time.Time
	tokens float64
}

// NewIPRateLimiter returns a very small, dependency-free limiter.
// rps: tokens/sec, burst: max tokens.
func NewIPRateLimiter(rps float64, burst float64) func(http.Handler) http.Handler {
	if rps <= 0 {
		rps = 5
	}
	if burst <= 0 {
		burst = 10
	}

	var (
		mu      sync.Mutex
		buckets = map[string]*bucket{}
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}

			mu.Lock()
			b, ok := buckets[host]
			if !ok {
				b = &bucket{last: time.Now(), tokens: burst}
				buckets[host] = b
			}

			now := time.Now()
			dt := now.Sub(b.last).Seconds()
			b.last = now
			b.tokens += dt * rps
			if b.tokens > burst {
				b.tokens = burst
			}
			if b.tokens < 1 {
				mu.Unlock()
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("rate limited"))
				return
			}
			b.tokens -= 1
			mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}
