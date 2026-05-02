package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestHandlePairingConfigIgnoresLegacyFeatureOnlyAnnouncements(t *testing.T) {
	srv := NewServer(nil, nil)
	srv.adapters.upsertFromHello([]byte(`{
		"schema":"hdp.v1",
		"type":"hello",
		"adapter_id":"legacy-zigbee",
		"protocol":"zigbee",
		"version":"test",
		"features": {"supports_pairing": true, "supports_interview": true},
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
	if len(got) != 0 {
		t.Fatalf("expected empty config for legacy feature-only announcement, got %d", len(got))
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

func TestHandlePairingConfigPreservesFullFlowAcrossStatusUpdates(t *testing.T) {
	srv := NewServer(nil, nil)
	srv.adapters.upsertFromHello([]byte(`{
		"schema":"hdp.v1",
		"type":"hello",
		"adapter_id":"matter-adapter-1",
		"protocol":"matter",
		"version":"test",
		"features":{"supports_pairing":true,"supports_interview":true},
		"pairing":{
		  "schema_version":"1.0",
		  "label":"Matter",
		  "supported":true,
		  "supports_interview":true,
		  "default_timeout_sec":300,
		  "instructions":["step one"],
		  "cta_label":"Start Matter pairing",
		  "flow":{
		    "id":"matter-commissioning-v1",
		    "entry_modes":["manual_code","qr_code"],
		    "forms":[{"mode":"manual_code","fields":[{"id":"manual_code","component":"text","label":"Manual setup code"}]}],
		    "steps":[{"id":"discovery","stage":"discovery","label":"Discovery"}]
		  }
		},
		"ts":1
	}`))
	srv.adapters.upsertFromStatusTopic("homenavi/hdp/adapter/status/matter-adapter-1", []byte(`{
		"schema":"hdp.v1",
		"type":"status",
		"adapter_id":"matter-adapter-1",
		"protocol":"matter",
		"status":"online",
		"version":"test",
		"pairing":{
		  "schema_version":"1.0",
		  "label":"Matter",
		  "supported":true,
		  "default_timeout_sec":300,
		  "flow":{"id":"matter-commissioning-v1","entry_modes":["manual_code","qr_code"]}
		},
		"ts":2
	}`))

	req := httptest.NewRequest(http.MethodGet, "/api/hdp/pairing-config", nil)
	rr := httptest.NewRecorder()
	srv.handlePairingConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusOK)
	}

	var got []PairingConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid json: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 pairing config, got %d", len(got))
	}
	flow, ok := got[0].Flow.(map[string]any)
	if !ok {
		t.Fatalf("expected flow map, got %T", got[0].Flow)
	}
	forms, ok := flow["forms"].([]any)
	if !ok || len(forms) == 0 {
		t.Fatalf("expected forms preserved from hello after status update, got %#v", flow["forms"])
	}
	steps, ok := flow["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("expected steps preserved from hello after status update, got %#v", flow["steps"])
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
