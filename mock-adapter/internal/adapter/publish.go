package adapter

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
)

func (s *Service) publishHello() {
	if s.adapterID == "" {
		return
	}
	payload := map[string]any{
		"schema":      hdp.SchemaV1,
		"type":        "hello",
		"adapter_id":  s.adapterID,
		"protocol":    "mock",
		"version":     s.version,
		"hdp_version": "1.0",
		"features": map[string]any{
			"supports_ack":         true,
			"supports_correlation": true,
			"supports_batch_state": false,
			"supports_pairing":     true,
			"supports_interview":   false,
		},
		"pairing": map[string]any{
			"schema_version":      "1.0",
			"label":               "Mock Adapter",
			"supported":           true,
			"supports_interview":  false,
			"default_timeout_sec": 60,
			"instructions": []string{
				"Use this adapter for integration and UI flow testing.",
				"Send a start command to trigger synthetic progress updates.",
				"No real Thread commissioning is performed.",
			},
			"cta_label": "Start Mock pairing",
			"notes":     "Placeholder implementation",
			"flow": map[string]any{
				"id":          "mock-default-flow",
				"entry_modes": []string{"default", "qr_code", "manual_code"},
				"forms": []map[string]any{
					{
						"mode":  "default",
						"label": "Default",
						"fields": []map[string]any{
							{"id": "timeout", "component": "number", "label": "Timeout (seconds)", "bind": "timeout", "min": 30, "max": 300, "default": 60},
							{"id": "room_hint", "component": "text", "label": "Room hint", "placeholder": "Living room"},
							{"id": "fast_mode", "component": "checkbox", "label": "Fast pairing", "default": true},
							{"id": "pairing_channel", "component": "select", "label": "Channel", "options": []map[string]any{{"value": "stable", "label": "Stable"}, {"value": "beta", "label": "Beta"}}, "default": "stable"},
							{"id": "capability_sets", "component": "multiselect", "label": "Capability presets", "multiple": true, "options": []map[string]any{{"value": "switch", "label": "Switch"}, {"value": "dimmer", "label": "Dimmer"}, {"value": "color", "label": "Color"}}},
							{"id": "profile", "component": "card_selector", "label": "Profile", "options": []map[string]any{{"value": "home", "label": "Home", "description": "Balanced profile"}, {"value": "lab", "label": "Lab", "description": "Verbose diagnostics"}}},
							{"id": "transport", "component": "list_selector", "label": "Transport", "multiple": true, "options": []map[string]any{{"value": "mqtt", "label": "MQTT"}, {"value": "http", "label": "HTTP"}}},
						},
					},
					{
						"mode":  "qr_code",
						"label": "QR Code",
						"fields": []map[string]any{
							{"id": "onboarding_payload", "component": "qr_payload", "label": "Onboarding payload", "required": true, "placeholder": "MT:..."},
							{"id": "scan_help", "component": "text_block", "label": "Use a phone to scan and paste payload if camera is unavailable."},
						},
					},
					{
						"mode":  "manual_code",
						"label": "Manual Code",
						"fields": []map[string]any{
							{"id": "manual_code", "component": "text", "label": "Manual setup code", "required": true},
							{"id": "discriminator", "component": "number", "label": "Discriminator", "min": 0, "max": 4095},
							{"id": "manual_loading", "component": "loading", "label": "Adapter validates manual code before start."},
						},
					},
				},
				"steps": []map[string]any{
					{"id": "commissioning", "label": "Commissioning", "stage": "commissioning"},
					{"id": "completed", "label": "Completed", "stage": "completed"},
				},
			},
		},
		"ts": time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.Publish(hdp.AdapterHelloTopic, b)
	}
}

func (s *Service) publishStatus(status, reason string) {
	if s.adapterID == "" {
		return
	}
	payload := map[string]any{
		"schema":     hdp.SchemaV1,
		"type":       "status",
		"adapter_id": s.adapterID,
		"protocol":   "mock",
		"status":     status,
		"reason":     reason,
		"version":    s.version,
		"features": map[string]any{
			"supports_pairing":   true,
			"supports_interview": false,
		},
		"pairing": map[string]any{
			"schema_version":      "1.0",
			"label":               "Mock Adapter",
			"supported":           true,
			"supports_interview":  false,
			"default_timeout_sec": 60,
			"cta_label":           "Start Mock pairing",
			"notes":               "Placeholder implementation",
			"flow": map[string]any{
				"id":          "mock-default-flow",
				"entry_modes": []string{"default", "qr_code", "manual_code"},
			},
		},
		"ts": time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.PublishWith(hdp.AdapterStatusPrefix+s.adapterID, b, true)
	}
}

func (s *Service) publishCommandResult(deviceID, corr string, success bool, status, errMsg string) {
	id := s.hdpDeviceID(deviceID)
	if id == "" || corr == "" {
		return
	}
	payload := map[string]any{
		"schema":    hdp.SchemaV1,
		"type":      "command_result",
		"origin":    "adapter",
		"device_id": id,
		"corr":      corr,
		"success":   success,
		"status":    status,
		"ts":        time.Now().UnixMilli(),
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.Publish(hdp.CommandResultPrefix+id, b)
	}
}

func (s *Service) publishPairingProgress(stage, status, external string, extra map[string]any) {
	if stage == "" {
		return
	}
	payload := map[string]any{
		"schema":   hdp.SchemaV1,
		"type":     "pairing_progress",
		"protocol": "mock",
		"stage":    stage,
		"status":   status,
		"ts":       time.Now().UnixMilli(),
	}
	if external != "" {
		payload["external_id"] = external
	}
	for key, value := range extra {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		payload[key] = value
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.Publish(hdp.PairingProgressPrefix+"mock", b)
	}
}
