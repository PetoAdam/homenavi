package cache

import (
	"sync"
	"time"

	"weather-service/internal/models"
)

type entry struct {
	data      models.WeatherResponse
	expiresAt time.Time
}

type Cache struct {
	mu    sync.RWMutex
	items map[string]entry
	ttl   time.Duration
}

func New(ttl time.Duration) *Cache {
	return &Cache{items: make(map[string]entry), ttl: ttl}
}

func (c *Cache) Get(key string) (models.WeatherResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok || time.Now().After(e.expiresAt) {
		return models.WeatherResponse{}, false
	}
	return e.data, true
}

func (c *Cache) Set(key string, data models.WeatherResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = entry{data: data, expiresAt: time.Now().Add(c.ttl)}
}
