package backend

import (
	"sync"
	"time"
)

type PlaybackCache struct {
	mu        sync.RWMutex
	payload   []byte
	updatedAt time.Time
}

func NewPlaybackCache() *PlaybackCache {
	return &PlaybackCache{}
}

func (c *PlaybackCache) Set(payload []byte) {
	if c == nil || len(payload) == 0 {
		return
	}
	c.mu.Lock()
	c.payload = append([]byte(nil), payload...)
	c.updatedAt = time.Now()
	c.mu.Unlock()
}

func (c *PlaybackCache) Get() ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	if len(c.payload) == 0 {
		c.mu.RUnlock()
		return nil, false
	}
	payload := append([]byte(nil), c.payload...)
	c.mu.RUnlock()
	return payload, true
}
