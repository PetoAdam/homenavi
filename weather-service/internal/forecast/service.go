package forecast

import (
	"context"
	"fmt"
	"strings"
)

// Provider resolves locations and weather data from an external data source.
type Provider interface {
	SearchLocations(ctx context.Context, query string, limit int) ([]Location, error)
	ReverseGeocode(ctx context.Context, lat, lon float64) (Location, error)
	GetWeather(ctx context.Context, lat, lon float64, city string) (WeatherResponse, error)
}

// Cache stores weather responses keyed by normalized coordinates.
type Cache interface {
	Get(key string) (WeatherResponse, bool)
	Set(key string, data WeatherResponse)
}

// Service orchestrates location resolution, caching, and weather retrieval.
type Service struct {
	provider Provider
	cache    Cache
}

func NewService(provider Provider, cache Cache) *Service {
	return &Service{provider: provider, cache: cache}
}

func (s *Service) SearchLocations(ctx context.Context, query string, limit int) ([]Location, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrQueryRequired
	}
	if limit <= 0 {
		limit = 5
	}
	return s.provider.SearchLocations(ctx, query, limit)
}

func (s *Service) ReverseGeocode(ctx context.Context, lat, lon float64) (Location, error) {
	return s.provider.ReverseGeocode(ctx, lat, lon)
}

func (s *Service) GetWeather(ctx context.Context, req WeatherRequest) (WeatherResponse, error) {
	city := strings.TrimSpace(req.City)

	var (
		lat float64
		lon float64
	)

	switch {
	case req.Latitude != nil && req.Longitude != nil:
		lat = *req.Latitude
		lon = *req.Longitude
		if city == "" {
			loc, err := s.provider.ReverseGeocode(ctx, lat, lon)
			if err == nil {
				city = loc.Name
			}
		}
	case city != "":
		locations, err := s.provider.SearchLocations(ctx, city, 1)
		if err != nil {
			return WeatherResponse{}, err
		}
		if len(locations) == 0 {
			return WeatherResponse{}, ErrCityNotFound
		}
		lat = locations[0].Lat
		lon = locations[0].Lon
		city = locations[0].Name
	default:
		return WeatherResponse{}, ErrLocationRequired
	}

	cacheKey := fmt.Sprintf("%.2f,%.2f", lat, lon)
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached, nil
	}

	weather, err := s.provider.GetWeather(ctx, lat, lon, city)
	if err != nil {
		return WeatherResponse{}, err
	}
	if city != "" && weather.City == "" {
		weather.City = city
	}
	s.cache.Set(cacheKey, weather)
	return weather, nil
}
