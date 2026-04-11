package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type integrationStepRecord struct {
	IntegrationID string         `json:"integration_id"`
	Scope         string         `json:"scope"`
	Step          map[string]any `json:"step"`
}

type integrationStepsCatalog struct {
	GeneratedAt string                  `json:"generated_at"`
	Actions     []integrationStepRecord `json:"actions"`
	Triggers    []integrationStepRecord `json:"triggers"`
	Conditions  []integrationStepRecord `json:"conditions"`
}

func (s *Server) handleIntegrationSteps(w http.ResponseWriter, r *http.Request) {
	if s.integrationProxyURL == "" {
		writeError(w, http.StatusInternalServerError, "integration proxy url not configured")
		return
	}

	url := s.integrationProxyURL + "/integrations/automation-steps.json"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build integration steps request")
		return
	}
	if token := getAuthToken(r); strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch integration steps")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = "integration steps upstream error"
		}
		writeError(w, http.StatusBadGateway, msg)
		return
	}

	var payload integrationStepsCatalog
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadGateway, "invalid integration steps response")
		return
	}
	filterScope := func(items []integrationStepRecord) []integrationStepRecord {
		out := make([]integrationStepRecord, 0, len(items))
		for _, item := range items {
			if strings.ToLower(strings.TrimSpace(item.Scope)) != "integration_only" {
				continue
			}
			out = append(out, item)
		}
		return out
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at": payload.GeneratedAt,
		"actions":      filterScope(payload.Actions),
		"triggers":     filterScope(payload.Triggers),
		"conditions":   filterScope(payload.Conditions),
	})
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	catalog := []map[string]any{
		{"kind": "trigger.manual", "label": "Manual", "fields": []map[string]any{}},
		{"kind": "trigger.device_state", "label": "Device state", "fields": []map[string]any{
			{"name": "targets", "type": "json", "required": true, "help": "Targets: {type:'device', ids:['zigbee/...']} or {type:'selector', selector:'tag:kitchen'}"},
			{"name": "key", "type": "string", "required": false, "help": "State key (e.g. motion, temperature). Empty matches any state frame."},
			{"name": "op", "type": "string", "required": false, "enum": []string{"exists", "eq", "neq", "gt", "gte", "lt", "lte"}},
			{"name": "value", "type": "json", "required": false},
			{"name": "ignore_retained", "type": "bool", "required": false, "default": true},
			{"name": "cooldown_sec", "type": "int", "required": false, "default": 2},
		}},
		{"kind": "trigger.schedule", "label": "Schedule (cron)", "fields": []map[string]any{
			{"name": "cron", "type": "string", "required": true, "help": "Cron with seconds: e.g. '0 */5 * * * *' (every 5 minutes)"},
			{"name": "cooldown_sec", "type": "int", "required": false, "default": 1},
		}},
		{"kind": "logic.sleep", "label": "Sleep", "fields": []map[string]any{{"name": "duration_sec", "type": "int", "required": true, "default": 5}}},
		{"kind": "logic.if", "label": "If", "fields": []map[string]any{
			{"name": "path", "type": "string", "required": true, "help": "Dot-path in trigger event, e.g. state.motion"},
			{"name": "op", "type": "string", "required": false, "enum": []string{"exists", "eq", "neq", "gt", "gte", "lt", "lte"}},
			{"name": "value", "type": "json", "required": false},
		}},
		{"kind": "logic.for", "label": "For loop", "fields": []map[string]any{{"name": "count", "type": "int", "required": true, "default": 3}}},
		{"kind": "action.send_command", "label": "Send device command", "fields": []map[string]any{
			{"name": "targets", "type": "json", "required": true, "help": "Targets: {type:'device', ids:['zigbee/...']} or {type:'selector', selector:'tag:kitchen'}"},
			{"name": "command", "type": "string", "required": true, "default": "set_state"},
			{"name": "args", "type": "json", "required": false, "help": "Command args map. For set_state: {state:'ON', brightness:80}."},
			{"name": "wait_for_result", "type": "bool", "required": false, "default": false},
			{"name": "result_timeout_sec", "type": "int", "required": false, "default": 15},
		}},
		{"kind": "action.notify_email", "label": "Notify email", "fields": []map[string]any{
			{"name": "user_ids", "type": "json", "required": true, "help": "Array of user IDs."},
			{"name": "subject", "type": "string", "required": true},
			{"name": "message", "type": "string", "required": true},
		}},
		{"kind": "action.integration", "label": "Integration action", "fields": []map[string]any{
			{"name": "integration_id", "type": "string", "required": true, "help": "Integration ID from integration-proxy registry."},
			{"name": "action_id", "type": "string", "required": true, "help": "Action kind from integration automation catalog."},
			{"name": "input", "type": "json", "required": false, "help": "Action input payload."},
			{"name": "timeout_sec", "type": "int", "required": false, "default": 15},
		}},
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": catalog, "version": "automation"})
}
