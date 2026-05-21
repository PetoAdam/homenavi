package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	model "github.com/PetoAdam/homenavi/device-hub/internal/devices"
	"gorm.io/datatypes"
)

func TestHandlePairingConfigReturnsConfigs(t *testing.T) {
	srv := NewServer(nil, nil)
	srv.adapters.upsertFromHello([]byte(`{
		"schema":"hdp.v1",
		"type":"hello",
		"adapter_id":"zigbee",
		"protocol":"zigbee",
		"version":"test",
		"features": {"supports_pairing": true, "supports_interview": true},
		"pairing": {
                  "schema_version": "1.0",
		  "label":"Zigbee",
		  "supported": true,
		  "supports_interview": true,
		  "default_timeout_sec": 60,
		  "instructions": ["a","b"],
		  "cta_label": "Start Zigbee pairing",
                  "flow": {"entry_modes": ["default"]}
		},
		"ts": 1
	}`))
	srv.adapters.upsertFromHello([]byte(`{
		"schema":"hdp.v1",
		"type":"hello",
		"adapter_id":"mock",
		"protocol":"mock",
		"version":"test",
		"features": {"supports_pairing": true, "supports_interview": false},
		"pairing": {
                  "schema_version": "1.0",
		  "label":"Mock Adapter",
		  "supported": false,
		  "supports_interview": false,
		  "default_timeout_sec": 30,
		  "notes": "placeholder"
		},
		"ts": 1
	}`))

	req := httptest.NewRequest(http.MethodGet, "/api/hdp/pairing-config", nil)
	rr := httptest.NewRecorder()

	srv.handlePairingConfig(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", status, http.StatusOK)
	}

	var got []PairingConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid json: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("unexpected config count: got %d want %d", len(got), 2)
	}

	byProto := map[string]PairingConfig{}
	for _, cfg := range got {
		byProto[cfg.Protocol] = cfg
	}
	if _, ok := byProto["zigbee"]; !ok {
		t.Fatalf("expected zigbee config present: %+v", got)
	}
	if _, ok := byProto["mock"]; !ok {
		t.Fatalf("expected mock config present: %+v", got)
	}
	if byProto["zigbee"].SchemaVersion != "1.0" {
		t.Fatalf("expected zigbee schema version 1.0, got %q", byProto["zigbee"].SchemaVersion)
	}
	if byProto["zigbee"].Flow == nil {
		t.Fatalf("expected zigbee flow in pairing config")
	}
}

func TestHandlePairingConfigEmpty(t *testing.T) {
	srv := NewServer(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/hdp/pairing-config", nil)
	rr := httptest.NewRecorder()

	srv.handlePairingConfig(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", status, http.StatusOK)
	}

	var got []any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid json: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty array, got %d items", len(got))
	}
}

func TestHandleDeviceCommandInvalidJSON(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/abc/commands", strings.NewReader("{not-json"))
	rr := httptest.NewRecorder()

	srv.handleDeviceCommand(rr, req, "abc")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "invalid json") {
		t.Fatalf("expected invalid json message, got %q", rr.Body.String())
	}
}

func TestHandleDeviceCommandMissingState(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/abc/commands", strings.NewReader(`{"state":{},"input":null}`))
	rr := httptest.NewRecorder()

	srv.handleDeviceCommand(rr, req, "abc")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "state is required") {
		t.Fatalf("expected missing state message, got %q", rr.Body.String())
	}
}

func TestHandleDeviceCreateInvalidJSON(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices", strings.NewReader("{bad"))
	rr := httptest.NewRecorder()

	srv.handleDeviceCreate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "invalid json") {
		t.Fatalf("expected invalid json message, got %q", rr.Body.String())
	}
}

func TestHandleDeviceCreateMissingFields(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices", strings.NewReader(`{"protocol":"","external_id":""}`))
	rr := httptest.NewRecorder()

	srv.handleDeviceCreate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "protocol and external_id are required") {
		t.Fatalf("expected external id message, got %q", rr.Body.String())
	}
}

func TestParseHDPDeviceRequestPath_WithSlashDeviceIDCommands(t *testing.T) {
	deviceID, action, ok := parseHDPDeviceRequestPath("/api/hdp/devices/zigbee/0xa4c13867e32d96d4/commands")
	if !ok {
		t.Fatalf("expected ok")
	}
	if action != "commands" {
		t.Fatalf("expected action commands, got %q", action)
	}
	if deviceID != "zigbee/0xa4c13867e32d96d4" {
		t.Fatalf("expected device id %q got %q", "zigbee/0xa4c13867e32d96d4", deviceID)
	}
}

func TestHandleDeviceRequest_StateIsRequired(t *testing.T) {
	srv := NewServer(nil, nil)

	body := `{"correlation_id":"test-corr"}`
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/zigbee/0xa4c13867e32d96d4/commands", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.handleDeviceRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d body=%q", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "state is required") {
		t.Fatalf("expected state required message, got %q", rr.Body.String())
	}
}

func TestParseHDPDeviceRequestPath_WithSlashDeviceIDRefresh(t *testing.T) {
	deviceID, action, ok := parseHDPDeviceRequestPath("/api/hdp/devices/zigbee/0xa4c13867e32d96d4/refresh")
	if !ok {
		t.Fatalf("expected ok")
	}
	if action != "refresh" {
		t.Fatalf("expected action refresh, got %q", action)
	}
	if deviceID != "zigbee/0xa4c13867e32d96d4" {
		t.Fatalf("expected device id %q got %q", "zigbee/0xa4c13867e32d96d4", deviceID)
	}
}

func TestParseHDPDeviceRequestPath_WithSlashDeviceIDReconfigure(t *testing.T) {
	deviceID, action, ok := parseHDPDeviceRequestPath("/api/hdp/devices/zigbee/0xa4c13867e32d96d4/reconfigure")
	if !ok {
		t.Fatalf("expected ok")
	}
	if action != "reconfigure" {
		t.Fatalf("expected action reconfigure, got %q", action)
	}
	if deviceID != "zigbee/0xa4c13867e32d96d4" {
		t.Fatalf("expected device id %q got %q", "zigbee/0xa4c13867e32d96d4", deviceID)
	}
}

func TestHandleDeviceReconfigureInvalidJSON(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/abc/reconfigure", strings.NewReader(`{bad`))
	rr := httptest.NewRecorder()

	srv.handleDeviceReconfigure(rr, req, "abc")

	if rr.Code != http.StatusInternalServerError && rr.Code != http.StatusBadRequest && rr.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: got %d", rr.Code)
	}
}

func TestManagementActionsForProtocol_UsesInterviewSupport(t *testing.T) {
	srv := NewServer(nil, nil)
	srv.adapters.byID["zigbee"] = adapterStatus{
		AdapterID: "zigbee",
		Protocol:  "zigbee",
		Status:    "online",
		LastSeen:  time.Now().UTC(),
		Pairing: &PairingConfig{
			Protocol:          "zigbee",
			Supported:         true,
			SupportsInterview: true,
		},
	}

	actions := srv.managementActionsForProtocol("zigbee")
	if len(actions) != 1 {
		t.Fatalf("expected one management action, got %#v", actions)
	}
	if actions[0].Command != "reconfigure" || actions[0].Mode != "interview" {
		t.Fatalf("expected reconfigure interview action, got %#v", actions[0])
	}
}

func TestConfigurationStatusForDevice_ReadyWhenCapabilitiesPresent(t *testing.T) {
	status := configurationStatusForDevice(&model.Device{
		Protocol:     "zigbee",
		Capabilities: datatypes.JSON([]byte(`[{"id":"state"}]`)),
	})
	if !status.Ready || status.Status != "configured" {
		t.Fatalf("expected configured status, got %#v", status)
	}
	if status.Message != "" {
		t.Fatalf("expected empty configured message, got %q", status.Message)
	}
}

func TestConfigurationStatusForDevice_IncompleteWhenMetadataMissing(t *testing.T) {
	status := configurationStatusForDevice(&model.Device{Protocol: "zigbee"})
	if status.Ready {
		t.Fatalf("expected incomplete status, got %#v", status)
	}
	if status.Status != "incomplete" {
		t.Fatalf("expected incomplete status string, got %#v", status)
	}
	if !strings.Contains(strings.ToLower(status.Message), "reinterview") {
		t.Fatalf("expected zigbee guidance message, got %q", status.Message)
	}
}

func TestParseHDPDeviceRequestPath_WithSlashDeviceIDGet(t *testing.T) {
	deviceID, action, ok := parseHDPDeviceRequestPath("/api/hdp/devices/zigbee/0xa4c13867e32d96d4")
	if !ok {
		t.Fatalf("expected ok")
	}
	if action != "" {
		t.Fatalf("expected empty action, got %q", action)
	}
	if deviceID != "zigbee/0xa4c13867e32d96d4" {
		t.Fatalf("expected device id %q got %q", "zigbee/0xa4c13867e32d96d4", deviceID)
	}
}
