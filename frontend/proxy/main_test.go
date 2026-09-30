package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectRuntimeScript(t *testing.T) {
	index := []byte("<html><head><title>Demo</title></head><body></body></html>")
	got := string(injectRuntimeScript(index, "/__homenavi_runtime_config__.js"))
	if !strings.Contains(got, `<script src="/__homenavi_runtime_config__.js"></script></head>`) {
		t.Fatalf("expected runtime config script to be injected, got %q", got)
	}
}

func TestRuntimeConfigHandlerServesDemoFlag(t *testing.T) {
	h := runtimeConfigHandler(runtimeConfig{
		ScriptPath: "/__homenavi_runtime_config__.js",
		Payload: map[string]any{"VITE_DEMO_MODE": true},
	})
	req := httptest.NewRequest(http.MethodGet, "/__homenavi_runtime_config__.js", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"VITE_DEMO_MODE":true`) {
		t.Fatalf("expected VITE_DEMO_MODE in payload, got %q", rr.Body.String())
	}
}

func TestSpaHandlerServesIndexWithRuntimeScript(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html><head></head><body>Hello</body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	h := spaHandler(root, "/__homenavi_runtime_config__.js")
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `/__homenavi_runtime_config__.js`) {
		t.Fatalf("expected runtime config script in index response, got %q", rr.Body.String())
	}
}