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
	readBaseURL    string
	commandReadURL string
	commandBaseURL string
	client         *http.Client
}

func (c *httpDeviceClient) List(ctx context.Context, bearer string) ([]device, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.readBaseURL+"/devices", nil)
	if err != nil {
		return nil, fmt.Errorf("build device list request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+bearer)
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

func (c *httpDeviceClient) Command(ctx context.Context, deviceID string, state map[string]any, transitionMs *int, correlationID, bearer string) (any, error) {
	if deviceID == "" || len(state) == 0 || correlationID == "" {
		return nil, fmt.Errorf("device_id, state, and correlation_id are required")
	}
	body, err := json.Marshal(map[string]any{"state": state, "transition_ms": transitionMs, "correlation_id": correlationID})
	if err != nil {
		return nil, fmt.Errorf("encode device command: %w", err)
	}
	commandURL := strings.TrimRight(c.commandBaseURL, "/") + "/" + deviceID + "/commands"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, commandURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build device command: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+bearer)
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
