package forecast

import (
	"context"
	"errors"
	"testing"
)

type fakeProvider struct {
	searchCalls  int
	reverseCalls int
	weatherCalls int

	searchResults   []Location
	reverseResult   Location
	weatherResult   WeatherResponse
	searchErr       error
	reverseErr      error
	weatherErr      error
	lastSearchQ     string
	lastSearchLim   int
	lastWeatherLat  float64
	lastWeatherLon  float64
	lastWeatherCity string
}

func (f *fakeProvider) SearchLocations(_ context.Context, query string, limit int) ([]Location, error) {
	f.searchCalls++
	f.lastSearchQ = query
	f.lastSearchLim = limit
	return f.searchResults, f.searchErr
}

func (f *fakeProvider) ReverseGeocode(_ context.Context, lat, lon float64) (Location, error) {
	f.reverseCalls++
	return f.reverseResult, f.reverseErr
}

func (f *fakeProvider) GetWeather(_ context.Context, lat, lon float64, city string) (WeatherResponse, error) {
	f.weatherCalls++
	f.lastWeatherLat = lat
	f.lastWeatherLon = lon
	f.lastWeatherCity = city
	return f.weatherResult, f.weatherErr
}

type fakeCache struct {
	items map[string]WeatherResponse
}

func newFakeCache() *fakeCache {
	return &fakeCache{items: map[string]WeatherResponse{}}
}

func (c *fakeCache) Get(key string) (WeatherResponse, bool) {
	v, ok := c.items[key]
	return v, ok
}

func (c *fakeCache) Set(key string, data WeatherResponse) {
	c.items[key] = data
}

func TestSearchLocationsRequiresQuery(t *testing.T) {
	svc := NewService(&fakeProvider{}, newFakeCache())

	_, err := svc.SearchLocations(context.Background(), "  ", 0)
	if !errors.Is(err, ErrQueryRequired) {
		t.Fatalf("expected ErrQueryRequired, got %v", err)
	}
}

func TestGetWeatherUsesCache(t *testing.T) {
	provider := &fakeProvider{weatherResult: WeatherResponse{City: "Budapest"}}
	cache := newFakeCache()
	svc := NewService(provider, cache)
	lat := 47.49
	lon := 19.04

	first, err := svc.GetWeather(context.Background(), WeatherRequest{City: "Budapest", Latitude: &lat, Longitude: &lon})
	if err != nil {
		t.Fatalf("first call returned error: %v", err)
	}
	second, err := svc.GetWeather(context.Background(), WeatherRequest{City: "Budapest", Latitude: &lat, Longitude: &lon})
	if err != nil {
		t.Fatalf("second call returned error: %v", err)
	}
	if provider.weatherCalls != 1 {
		t.Fatalf("expected provider weather to be called once, got %d", provider.weatherCalls)
	}
	if first.City != second.City {
		t.Fatalf("expected cached result to match first result")
	}
}

func TestGetWeatherResolvesCityCoordinates(t *testing.T) {
	provider := &fakeProvider{
		searchResults: []Location{{Name: "Budapest", Lat: 47.4979, Lon: 19.0402}},
		weatherResult: WeatherResponse{City: "Budapest"},
	}
	svc := NewService(provider, newFakeCache())

	_, err := svc.GetWeather(context.Background(), WeatherRequest{City: "Budapest"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.searchCalls != 1 {
		t.Fatalf("expected search call, got %d", provider.searchCalls)
	}
	if provider.lastWeatherLat != 47.4979 || provider.lastWeatherLon != 19.0402 {
		t.Fatalf("expected weather request to use resolved coordinates, got %f,%f", provider.lastWeatherLat, provider.lastWeatherLon)
	}
}

func TestGetWeatherRequiresLocation(t *testing.T) {
	svc := NewService(&fakeProvider{}, newFakeCache())

	_, err := svc.GetWeather(context.Background(), WeatherRequest{})
	if !errors.Is(err, ErrLocationRequired) {
		t.Fatalf("expected ErrLocationRequired, got %v", err)
	}
}

func TestGetWeatherCityNotFound(t *testing.T) {
	svc := NewService(&fakeProvider{searchResults: []Location{}}, newFakeCache())

	_, err := svc.GetWeather(context.Background(), WeatherRequest{City: "Atlantis"})
	if !errors.Is(err, ErrCityNotFound) {
		t.Fatalf("expected ErrCityNotFound, got %v", err)
	}
}
