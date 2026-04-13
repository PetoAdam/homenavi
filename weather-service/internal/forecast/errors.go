package forecast

import "errors"

var (
	ErrQueryRequired    = errors.New("query parameter 'q' is required")
	ErrLocationRequired = errors.New("location is required")
	ErrCityNotFound     = errors.New("city not found")
)
