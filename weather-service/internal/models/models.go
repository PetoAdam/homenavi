package models

type Location struct {
	Name    string  `json:"name"`
	Country string  `json:"country"`
	State   string  `json:"state,omitempty"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
}

type CurrentWeather struct {
	TempC float64 `json:"temp_c"`
	HiC   float64 `json:"hi_c"`
	LoC   float64 `json:"lo_c"`
	Desc  string  `json:"desc"`
	Icon  string  `json:"icon"`
}

type ForecastItem struct {
	Hour  string  `json:"hour,omitempty"`
	Day   string  `json:"day,omitempty"`
	TempC float64 `json:"temp_c"`
	Icon  string  `json:"icon"`
}

type WeatherResponse struct {
	City    string         `json:"city"`
	Current CurrentWeather `json:"current"`
	Daily   []ForecastItem `json:"daily"`
	Weekly  []ForecastItem `json:"weekly"`
}
