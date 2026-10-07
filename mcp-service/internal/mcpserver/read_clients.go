package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type readClients struct {
	devices           *httpDeviceClient
	historyURL        string
	entityRegistryURL string
	automationURL     string
	authServiceURL    string
	client            *http.Client
}

type deviceGroup struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Slug           string   `json:"slug"`
	Description    string   `json:"description"`
	HDPExternalIDs []string `json:"hdp_external_ids"`
}

func (c *readClients) ListGroups(ctx context.Context) (any, error) {
	return c.getAny(ctx, c.entityRegistryURL, "/api/ers/groups/", nil)
}

func (c *readClients) GetGroup(ctx context.Context, groupID string) (deviceGroup, error) {
	if strings.TrimSpace(groupID) == "" {
		return deviceGroup{}, fmt.Errorf("group_id is required")
	}
	var result deviceGroup
	if err := c.get(ctx, c.entityRegistryURL, "/api/ers/groups/"+url.PathEscape(groupID), nil, &result); err != nil {
		return deviceGroup{}, err
	}
	return result, nil
}

func (c *readClients) ListAutomationWorkflows(ctx context.Context, delegatedToken string) (any, error) {
	return c.getAuthorizedAny(ctx, c.automationURL, "/api/automation/workflows", delegatedToken)
}

func (c *readClients) GetAutomationWorkflow(ctx context.Context, workflowID, delegatedToken string) (any, error) {
	if strings.TrimSpace(workflowID) == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}
	return c.getAuthorizedAny(ctx, c.automationURL, "/api/automation/workflows/"+url.PathEscape(workflowID), delegatedToken)
}

func (c *readClients) RunAutomationWorkflow(ctx context.Context, workflowID, delegatedToken, idempotencyKey string) (any, error) {
	if strings.TrimSpace(workflowID) == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}
	return c.postAuthorizedAny(ctx, c.automationURL, "/api/automation/workflows/"+url.PathEscape(workflowID)+"/run", delegatedToken, nil, idempotencyKey)
}

func (c *readClients) CreateGroup(ctx context.Context, input groupCreateInput, idempotencyKey string) (any, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	return c.postAuthorizedAny(ctx, c.entityRegistryURL, "/api/ers/groups/", "", map[string]any{"name": input.Name, "description": input.Description, "device_ids": input.DeviceIDs}, idempotencyKey)
}

func (c *readClients) exchangeAutomationToken(ctx context.Context, subjectToken, resource, scope string) (string, error) {
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:token-exchange"}, "resource": {resource}, "scope": {scope}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.authServiceURL, "/")+"/api/auth/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token exchange: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+subjectToken)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("perform token exchange: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange returned %d", response.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(&payload); err != nil || payload.AccessToken == "" {
		return "", fmt.Errorf("decode token exchange response")
	}
	return payload.AccessToken, nil
}

func (c *readClients) GetDevice(ctx context.Context, deviceID string) (device, error) {
	var result device
	err := c.get(ctx, c.devices.baseURL, "/api/hdp/devices/"+url.PathEscape(strings.TrimSpace(deviceID)), nil, &result)
	return result, err
}

func (c *readClients) ListIntegrations(ctx context.Context) (any, error) {
	return c.getAny(ctx, c.devices.baseURL, "/api/hdp/integrations", nil)
}

func (c *readClients) ListPairings(ctx context.Context) (any, error) {
	return c.getAny(ctx, c.devices.baseURL, "/api/hdp/pairings", nil)
}

func (c *readClients) ListRooms(ctx context.Context) (any, error) {
	return c.getAny(ctx, c.entityRegistryURL, "/api/ers/rooms/", nil)
}

func (c *readClients) QueryStateHistory(ctx context.Context, input historyInput) (any, error) {
	if strings.TrimSpace(input.DeviceID) == "" {
		return nil, fmt.Errorf("device_id is required")
	}
	if input.Limit < 0 || input.Limit > 100 {
		return nil, fmt.Errorf("limit must be between 0 and 100")
	}
	query := url.Values{"device_id": {strings.TrimSpace(input.DeviceID)}}
	if input.From != "" {
		query.Set("from", input.From)
	}
	if input.To != "" {
		query.Set("to", input.To)
	}
	if input.Limit > 0 {
		query.Set("limit", fmt.Sprint(input.Limit))
	}
	if input.Order == "asc" || input.Order == "desc" {
		query.Set("order", input.Order)
	} else if input.Order != "" {
		return nil, fmt.Errorf("order must be asc or desc")
	}
	return c.getAny(ctx, c.historyURL, "/api/history/state", query)
}

func (c *readClients) getAny(ctx context.Context, baseURL, endpoint string, query url.Values) (any, error) {
	var result any
	if err := c.get(ctx, baseURL, endpoint, query, &result); err != nil {
		return nil, err
	}
	return redact(result), nil
}

func (c *readClients) getAuthorizedAny(ctx context.Context, baseURL, endpoint, bearer string) (any, error) {
	var result any
	if err := c.getAuthorized(ctx, baseURL, endpoint, bearer, &result); err != nil {
		return nil, err
	}
	return redact(result), nil
}

func (c *readClients) get(ctx context.Context, baseURL, endpoint string, query url.Values, output any) error {
	requestURL, err := url.Parse(strings.TrimRight(baseURL, "/") + endpoint)
	if err != nil {
		return fmt.Errorf("build read request: %w", err)
	}
	requestURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("build read request: %w", err)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("perform read request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("read service returned %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(output); err != nil {
		return fmt.Errorf("decode read response: %w", err)
	}
	return nil
}

func (c *readClients) getAuthorized(ctx context.Context, baseURL, endpoint, bearer string, output any) error {
	requestURL := strings.TrimRight(baseURL, "/") + endpoint
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("build read request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+bearer)
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("perform read request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("read service returned %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(output); err != nil {
		return fmt.Errorf("decode read response: %w", err)
	}
	return nil
}

func (c *readClients) postAuthorizedAny(ctx context.Context, baseURL, endpoint, bearer string, payload any, idempotencyKey string) (any, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode write request: %w", err)
		}
		body = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("build write request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("perform write request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("write service returned %d", response.StatusCode)
	}
	var result any
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode write response: %w", err)
	}
	return redact(result), nil
}

func redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") {
				continue
			}
			result[key] = redact(item)
		}
		return result
	case []any:
		if len(typed) > 100 {
			typed = typed[:100]
		}
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redact(item)
		}
		return result
	default:
		return value
	}
}
