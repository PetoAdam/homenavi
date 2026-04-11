package openweather

import (
	"net/http"
	"time"
)

// Client talks to the OpenWeather APIs and falls back to mock data when no key is configured.
type Client struct {
	apiKey        string
	httpClient    *http.Client
	geocodingBase string
	weatherBase   string
}

func New(apiKey string) *Client {
	return newClient(apiKey, nil, "https://api.openweathermap.org/geo/1.0", "https://api.openweathermap.org/data/2.5")
}

func newClient(apiKey string, httpClient *http.Client, geocodingBase, weatherBase string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		apiKey:        apiKey,
		httpClient:    httpClient,
		geocodingBase: geocodingBase,
		weatherBase:   weatherBase,
	}
}
