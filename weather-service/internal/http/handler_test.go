package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PetoAdam/homenavi/weather-service/internal/forecast"
)

type fakeService struct {
	searchResults []forecast.Location
	reverseResult forecast.Location
	weatherResult forecast.WeatherResponse
	searchErr     error
	reverseErr    error
	weatherErr    error
}

func (f *fakeService) SearchLocations(context.Context, string, int) ([]forecast.Location, error) {
	return f.searchResults, f.searchErr
}
func (f *fakeService) ReverseGeocode(context.Context, float64, float64) (forecast.Location, error) {
	return f.reverseResult, f.reverseErr
}
func (f *fakeService) GetWeather(context.Context, forecast.WeatherRequest) (forecast.WeatherResponse, error) {
	return f.weatherResult, f.weatherErr
}

func TestHandleSearchLocationsRequiresQuery(t *testing.T) {
	h := NewHandler(&fakeService{searchErr: forecast.ErrQueryRequired})
	req := httptest.NewRequest(http.MethodGet, "/api/weather/search", nil)
	rr := httptest.NewRecorder()

	h.handleSearchLocations(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleWeatherMapsLocationRequired(t *testing.T) {
	h := NewHandler(&fakeService{weatherErr: forecast.ErrLocationRequired})
	req := httptest.NewRequest(http.MethodGet, "/api/weather", nil)
	rr := httptest.NewRecorder()

	h.handleWeather(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleWeatherReturnsResponse(t *testing.T) {
	h := NewHandler(&fakeService{weatherResult: forecast.WeatherResponse{City: "Budapest"}})
	req := httptest.NewRequest(http.MethodGet, "/api/weather?city=Budapest", nil)
	rr := httptest.NewRecorder()

	h.handleWeather(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload forecast.WeatherResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.City != "Budapest" {
		t.Fatalf("expected Budapest, got %q", payload.City)
	}
}

func TestHandleReverseGeocodeValidatesCoordinates(t *testing.T) {
	h := NewHandler(&fakeService{})
	req := httptest.NewRequest(http.MethodGet, "/api/weather/reverse?lat=x&lon=10", nil)
	rr := httptest.NewRecorder()

	h.handleReverseGeocode(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestRouterHealth(t *testing.T) {
	router := NewRouter(NewHandler(&fakeService{}))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestNewCORSOptionsDisablesCredentialsWithWildcardOrigin(t *testing.T) {
	opts := newCORSOptions()
	if len(opts.AllowedOrigins) != 1 || opts.AllowedOrigins[0] != "*" {
		t.Fatalf("expected wildcard allowed origin, got %#v", opts.AllowedOrigins)
	}
	if opts.AllowCredentials {
		t.Fatal("expected credentials disabled")
	}
}

func TestHandleWeatherMapsUnexpectedErrorToBadGateway(t *testing.T) {
	h := NewHandler(&fakeService{weatherErr: errors.New("boom")})
	req := httptest.NewRequest(http.MethodGet, "/api/weather?city=Budapest", nil)
	rr := httptest.NewRecorder()

	h.handleWeather(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rr.Code)
	}
}
