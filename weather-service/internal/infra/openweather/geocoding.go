package openweather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/PetoAdam/homenavi/weather-service/internal/forecast"
)

func (c *Client) SearchLocations(ctx context.Context, query string, limit int) ([]forecast.Location, error) {
	if c.apiKey == "" {
		return c.mockSearchLocations(query), nil
	}

	u := fmt.Sprintf(
		"%s/direct?q=%s&limit=%d&appid=%s",
		c.geocodingBase, url.QueryEscape(query), limit, c.apiKey,
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

	locations := make([]forecast.Location, len(results))
	for i, r := range results {
		locations[i] = forecast.Location{Name: r.Name, Country: r.Country, State: r.State, Lat: r.Lat, Lon: r.Lon}
	}
	return locations, nil
}

func (c *Client) ReverseGeocode(ctx context.Context, lat, lon float64) (forecast.Location, error) {
	if c.apiKey == "" {
		return forecast.Location{Name: "Unknown", Lat: lat, Lon: lon}, nil
	}

	u := fmt.Sprintf(
		"%s/reverse?lat=%f&lon=%f&limit=1&appid=%s",
		c.geocodingBase, lat, lon, c.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return forecast.Location{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return forecast.Location{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return forecast.Location{Name: "Unknown", Lat: lat, Lon: lon}, nil
		}
		return forecast.Location{}, fmt.Errorf("reverse geocoding API returned status %d", resp.StatusCode)
	}

	var results []struct {
		Name    string  `json:"name"`
		Country string  `json:"country"`
		State   string  `json:"state"`
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return forecast.Location{}, err
	}
	if len(results) == 0 {
		return forecast.Location{Name: "Unknown", Lat: lat, Lon: lon}, nil
	}

	r := results[0]
	return forecast.Location{Name: r.Name, Country: r.Country, State: r.State, Lat: r.Lat, Lon: r.Lon}, nil
}
