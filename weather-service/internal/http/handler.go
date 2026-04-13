package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/PetoAdam/homenavi/weather-service/internal/forecast"
)

// WeatherService is the application service surface exposed to HTTP handlers.
type WeatherService interface {
	SearchLocations(ctx context.Context, query string, limit int) ([]forecast.Location, error)
	ReverseGeocode(ctx context.Context, lat, lon float64) (forecast.Location, error)
	GetWeather(ctx context.Context, req forecast.WeatherRequest) (forecast.WeatherResponse, error)
}

type Handler struct {
	service WeatherService
}

func NewHandler(service WeatherService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) registerRoutes(r interface {
	Get(string, http.HandlerFunc)
}) {
	r.Get("/weather", h.handleWeather)
	r.Get("/weather/search", h.handleSearchLocations)
	r.Get("/weather/reverse", h.handleReverseGeocode)
}

func (h *Handler) handleSearchLocations(w http.ResponseWriter, r *http.Request) {
	limit := 5
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 10 {
			limit = l
		}
	}

	locations, err := h.service.SearchLocations(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		if errors.Is(err, forecast.ErrQueryRequired) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to search locations"})
		return
	}

	writeJSON(w, http.StatusOK, locations)
}

func (h *Handler) handleReverseGeocode(w http.ResponseWriter, r *http.Request) {
	lat, lon, ok := parseCoordinates(w, r)
	if !ok {
		return
	}

	location, err := h.service.ReverseGeocode(r.Context(), lat, lon)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to reverse geocode"})
		return
	}
	writeJSON(w, http.StatusOK, location)
}

func (h *Handler) handleWeather(w http.ResponseWriter, r *http.Request) {
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	var req forecast.WeatherRequest
	if city != "" {
		req.City = city
	}

	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	if latStr != "" || lonStr != "" {
		lat, lon, ok := parseCoordinates(w, r)
		if !ok {
			return
		}
		req.Latitude = &lat
		req.Longitude = &lon
	}

	weather, err := h.service.GetWeather(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, forecast.ErrLocationRequired):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "location is required (provide lat/lon or city)"})
		case errors.Is(err, forecast.ErrCityNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch weather"})
		}
		return
	}

	writeJSON(w, http.StatusOK, weather)
}

func parseCoordinates(w http.ResponseWriter, r *http.Request) (float64, float64, bool) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	if latStr == "" || lonStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lon parameters are required"})
		return 0, 0, false
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lat parameter"})
		return 0, 0, false
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lon parameter"})
		return 0, 0, false
	}
	return lat, lon, true
}
