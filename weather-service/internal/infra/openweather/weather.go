package openweather

import (
	"context"
	"fmt"
	"time"

	"github.com/PetoAdam/homenavi/weather-service/internal/forecast"
)

func (c *Client) GetWeather(ctx context.Context, lat, lon float64, city string) (forecast.WeatherResponse, error) {
	if c.apiKey == "" {
		return c.mockWeather(city), nil
	}

	currentURL := fmt.Sprintf(
		"%s/weather?lat=%f&lon=%f&units=metric&appid=%s",
		c.weatherBase, lat, lon, c.apiKey,
	)
	forecastURL := fmt.Sprintf(
		"%s/forecast?lat=%f&lon=%f&units=metric&appid=%s",
		c.weatherBase, lat, lon, c.apiKey,
	)
	oneCallURL := fmt.Sprintf(
		"%s/onecall?lat=%f&lon=%f&units=metric&exclude=minutely,alerts&appid=%s",
		c.weatherBase, lat, lon, c.apiKey,
	)

	currentResp, err := c.fetchJSON(ctx, currentURL)
	if err != nil {
		if isAuthFailure(err) {
			return c.mockWeather(city), nil
		}
		return forecast.WeatherResponse{}, fmt.Errorf("fetching current weather: %w", err)
	}
	forecastResp, err := c.fetchJSON(ctx, forecastURL)
	if err != nil {
		if isAuthFailure(err) {
			return c.mockWeather(city), nil
		}
		return forecast.WeatherResponse{}, fmt.Errorf("fetching forecast: %w", err)
	}

	main := getMap(currentResp, "main")
	weather := getFirstInArray(currentResp, "weather")
	current := forecast.CurrentWeather{
		TempC: getFloat(main, "temp"),
		HiC:   getFloat(main, "temp_max"),
		LoC:   getFloat(main, "temp_min"),
		Desc:  getString(weather, "description"),
		Icon:  mapOWMIcon(getString(weather, "icon")),
	}

	now := time.Now()
	if oneCallResp, err := c.fetchJSON(ctx, oneCallURL); err == nil {
		hourly := getArray(oneCallResp, "hourly")
		dailySrc := getArray(oneCallResp, "daily")
		daily := make([]forecast.ForecastItem, 0, 24)
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
			daily = append(daily, forecast.ForecastItem{
				Hour:  fmt.Sprintf("%02d", t.Hour()),
				TempC: getFloat(itemMap, "temp"),
				Icon:  mapOWMIcon(getString(itemWeather, "icon")),
			})
			if len(daily) >= 24 {
				break
			}
		}

		weekly := make([]forecast.ForecastItem, 0, 7)
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
			weekly = append(weekly, forecast.ForecastItem{
				Day:   t.Weekday().String()[:3],
				TempC: getFloat(tempMap, "day"),
				Icon:  mapOWMIcon(getString(itemWeather, "icon")),
			})
			if len(weekly) >= 7 {
				break
			}
		}

		if len(daily) > 0 && len(weekly) > 0 {
			return forecast.WeatherResponse{City: city, Current: current, Daily: daily, Weekly: weekly}, nil
		}
	}

	list := getArray(forecastResp, "list")
	daily := make([]forecast.ForecastItem, 0, 24)
	weekly := make([]forecast.ForecastItem, 0, 7)
	seenDays := make(map[string]bool)

	for _, item := range list {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		dt := int64(getFloat(itemMap, "dt"))
		t := time.Unix(dt, 0)
		itemMain := getMap(itemMap, "main")
		itemWeather := getFirstInArray(itemMap, "weather")
		if !t.Before(now) && len(daily) < 24 {
			tempC := getFloat(itemMain, "temp")
			icon := mapOWMIcon(getString(itemWeather, "icon"))
			for k := 0; k < 3 && len(daily) < 24; k++ {
				tt := t.Add(time.Duration(k) * time.Hour)
				daily = append(daily, forecast.ForecastItem{Hour: fmt.Sprintf("%02d", tt.Hour()), TempC: tempC, Icon: icon})
			}
		}

		dayKey := t.Format("2006-01-02")
		if !seenDays[dayKey] && t.Hour() >= 11 && t.Hour() <= 14 {
			seenDays[dayKey] = true
			weekly = append(weekly, forecast.ForecastItem{Day: t.Weekday().String()[:3], TempC: getFloat(itemMain, "temp"), Icon: mapOWMIcon(getString(itemWeather, "icon"))})
			if len(weekly) >= 7 {
				break
			}
		}
	}

	if len(weekly) > 0 && len(weekly) < 7 {
		last := weekly[len(weekly)-1]
		for i := len(weekly); i < 7; i++ {
			t := now.AddDate(0, 0, i)
			weekly = append(weekly, forecast.ForecastItem{Day: t.Weekday().String()[:3], TempC: last.TempC, Icon: last.Icon})
		}
	}

	return forecast.WeatherResponse{City: city, Current: current, Daily: daily, Weekly: weekly}, nil
}
