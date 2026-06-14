package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestResolveSelector_DedupAndCache(t *testing.T) {
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/ers/selectors/resolve" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			Selector string `json:"selector"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Selector != "tag:kitchen" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hdp_external_ids": []string{"dev-1", "dev-2", "dev-1", ""},
			"hdp_device_ids":   []string{"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "11111111-1111-1111-1111-111111111111", ""},
		})
	}))
	defer ts.Close()

	e := New(nil, nil, Options{HTTPClient: ts.Client(), ERSServiceURL: ts.URL})

	ctx := context.Background()
	ids1, err := e.resolveSelector(ctx, "tag:kitchen")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(ids1) != 2 || ids1[0] != "dev-1" || ids1[1] != "dev-2" {
		t.Fatalf("unexpected ids: %#v", ids1)
	}

	ids2, err := e.resolveSelector(ctx, "tag:kitchen")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(ids2) != 2 || ids2[0] != "dev-1" || ids2[1] != "dev-2" {
		t.Fatalf("unexpected ids: %#v", ids2)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 HTTP call due to cache, got %d", got)
	}
}

func TestResolveSelectorTargets_TypedIDs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hdp_external_ids": []string{"dev-1"},
			"hdp_device_ids":   []string{"11111111-1111-1111-1111-111111111111"},
		})
	}))
	defer ts.Close()

	e := New(nil, nil, Options{HTTPClient: ts.Client(), ERSServiceURL: ts.URL})
	ctx := context.Background()
	targets, err := e.resolveSelectorTargets(ctx, "tag:kitchen")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(targets) != 1 || targets[0].ExternalID != "dev-1" || targets[0].HDPDeviceID == nil || targets[0].HDPDeviceID.String() != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestResolveTargets_Device(t *testing.T) {
	e := New(nil, nil, Options{ERSServiceURL: "http://example"})
	ctx := context.Background()

	ids, err := e.resolveTargets(ctx, NodeTargets{Type: "device", IDs: []string{" dev-1 ", "dev-1", "", "dev-2"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(ids) != 2 || ids[0].ExternalID != "dev-1" || ids[1].ExternalID != "dev-2" {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}

func TestTargetMatchesDevice_Selector(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"hdp_external_ids": []string{"dev-9"}})
	}))
	defer ts.Close()

	e := New(nil, nil, Options{HTTPClient: ts.Client(), ERSServiceURL: ts.URL})
	ctx := context.Background()

	ok, err := e.targetMatchesDevice(ctx, NodeTargets{Type: "selector", Selector: "tag:any"}, "dev-9")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Fatalf("expected match")
	}
}
