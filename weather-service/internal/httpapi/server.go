package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"weather-service/internal/cache"
	"weather-service/internal/models"
	"weather-service/internal/owm"

	"github.com/go-chi/chi/v5"
)

type Server struct {
	owm   *owm.Client
	cache *cache.Cache
}

func NewServer(owmClient *owm.Client, weatherCache *cache.Cache) *Server {
	return &Server{owm: owmClient, cache: weatherCache}
}

func (s *Server) RegisterRoutes(r chi.Router) {
	r.Get("/weather", s.handleWeather)
	r.Get("/weather/search", s.handleSearchLocations)
	r.Get("/weather/reverse", s.handleReverseGeocode)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) handleSearchLocations(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter 'q' is required"})
		return
	}

	limit := 5
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 10 {
			limit = l
		}
	}

	locations, err := s.owm.SearchLocations(r.Context(), query, limit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to search locations"})
		return
	}

	// Return a plain array for frontend convenience.
	writeJSON(w, http.StatusOK, locations)
}

func (s *Server) handleReverseGeocode(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	if latStr == "" || lonStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lon parameters are required"})
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lat parameter"})
		return
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lon parameter"})
		return
	}

	location, err := s.owm.ReverseGeocode(r.Context(), lat, lon)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to reverse geocode"})
		return
	}

	writeJSON(w, http.StatusOK, location)
}

func (s *Server) handleWeather(w http.ResponseWriter, r *http.Request) {
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")

	var lat, lon float64
	var err error

	if latStr != "" && lonStr != "" {
		lat, err = strconv.ParseFloat(latStr, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lat parameter"})
			return
		}
		lon, err = strconv.ParseFloat(lonStr, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lon parameter"})
			return
		}

		if city == "" {
			loc, _ := s.owm.ReverseGeocode(r.Context(), lat, lon)
			city = loc.Name
		}
	} else if city != "" {
		locations, err := s.owm.SearchLocations(r.Context(), city, 1)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to resolve city"})
			return
		}
		if len(locations) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "city not found"})
			return
		}
		lat = locations[0].Lat
		lon = locations[0].Lon
		city = locations[0].Name
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "location is required (provide lat/lon or city)"})
		return
	}

	cacheKey := fmt.Sprintf("%.2f,%.2f", lat, lon)
	if cached, ok := s.cache.Get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	weather, err := s.owm.GetWeather(r.Context(), lat, lon, city)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch weather"})
		return
	}

	s.cache.Set(cacheKey, weather)
	writeJSON(w, http.StatusOK, weather)
}

// compile-time guard for shared types not being accidentally removed
var _ models.WeatherResponse
