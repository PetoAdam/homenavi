package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port              string
	OpenWeatherAPIKey string
	CacheTTL          time.Duration
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("WEATHER_SERVICE_PORT")
	}
	if port == "" {
		port = "8095"
	}

	ttl := 15 * time.Minute
	if v := os.Getenv("CACHE_TTL_MINUTES"); v != "" {
		if m, err := strconv.Atoi(v); err == nil && m > 0 {
			ttl = time.Duration(m) * time.Minute
		}
	} else if v := os.Getenv("WEATHER_CACHE_TTL_MINUTES"); v != "" {
		if m, err := strconv.Atoi(v); err == nil && m > 0 {
			ttl = time.Duration(m) * time.Minute
		}
	} else if v := os.Getenv("WEATHER_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			ttl = d
		}
	}

	return Config{
		Port:              port,
		OpenWeatherAPIKey: os.Getenv("OPENWEATHER_API_KEY"),
		CacheTTL:          ttl,
	}
}
