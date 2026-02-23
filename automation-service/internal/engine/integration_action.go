package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type integrationRegistryPayload struct {
	Integrations []struct {
		ID                  string `json:"id"`
		AutomationExtension *struct {
			ExecuteEndpoint string `json:"execute_endpoint"`
		} `json:"automation_extension,omitempty"`
	} `json:"integrations"`
}

var safeIntegrationID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}[a-z0-9]$`)

func (e *Engine) executeIntegrationAction(ctx context.Context, runID uuid.UUID, nodeID string, action ActionIntegration) (map[string]any, error) {
	integrationID := strings.TrimSpace(action.IntegrationID)
	if integrationID == "" {
		return nil, errors.New("integration_id is required")
	}
	actionID := strings.TrimSpace(action.ActionID)
	if actionID == "" {
		return nil, errors.New("action_id is required")
	}

	timeoutSec := action.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 15
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	execPath := "/api/automation/execute"
	if strings.TrimSpace(e.integrationProxyURL) != "" {
		if resolved, err := e.resolveIntegrationExecuteEndpoint(callCtx, integrationID); err == nil {
			execPath = resolved
		}
	}

	body, _ := json.Marshal(map[string]any{
		"action_id": actionID,
		"input":     action.Input,
		"run_id":    runID.String(),
		"node_id":   nodeID,
	})

	doCall := func(urlStr string) (*http.Response, []byte, error) {
		req, err := http.NewRequestWithContext(callCtx, http.MethodPost, urlStr, bytes.NewReader(body))
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := e.httpClient.Do(req)
		if err != nil {
			return nil, nil, err
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return resp, raw, nil
	}

	// Automations should not depend on browser/user auth. Execute integration actions by calling
	// the integration container directly on the internal network.
	if !safeIntegrationID.MatchString(integrationID) {
		return nil, errors.New("invalid integration_id")
	}
	if !strings.HasPrefix(execPath, "/") {
		execPath = "/" + execPath
	}
	directURL := "http://" + integrationID + ":8099" + execPath
	resp, raw, err := doCall(directURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("integration action request failed: %s", msg)
	}

	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{"success": true}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, errors.New("integration action response must be valid json")
	}
	if success, ok := payload["success"].(bool); ok && !success {
		if errMsg, ok := payload["error"].(string); ok && strings.TrimSpace(errMsg) != "" {
			return nil, errors.New(strings.TrimSpace(errMsg))
		}
		return nil, errors.New("integration action failed")
	}
	return payload, nil
}

func (e *Engine) resolveIntegrationExecuteEndpoint(ctx context.Context, integrationID string) (string, error) {
	if strings.TrimSpace(e.integrationProxyURL) == "" {
		return "", errors.New("integration proxy url not configured")
	}
	url := strings.TrimRight(e.integrationProxyURL, "/") + "/integrations/registry.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("registry request failed: %s", resp.Status)
	}

	var payload integrationRegistryPayload
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", errors.New("invalid registry response")
	}

	for _, it := range payload.Integrations {
		if strings.TrimSpace(it.ID) != integrationID || it.AutomationExtension == nil {
			continue
		}
		path := strings.TrimSpace(it.AutomationExtension.ExecuteEndpoint)
		if path == "" {
			return "/api/automation/execute", nil
		}
		if strings.Contains(path, "://") {
			return "", errors.New("invalid execute_endpoint")
		}
		if len(path) > 256 || strings.ContainsAny(path, " \t\r\n") {
			return "", errors.New("invalid execute_endpoint")
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return path, nil
	}

	return "", fmt.Errorf("integration %s not found in registry", integrationID)
}
