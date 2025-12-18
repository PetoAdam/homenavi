package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"history-service/internal/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	// Use a unique in-memory DB per test to avoid cross-test contamination.
	dsn := "file:httpapi_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := store.New(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(repo)
}

func TestHealth(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/history/health", nil)
	rw := httptest.NewRecorder()
	s.Handler().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rw.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rw.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp)
	}
}

func TestListStateRequiresDeviceID(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/history/state", nil)
	rw := httptest.NewRecorder()
	s.Handler().ServeHTTP(rw, req)
	if rw.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rw.Code)
	}
}

func TestListStateReturnsPoints(t *testing.T) {
	s := newTestServer(t)
	// Insert one row
	received := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = s.repo.InsertStatePoint(context.Background(), &store.DeviceStatePoint{DeviceID: "zigbee/0x1", TS: received, Payload: []byte(`{"x":1}`)})

	req := httptest.NewRequest(http.MethodGet, "/api/history/state?device_id=zigbee/0x1&limit=10", nil)
	rw := httptest.NewRecorder()
	s.Handler().ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rw.Code, rw.Body.String())
	}
	var resp listStateResponse
	if err := json.Unmarshal(rw.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(resp.Points))
	}
}
