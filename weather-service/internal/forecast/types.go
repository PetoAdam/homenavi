package forecast

// Location is a user-facing location search/reverse-geocode result.
type Location struct {
	Name    string  `json:"name"`
	Country string  `json:"country"`
	State   string  `json:"state,omitempty"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
}

// CurrentWeather is the current weather snapshot returned to the UI.
type CurrentWeather struct {
	TempC float64 `json:"temp_c"`
	HiC   float64 `json:"hi_c"`
	LoC   float64 `json:"lo_c"`
	Desc  string  `json:"desc"`
	Icon  string  `json:"icon"`
}

// ForecastItem is an hourly or daily forecast entry.
type ForecastItem struct {
	Hour  string  `json:"hour,omitempty"`
	Day   string  `json:"day,omitempty"`
	TempC float64 `json:"temp_c"`
	Icon  string  `json:"icon"`
}

// WeatherResponse is the API payload consumed by the frontend.
type WeatherResponse struct {
	City    string         `json:"city"`
	Current CurrentWeather `json:"current"`
	Daily   []ForecastItem `json:"daily"`
	Weekly  []ForecastItem `json:"weekly"`
}

// WeatherRequest captures the supported location inputs for weather lookup.
type WeatherRequest struct {
	City      string
	Latitude  *float64
	Longitude *float64
}
