package owm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"weather-service/internal/models"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
}

type httpStatusError struct {
	status int
	body   string
}

func (e httpStatusError) Error() string {
	if e.body == "" {
		return fmt.Sprintf("API returned status %d", e.status)
	}
	return fmt.Sprintf("API returned status %d: %s", e.status, e.body)
}

func isAuthFailure(err error) bool {
	var se httpStatusError
	if !errors.As(err, &se) {
		return false
	}
	return se.status == http.StatusUnauthorized || se.status == http.StatusForbidden
}

func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) SearchLocations(ctx context.Context, query string, limit int) ([]models.Location, error) {
	if c.apiKey == "" {
		return c.mockSearchLocations(query), nil
	}

	u := fmt.Sprintf(
		"http://api.openweathermap.org/geo/1.0/direct?q=%s&limit=%d&appid=%s",
		url.QueryEscape(query), limit, c.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// If the key is invalid / not yet activated, fall back to mock data
		// so the dashboard stays usable.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return c.mockSearchLocations(query), nil
		}
		return nil, fmt.Errorf("geocoding API returned status %d", resp.StatusCode)
	}

	var results []struct {
		Name    string  `json:"name"`
		Country string  `json:"country"`
		State   string  `json:"state"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	locations := make([]models.Location, len(results))
	for i, r := range results {
		locations[i] = models.Location{Name: r.Name, Country: r.Country, State: r.State, Lat: r.Lat, Lon: r.Lon}
	}
	return locations, nil
}

func (c *Client) ReverseGeocode(ctx context.Context, lat, lon float64) (models.Location, error) {
	if c.apiKey == "" {
		return models.Location{Name: "Unknown", Lat: lat, Lon: lon}, nil
	}

	u := fmt.Sprintf(
		"http://api.openweathermap.org/geo/1.0/reverse?lat=%f&lon=%f&limit=1&appid=%s",
		lat, lon, c.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return models.Location{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return models.Location{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return models.Location{Name: "Unknown", Lat: lat, Lon: lon}, nil
		}
		return models.Location{}, fmt.Errorf("reverse geocoding API returned status %d", resp.StatusCode)
	}

	var results []struct {
		Name    string  `json:"name"`
		Country string  `json:"country"`
		State   string  `json:"state"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return models.Location{}, err
	}
	if len(results) == 0 {
		return models.Location{Name: "Unknown", Lat: lat, Lon: lon}, nil
	}

	r := results[0]
	return models.Location{Name: r.Name, Country: r.Country, State: r.State, Lat: r.Lat, Lon: r.Lon}, nil
}

func (c *Client) GetWeather(ctx context.Context, lat, lon float64, city string) (models.WeatherResponse, error) {
	if c.apiKey == "" {
		return c.mockWeather(city), nil
	}

	currentURL := fmt.Sprintf(
		"https://api.openweathermap.org/data/2.5/weather?lat=%f&lon=%f&units=metric&appid=%s",
		lat, lon, c.apiKey,
	)

	forecastURL := fmt.Sprintf(
		"https://api.openweathermap.org/data/2.5/forecast?lat=%f&lon=%f&units=metric&appid=%s",
		lat, lon, c.apiKey,
	)

	oneCallURL := fmt.Sprintf(
		"https://api.openweathermap.org/data/2.5/onecall?lat=%f&lon=%f&units=metric&exclude=minutely,alerts&appid=%s",
		lat, lon, c.apiKey,
	)

	currentResp, err := c.fetchJSON(ctx, currentURL)
	if err != nil {
		if isAuthFailure(err) {
			return c.mockWeather(city), nil
		}
		return models.WeatherResponse{}, fmt.Errorf("fetching current weather: %w", err)
	}
	forecastResp, err := c.fetchJSON(ctx, forecastURL)
	if err != nil {
		if isAuthFailure(err) {
			return c.mockWeather(city), nil
		}
		return models.WeatherResponse{}, fmt.Errorf("fetching forecast: %w", err)
	}

	main := getMap(currentResp, "main")
	weather := getFirstInArray(currentResp, "weather")

	current := models.CurrentWeather{
		TempC: getFloat(main, "temp"),
		HiC:   getFloat(main, "temp_max"),
		LoC:   getFloat(main, "temp_min"),
		Desc:  getString(weather, "description"),
		Icon:  mapOWMIcon(getString(weather, "icon")),
	}

	// Prefer One Call for next-24-hours + next-7-days.
	// If it fails (e.g., API plan/limits), fall back to the 5-day/3h forecast.
	now := time.Now()
	if oneCallResp, err := c.fetchJSON(ctx, oneCallURL); err == nil {
		hourly := getArray(oneCallResp, "hourly")
		dailySrc := getArray(oneCallResp, "daily")

		daily := make([]models.ForecastItem, 0, 24)
		for _, item := range hourly {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			dt := int64(getFloat(itemMap, "dt"))
			t := time.Unix(dt, 0)
			if t.Before(now) {
				continue
			}
			itemWeather := getFirstInArray(itemMap, "weather")
			daily = append(daily, models.ForecastItem{
				Hour:  fmt.Sprintf("%02d", t.Hour()),
				TempC: getFloat(itemMap, "temp"),
				Icon:  mapOWMIcon(getString(itemWeather, "icon")),
			})
			if len(daily) >= 24 {
				break
			}
		}

		weekly := make([]models.ForecastItem, 0, 7)
		for _, item := range dailySrc {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			dt := int64(getFloat(itemMap, "dt"))
			t := time.Unix(dt, 0)
			if t.Before(now.Add(-12 * time.Hour)) {
				continue
			}
			tempMap := getMap(itemMap, "temp")
			itemWeather := getFirstInArray(itemMap, "weather")
			weekly = append(weekly, models.ForecastItem{
				Day:   t.Weekday().String()[:3],
				TempC: getFloat(tempMap, "day"),
				Icon:  mapOWMIcon(getString(itemWeather, "icon")),
			})
			if len(weekly) >= 7 {
				break
			}
		}

		// If One Call provided at least some usable data, return it.
		if len(daily) > 0 && len(weekly) > 0 {
			return models.WeatherResponse{City: city, Current: current, Daily: daily, Weekly: weekly}, nil
		}
	}

	list := getArray(forecastResp, "list")
	daily := make([]models.ForecastItem, 0, 24)
	weekly := make([]models.ForecastItem, 0, 7)
	seenDays := make(map[string]bool)
	// keep `now` from above

	for _, item := range list {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		dt := int64(getFloat(itemMap, "dt"))
		t := time.Unix(dt, 0)
		itemMain := getMap(itemMap, "main")
		itemWeather := getFirstInArray(itemMap, "weather")

		// Next 24 hours. The 5-day forecast is in 3-hour steps, so expand each step into 3 hourly items.
		if !t.Before(now) && len(daily) < 24 {
			tempC := getFloat(itemMain, "temp")
			icon := mapOWMIcon(getString(itemWeather, "icon"))
			for k := 0; k < 3 && len(daily) < 24; k++ {
				tt := t.Add(time.Duration(k) * time.Hour)
				daily = append(daily, models.ForecastItem{
					Hour:  fmt.Sprintf("%02d", tt.Hour()),
					TempC: tempC,
					Icon:  icon,
				})
			}
		}

		dayKey := t.Format("2006-01-02")
		if !seenDays[dayKey] && t.Hour() >= 11 && t.Hour() <= 14 {
			seenDays[dayKey] = true
			weekly = append(weekly, models.ForecastItem{
				Day:   t.Weekday().String()[:3],
				TempC: getFloat(itemMain, "temp"),
				Icon:  mapOWMIcon(getString(itemWeather, "icon")),
			})
			if len(weekly) >= 7 {
				break
			}
		}
	}

	// If the 5-day forecast doesn't have enough unique days, pad to 7 to keep UI stable.
	if len(weekly) > 0 && len(weekly) < 7 {
		last := weekly[len(weekly)-1]
		for i := len(weekly); i < 7; i++ {
			t := now.AddDate(0, 0, i)
			weekly = append(weekly, models.ForecastItem{
				Day:   t.Weekday().String()[:3],
				TempC: last.TempC,
				Icon:  last.Icon,
			})
		}
	}

	return models.WeatherResponse{City: city, Current: current, Daily: daily, Weekly: weekly}, nil
}

func (c *Client) fetchJSON(ctx context.Context, u string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, httpStatusError{status: resp.StatusCode, body: string(body)}
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) mockSearchLocations(query string) []models.Location {
	cities := []models.Location{
		{Name: "Budapest", Country: "HU", Lat: 47.4979, Lon: 19.0402},
		{Name: "London", Country: "GB", Lat: 51.5074, Lon: -0.1278},
		{Name: "New York", Country: "US", State: "New York", Lat: 40.7128, Lon: -74.0060},
		{Name: "Tokyo", Country: "JP", Lat: 35.6762, Lon: 139.6503},
		{Name: "Paris", Country: "FR", Lat: 48.8566, Lon: 2.3522},
		{Name: "Berlin", Country: "DE", Lat: 52.5200, Lon: 13.4050},
		{Name: "Sydney", Country: "AU", Lat: -33.8688, Lon: 151.2093},
		{Name: "San Francisco", Country: "US", State: "California", Lat: 37.7749, Lon: -122.4194},
		{Name: "Amsterdam", Country: "NL", Lat: 52.3676, Lon: 4.9041},
		{Name: "Vienna", Country: "AT", Lat: 48.2082, Lon: 16.3738},
	}

	q := strings.ToLower(query)
	var matches []models.Location
	for _, city := range cities {
		if strings.Contains(strings.ToLower(city.Name), q) {
			matches = append(matches, city)
		}
	}
	if len(matches) > 5 {
		matches = matches[:5]
	}
	return matches
}

func (c *Client) mockWeather(city string) models.WeatherResponse {
	// Provide stable sample data matching the UI expectations:
	// - Daily: next 24 hours
	// - Weekly: next 7 days
	now := time.Now()
	daily := make([]models.ForecastItem, 0, 24)
	for i := 0; i < 24; i++ {
		t := now.Add(time.Duration(i) * time.Hour)
		icon := "cloud"
		if i%6 == 0 {
			icon = "sun"
		} else if i%6 == 3 {
			icon = "cloud_sun"
		}
		daily = append(daily, models.ForecastItem{
			Hour:  fmt.Sprintf("%02d", t.Hour()),
			TempC: 18 + float64((i%8)-3),
			Icon:  icon,
		})
	}

	weekly := make([]models.ForecastItem, 0, 7)
	for i := 0; i < 7; i++ {
		t := now.AddDate(0, 0, i)
		icon := "cloud"
		if i%3 == 0 {
			icon = "sun"
		} else if i%3 == 1 {
			icon = "cloud_sun"
		}
		weekly = append(weekly, models.ForecastItem{
			Day:   t.Weekday().String()[:3],
			TempC: 20 + float64((i%5)-2),
			Icon:  icon,
		})
	}

	return models.WeatherResponse{
		City: city,
		Current: models.CurrentWeather{
			TempC: 22,
			HiC:   24,
			LoC:   15,
			Desc:  "Sunny",
			Icon:  "sun",
		},
		Daily:  daily,
		Weekly: weekly,
	}
}

func getMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

func getArray(m map[string]any, key string) []any {
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}

func getFirstInArray(m map[string]any, key string) map[string]any {
	arr := getArray(m, key)
	if len(arr) > 0 {
		if v, ok := arr[0].(map[string]any); ok {
			return v
		}
	}
	return nil
}

func getFloat(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func mapOWMIcon(code string) string {
	code = strings.TrimSuffix(code, "d")
	code = strings.TrimSuffix(code, "n")

	switch code {
	case "01":
		return "sun"
	case "02":
		return "cloud_sun"
	case "03", "04":
		return "cloud"
	case "09", "10":
		return "rain"
	case "11":
		return "storm"
	case "13":
		return "snow"
	case "50":
		return "fog"
	default:
		return "sun"
	}
}
