package cache

import (
	"sync"
	"time"

	"github.com/PetoAdam/homenavi/weather-service/internal/forecast"
)

type entry struct {
	data      forecast.WeatherResponse
	expiresAt time.Time
}

// MemoryCache is an in-memory TTL cache for weather responses.
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]entry
	ttl   time.Duration
}

func NewMemoryCache(ttl time.Duration) *MemoryCache {
	return &MemoryCache{items: make(map[string]entry), ttl: ttl}
}

func (c *MemoryCache) Get(key string) (forecast.WeatherResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok || time.Now().After(e.expiresAt) {
		return forecast.WeatherResponse{}, false
	}
	return e.data, true
}

func (c *MemoryCache) Set(key string, data forecast.WeatherResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = entry{data: data, expiresAt: time.Now().Add(c.ttl)}
}

var _ forecast.Cache = (*MemoryCache)(nil)
