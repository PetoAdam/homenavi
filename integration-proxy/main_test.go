package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"testing"

	"homenavi/integration-proxy/internal/server"
)

func TestRegistryEmptyIntegrationsIsArray(t *testing.T) {
	s := server.New(log.New(io.Discard, "", 0), nil, nil, "", "")
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/integrations/registry.json", nil)

	s.Routes().ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var payload map[string]any
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}

	v, ok := payload["integrations"]
	if !ok {
		t.Fatalf("missing integrations field")
	}
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("expected integrations to be array, got %T (%v)", v, v)
	}
	if len(arr) != 0 {
		t.Fatalf("expected empty integrations array, got len=%d", len(arr))
	}
}
