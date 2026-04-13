package openweather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/PetoAdam/homenavi/weather-service/internal/forecast"
)

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

var _ forecast.Provider = (*Client)(nil)
