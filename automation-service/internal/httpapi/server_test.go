package httpapi

import (
	"encoding/json"
	"testing"
)

func TestValidateDefinition_DeviceState_MinimalOK(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{
				"id":   "t1",
				"kind": "trigger.device_state",
				"x":    0,
				"y":    0,
				"data": map[string]any{"device_id": "dev-1"},
			},
			{
				"id":   "a1",
				"kind": "action.send_command",
				"x":    100,
				"y":    0,
				"data": map[string]any{"device_id": "dev-1", "command": "set_state"},
			},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	out, err := validateDefinition(b, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected normalized definition bytes")
	}
}

func TestValidateDefinition_DeviceState_RequiresDeviceID(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{
				"id":   "t1",
				"kind": "trigger.device_state",
				"x":    0,
				"y":    0,
				"data": map[string]any{},
			},
			{
				"id":   "a1",
				"kind": "action.send_command",
				"x":    100,
				"y":    0,
				"data": map[string]any{"device_id": "dev-1", "command": "set_state"},
			},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	_, err := validateDefinition(b, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateDefinition_Schedule_RequiresCron(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{
				"id":   "t1",
				"kind": "trigger.schedule",
				"x":    0,
				"y":    0,
				"data": map[string]any{"cron": ""},
			},
			{
				"id":   "a1",
				"kind": "action.send_command",
				"x":    100,
				"y":    0,
				"data": map[string]any{"device_id": "dev-1", "command": "set_state"},
			},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	_, err := validateDefinition(b, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateDefinition_DefaultsCommand(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{
				"id":   "t1",
				"kind": "trigger.device_state",
				"x":    0,
				"y":    0,
				"data": map[string]any{"device_id": "dev-1"},
			},
			{
				"id":   "a1",
				"kind": "action.send_command",
				"x":    100,
				"y":    0,
				"data": map[string]any{"device_id": "dev-1", "command": ""},
			},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	out, err := validateDefinition(b, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected normalized definition bytes")
	}
}

func TestValidateDefinition_Manual_OK(t *testing.T) {
	payload := map[string]any{
		"version": "automation",
		"nodes": []map[string]any{
			{
				"id":   "t1",
				"kind": "trigger.manual",
				"x":    0,
				"y":    0,
				"data": map[string]any{},
			},
			{
				"id":   "a1",
				"kind": "action.send_command",
				"x":    100,
				"y":    0,
				"data": map[string]any{"device_id": "dev-1", "command": "set_state"},
			},
		},
		"edges": []map[string]any{{"from": "t1", "to": "a1"}},
	}
	b, _ := json.Marshal(payload)
	_, err := validateDefinition(b, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
