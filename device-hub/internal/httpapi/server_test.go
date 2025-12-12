package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlePairingConfigReturnsConfigs(t *testing.T) {
	configs := []PairingConfig{
		{Protocol: "zigbee", Label: "Zigbee", Supported: true, DefaultTimeoutSec: 60},
		{Protocol: "thread", Label: "Thread", Supported: false, DefaultTimeoutSec: 30},
	}
	srv := NewServer(nil, nil, nil, configs)

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

	if len(got) != len(configs) {
		t.Fatalf("unexpected config count: got %d want %d", len(got), len(configs))
	}

	if got[0].Protocol != "zigbee" || got[1].Protocol != "thread" {
		t.Fatalf("protocols not preserved: %+v", got)
	}
}

func TestHandlePairingConfigEmpty(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil)

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
	srv := NewServer(nil, nil, nil, nil)
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
	srv := NewServer(nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/abc/commands", strings.NewReader(`{"state":{},"input":null}`))
	rr := httptest.NewRecorder()

	srv.handleDeviceCommand(rr, req, "abc")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "state or input required") {
		t.Fatalf("expected missing state message, got %q", rr.Body.String())
	}
}

func TestHandleDeviceCreateInvalidJSON(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil)
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
	srv := NewServer(nil, nil, nil, nil)
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
