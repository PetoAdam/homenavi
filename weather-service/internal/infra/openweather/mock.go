package openweather

import (
	"fmt"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/weather-service/internal/forecast"
)

func (c *Client) mockSearchLocations(query string) []forecast.Location {
	cities := []forecast.Location{
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
	var matches []forecast.Location
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

func (c *Client) mockWeather(city string) forecast.WeatherResponse {
	now := time.Now()
	daily := make([]forecast.ForecastItem, 0, 24)
	for i := 0; i < 24; i++ {
		t := now.Add(time.Duration(i) * time.Hour)
		icon := "cloud"
		if i%6 == 0 {
			icon = "sun"
		} else if i%6 == 3 {
			icon = "cloud_sun"
		}
		daily = append(daily, forecast.ForecastItem{Hour: fmt.Sprintf("%02d", t.Hour()), TempC: 18 + float64((i%8)-3), Icon: icon})
	}

	weekly := make([]forecast.ForecastItem, 0, 7)
	for i := 0; i < 7; i++ {
		t := now.AddDate(0, 0, i)
		icon := "cloud"
		if i%3 == 0 {
			icon = "sun"
		} else if i%3 == 1 {
			icon = "cloud_sun"
		}
		weekly = append(weekly, forecast.ForecastItem{Day: t.Weekday().String()[:3], TempC: 20 + float64((i%5)-2), Icon: icon})
	}

	return forecast.WeatherResponse{
		City: city,
		Current: forecast.CurrentWeather{
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
