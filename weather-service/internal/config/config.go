package config

import (
	"time"

	"github.com/PetoAdam/homenavi/shared/envx"
)

type Config struct {
	Port              string
	OpenWeatherAPIKey string
	CacheTTL          time.Duration
}

func Load() Config {
	port := envx.String("PORT", envx.String("WEATHER_SERVICE_PORT", "8095"))

	ttl := 15 * time.Minute
	if minutes := envx.Int("CACHE_TTL_MINUTES", 0); minutes > 0 {
		ttl = time.Duration(minutes) * time.Minute
	} else if minutes := envx.Int("WEATHER_CACHE_TTL_MINUTES", 0); minutes > 0 {
		ttl = time.Duration(minutes) * time.Minute
	} else if duration := envx.Duration("WEATHER_CACHE_TTL", 0); duration > 0 {
		ttl = duration
	}

	return Config{
		Port:              port,
		OpenWeatherAPIKey: envx.String("OPENWEATHER_API_KEY", ""),
		CacheTTL:          ttl,
	}
}
