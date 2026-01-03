package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/homenavi/assistant-service/internal/config"
	"github.com/homenavi/assistant-service/internal/tools"
)

func TestRegistry_FilterByRole(t *testing.T) {
	cfg := &config.Config{
		DeviceHubURL:  "http://localhost:8090",
		ERSURL:        "http://localhost:8095",
		AutomationURL: "http://localhost:8094",
		HistoryURL:    "http://localhost:8093",
	}
	registry := tools.NewRegistry(cfg)

	tests := []struct {
		name     string
		role     string
		minTools int // Minimum number of tools expected
	}{
		{
			name:     "user role has limited tools",
			role:     "user",
			minTools: 1, // At least get_current_time
		},
		{
			name:     "resident role has more tools",
			role:     "resident",
			minTools: 4, // Device control, list, etc.
		},
		{
			name:     "admin role has all tools",
			role:     "admin",
			minTools: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := registry.FilterByRole(tt.role)
			if len(filtered) < tt.minTools {
				t.Errorf("FilterByRole(%s) returned %d tools, want at least %d", tt.role, len(filtered), tt.minTools)
			}
		})
	}
}

func TestRegistry_CanUse(t *testing.T) {
	cfg := &config.Config{
		DeviceHubURL:  "http://localhost:8090",
		ERSURL:        "http://localhost:8095",
		AutomationURL: "http://localhost:8094",
		HistoryURL:    "http://localhost:8093",
	}
	registry := tools.NewRegistry(cfg)

	tests := []struct {
		name     string
		toolName string
		role     string
		want     bool
	}{
		{
			name:     "user can use get_current_time",
			toolName: "get_current_time",
			role:     "user",
			want:     true,
		},
		{
			name:     "user cannot use control_device",
			toolName: "control_device",
			role:     "user",
			want:     false,
		},
		{
			name:     "resident can use control_device",
			toolName: "control_device",
			role:     "resident",
			want:     true,
		},
		{
			name:     "admin can use all tools",
			toolName: "list_devices",
			role:     "admin",
			want:     true,
		},
		{
			name:     "unknown tool returns false",
			toolName: "unknown_tool",
			role:     "admin",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.CanUse(tt.toolName, tt.role); got != tt.want {
				t.Errorf("CanUse(%s, %s) = %v, want %v", tt.toolName, tt.role, got, tt.want)
			}
		})
	}
}

func TestRegistry_Execute_GetCurrentTime(t *testing.T) {
	cfg := &config.Config{}
	registry := tools.NewRegistry(cfg)

	ctx := context.Background()
	result, err := registry.Execute(ctx, "get_current_time", nil, "user", "user-123")

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Execute() success = false, want true")
	}

	data, ok := result.Data.(map[string]string)
	if !ok {
		t.Fatalf("Execute() data is not map[string]string")
	}

	if data["time"] == "" {
		t.Error("Execute() time is empty")
	}
	if data["date"] == "" {
		t.Error("Execute() date is empty")
	}
	if data["weekday"] == "" {
		t.Error("Execute() weekday is empty")
	}
}

func TestRegistry_Execute_InsufficientRole(t *testing.T) {
	cfg := &config.Config{}
	registry := tools.NewRegistry(cfg)

	ctx := context.Background()
	_, err := registry.Execute(ctx, "control_device", nil, "user", "user-123")

	if err == nil {
		t.Fatalf("Execute() expected error")
	}
	if !errors.Is(err, tools.ErrInsufficientRole) {
		t.Errorf("Execute() error = %v, want ErrInsufficientRole", err)
	}
}

func TestRegistry_Execute_UnknownTool(t *testing.T) {
	cfg := &config.Config{}
	registry := tools.NewRegistry(cfg)

	ctx := context.Background()
	_, err := registry.Execute(ctx, "unknown_tool", nil, "admin", "user-123")

	if err == nil {
		t.Error("Execute() expected error for unknown tool")
	}
}

func TestRegistry_Execute_ListRooms(t *testing.T) {
	// Mock ERS server
	ers := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ers/rooms" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "room-1", "name": "Bedroom", "slug": "bedroom"},
			{"id": "room-2", "name": "Living Room", "slug": "living-room"},
		})
	}))
	defer ers.Close()

	cfg := &config.Config{ERSURL: ers.URL}
	registry := tools.NewRegistry(cfg)

	ctx := context.Background()
	result, err := registry.Execute(ctx, "list_rooms", map[string]interface{}{}, "resident", "user-123")
	if err != nil {
		t.Fatalf("Execute(list_rooms) error = %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("Execute(list_rooms) success = false, result=%v", result)
	}

	rooms, ok := result.Data.([]map[string]interface{})
	if !ok {
		// json.Unmarshal into []map[string]interface{} typically yields that type in the tool.
		// But in case it crosses interface boundaries, accept []interface{} and validate minimally.
		roomsAny, ok2 := result.Data.([]interface{})
		if !ok2 || len(roomsAny) != 2 {
			t.Fatalf("Execute(list_rooms) unexpected data type: %T", result.Data)
		}
		return
	}
	if len(rooms) != 2 {
		t.Fatalf("Execute(list_rooms) rooms = %d, want 2", len(rooms))
	}
}

func TestRegistry_Execute_FindDevices(t *testing.T) {
	// Mock ERS + Device Hub
	ers := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ers/rooms":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "room-1", "name": "Bedroom", "slug": "bedroom"},
			})
		case "/api/ers/devices":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "ers-1", "name": "RGB Lamp", "room_id": "room-1", "hdp_external_ids": []string{"zigbee/0x1"}},
				{"id": "ers-2", "name": "Temp Sensor", "room_id": "room-1", "hdp_external_ids": []string{"zigbee/0x2"}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ers.Close()

	hdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hdp/devices" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"device_id":    "zigbee/0x1",
				"description":  "Lamp",
				"type":         "light",
				"manufacturer": "Acme",
				"model":        "RGB-1",
				"online":       true,
				"state":        map[string]interface{}{"state": "OFF"},
				"capabilities": []interface{}{},
				"inputs":       []interface{}{},
			},
			{
				"device_id":    "zigbee/0x2",
				"description":  "Sensor",
				"type":         "sensor",
				"manufacturer": "Acme",
				"model":        "T-1",
				"online":       true,
				"state":        map[string]interface{}{"temperature": 21.5},
				"capabilities": []interface{}{},
				"inputs":       []interface{}{},
			},
		})
	}))
	defer hdp.Close()

	cfg := &config.Config{ERSURL: ers.URL, DeviceHubURL: hdp.URL}
	registry := tools.NewRegistry(cfg)

	ctx := context.Background()
	res, err := registry.Execute(ctx, "find_devices", map[string]interface{}{"query": "rgb"}, "resident", "user-123")
	if err != nil {
		t.Fatalf("Execute(find_devices) error = %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("Execute(find_devices) success=false, res=%v", res)
	}
	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Execute(find_devices) unexpected data type: %T", res.Data)
	}
	var matches []map[string]interface{}
	switch v := data["matches"].(type) {
	case []map[string]interface{}:
		matches = v
	case []interface{}:
		for _, it := range v {
			m, _ := it.(map[string]interface{})
			if m != nil {
				matches = append(matches, m)
			}
		}
	default:
		t.Fatalf("Execute(find_devices) unexpected matches type: %T", data["matches"])
	}
	if len(matches) == 0 {
		t.Fatalf("Execute(find_devices) matches empty: %v", data["matches"])
	}
	first := matches[0]
	if first == nil {
		t.Fatalf("Execute(find_devices) match type unexpected")
	}
	if first["device_id"] != "zigbee/0x1" {
		t.Fatalf("Execute(find_devices) first device_id=%v, want zigbee/0x1", first["device_id"])
	}
}

func TestRegistry_ControlDevice_CapabilityIDValue(t *testing.T) {
	var gotPayload map[string]interface{}

	hdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hdp/devices/zigbee/0x1/commands" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		dec := json.NewDecoder(r.Body)
		_ = dec.Decode(&gotPayload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer hdp.Close()

	cfg := &config.Config{DeviceHubURL: hdp.URL}
	registry := tools.NewRegistry(cfg)

	ctx := context.Background()
	res, err := registry.Execute(ctx, "control_device", map[string]interface{}{
		"device_id":     "zigbee/0x1",
		"capability_id": "brightness",
		"value":         10,
		"transition_ms": 0,
	}, "resident", "user-123")
	if err != nil {
		t.Fatalf("Execute(control_device) error = %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("Execute(control_device) success=false, res=%v", res)
	}
	st, _ := gotPayload["state"].(map[string]interface{})
	if st == nil {
		t.Fatalf("control_device payload missing state: %v", gotPayload)
	}
	if _, ok := st["brightness"]; !ok {
		t.Fatalf("control_device payload missing brightness: %v", gotPayload)
	}
}

func TestRegistry_GetToolsForPrompt(t *testing.T) {
	cfg := &config.Config{}
	registry := tools.NewRegistry(cfg)

	prompt := registry.GetToolsForPrompt("resident")

	if prompt == "" {
		t.Error("GetToolsForPrompt() returned empty string")
	}

	// Should contain some tool names
	if !containsSubstring(prompt, "list_devices") {
		t.Error("GetToolsForPrompt() should contain list_devices")
	}
	if !containsSubstring(prompt, "control_device") {
		t.Error("GetToolsForPrompt() should contain control_device")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
