package app

import (
	"errors"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPairHandlerReturnsStubResponse(t *testing.T) {
	handler := pairHandler(Config{})
	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"protocol":"matter","mode":"qr_code","flow_id":"flow-1","inputs":{"network_path":"thread","onboarding_payload":"MT:ABC123"}}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response PairResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(response.ExternalID, "matter-") {
		t.Fatalf("expected generated external id, got %#v", response)
	}
	if response.Metadata["network_path"] != "thread" {
		t.Fatalf("expected thread network path metadata, got %#v", response.Metadata)
	}
	if response.State["on"] != false {
		t.Fatalf("expected default state off, got %#v", response.State)
	}
}

func TestPairHandlerRejectsInvalidMethod(t *testing.T) {
	handler := pairHandler(Config{})
	req := httptest.NewRequest(http.MethodGet, "/pair", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", recorder.Code)
	}
}

func TestCommandHandlerReturnsStubResponse(t *testing.T) {
	handler := commandHandler(Config{})
	req := httptest.NewRequest(http.MethodPost, "/command", strings.NewReader(`{"protocol":"matter","device_id":"123456","command":"set_level","args":{"level":42}}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response PairResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.DeviceID != "123456" {
		t.Fatalf("expected device id to round-trip, got %#v", response)
	}
	if response.State["level"] != float64(42) {
		t.Fatalf("expected stub level state, got %#v", response.State)
	}
	if response.State["on"] != true {
		t.Fatalf("expected stub on=true, got %#v", response.State)
	}
}

func TestCommandHandlerRejectsInvalidMethod(t *testing.T) {
	handler := commandHandler(Config{})
	req := httptest.NewRequest(http.MethodGet, "/command", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", recorder.Code)
	}
}

func TestClassifyBackendErrorBLEDiscoveryTimeout(t *testing.T) {
	message := classifyBackendError("pair", []byte("\x1b[1;31mBLE adapter unavailable\x1b[0m\nDiscovery timed out\n"), errors.New("exit status 1"))
	if !strings.Contains(message, "BLE adapter unavailable") {
		t.Fatalf("expected BLE availability message, got %q", message)
	}
	if !strings.Contains(message, "timed out") {
		t.Fatalf("expected timeout message, got %q", message)
	}
}

func TestClassifyBackendErrorLongDiscriminator(t *testing.T) {
	message := classifyBackendError("pair", []byte("Error, Long discriminator is required\n"), errors.New("exit status 1"))
	if message != "On-network commissioning requires the long discriminator, which is not available from manual code alone; use QR/onboarding payload, provide the discriminator explicitly, or switch to Manual Code mode for BLE commissioning" {
		t.Fatalf("unexpected message %q", message)
	}
}