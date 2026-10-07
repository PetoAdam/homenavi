package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type httpDeviceClient struct {
	baseURL string
	client  *http.Client
}

func (c *httpDeviceClient) List(ctx context.Context) ([]device, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/hdp/devices", nil)
	if err != nil {
		return nil, fmt.Errorf("build device list request: %w", err)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request device list: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device hub returned %d", response.StatusCode)
	}
	var devices []device
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&devices); err != nil {
		return nil, fmt.Errorf("decode device list: %w", err)
	}
	if len(devices) > 100 {
		devices = devices[:100]
	}
	return devices, nil
}

func (c *httpDeviceClient) Command(ctx context.Context, deviceID string, state map[string]any, transitionMs *int, correlationID string) (any, error) {
	if deviceID == "" || len(state) == 0 || correlationID == "" {
		return nil, fmt.Errorf("device_id, state, and correlation_id are required")
	}
	body, err := json.Marshal(map[string]any{"state": state, "transition_ms": transitionMs, "correlation_id": correlationID})
	if err != nil {
		return nil, fmt.Errorf("encode device command: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/hdp/devices/"+deviceID+"/commands", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build device command: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("perform device command: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device hub returned %d", response.StatusCode)
	}
	var result any
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode device command response: %w", err)
	}
	return result, nil
}
