package cache

import (
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/weather-service/internal/forecast"
)

func TestMemoryCacheExpiresEntries(t *testing.T) {
	cache := NewMemoryCache(10 * time.Millisecond)
	cache.Set("key", forecast.WeatherResponse{City: "Budapest"})

	if got, ok := cache.Get("key"); !ok || got.City != "Budapest" {
		t.Fatalf("expected cached item before expiry")
	}

	time.Sleep(15 * time.Millisecond)
	if _, ok := cache.Get("key"); ok {
		t.Fatal("expected cached item to expire")
	}
}
