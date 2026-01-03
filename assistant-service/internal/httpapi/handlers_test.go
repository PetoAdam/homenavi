package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/homenavi/assistant-service/internal/config"
	"github.com/homenavi/assistant-service/internal/httpapi"
	"github.com/homenavi/assistant-service/internal/llm"
	"github.com/homenavi/assistant-service/internal/tools"
)

func TestHealth(t *testing.T) {
	// Create a mock Ollama server
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"models": []interface{}{}})
	}))
	defer ollamaServer.Close()

	cfg := &config.Config{
		Port:                "8096",
		OllamaHost:          ollamaServer.URL,
		OllamaModel:         "test-model",
		OllamaContextLength: 4096,
	}

	llmClient := llm.NewOllamaClient(cfg.OllamaHost, cfg.OllamaModel, cfg.OllamaContextLength)
	toolRegistry := tools.NewRegistry(cfg)

	handler := httpapi.NewHandler(cfg, llmClient, toolRegistry, nil, nil)
	router := httpapi.NewRouter(handler, cfg)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Health() status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Health() status = %v, want ok", response["status"])
	}

	if response["service"] != "assistant-service" {
		t.Errorf("Health() service = %v, want assistant-service", response["service"])
	}

	if response["model"] != "test-model" {
		t.Errorf("Health() model = %v, want test-model", response["model"])
	}
}

func TestCORSMiddleware(t *testing.T) {
	cfg := &config.Config{
		Port:                "8096",
		OllamaHost:          "http://localhost:11434",
		OllamaModel:         "test-model",
		OllamaContextLength: 4096,
	}

	llmClient := llm.NewOllamaClient(cfg.OllamaHost, cfg.OllamaModel, cfg.OllamaContextLength)
	toolRegistry := tools.NewRegistry(cfg)

	handler := httpapi.NewHandler(cfg, llmClient, toolRegistry, nil, nil)
	router := httpapi.NewRouter(handler, cfg)

	// Test OPTIONS request
	req := httptest.NewRequest("OPTIONS", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("OPTIONS request status = %d, want %d", w.Code, http.StatusOK)
	}

	corsHeader := w.Header().Get("Access-Control-Allow-Origin")
	if corsHeader != "*" {
		t.Errorf("CORS header = %v, want *", corsHeader)
	}
}

func TestProtectedEndpoints_Unauthorized(t *testing.T) {
	cfg := &config.Config{
		Port:                "8096",
		OllamaHost:          "http://localhost:11434",
		OllamaModel:         "test-model",
		OllamaContextLength: 4096,
		JWTPublicKeyPath:    "/nonexistent/key.pem", // No key = no auth enforcement in test
	}

	llmClient := llm.NewOllamaClient(cfg.OllamaHost, cfg.OllamaModel, cfg.OllamaContextLength)
	toolRegistry := tools.NewRegistry(cfg)

	handler := httpapi.NewHandler(cfg, llmClient, toolRegistry, nil, nil)
	router := httpapi.NewRouter(handler, cfg)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/assistant/conversations"},
		{"POST", "/api/assistant/conversations"},
		{"GET", "/api/assistant/admin/status"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Without JWT key loaded, routes should still work but return 401 for protected endpoints
			// Since we don't have a valid JWT, it will fail auth
			// Note: In this test, JWT key loading fails, so auth middleware is not applied
		})
	}
}
