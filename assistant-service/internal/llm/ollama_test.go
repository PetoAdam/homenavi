package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/homenavi/assistant-service/internal/llm"
)

func TestOllamaClient_Available(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse int
		want           bool
		wantErr        bool
	}{
		{
			name:           "server available",
			serverResponse: http.StatusOK,
			want:           true,
			wantErr:        false,
		},
		{
			name:           "server unavailable",
			serverResponse: http.StatusServiceUnavailable,
			want:           false,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/tags" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(tt.serverResponse)
				json.NewEncoder(w).Encode(map[string]interface{}{"models": []interface{}{}})
			}))
			defer server.Close()

			client := llm.NewOllamaClient(server.URL, "test-model", 4096)
			ctx := context.Background()

			got, err := client.Available(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Available() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Available() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOllamaClient_Models(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]string{
				{"name": "llama3.1:8b"},
				{"name": "mistral:7b"},
			},
		})
	}))
	defer server.Close()

	client := llm.NewOllamaClient(server.URL, "test-model", 4096)
	ctx := context.Background()

	models, err := client.Models(ctx)
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}

	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}

	if models[0] != "llama3.1:8b" {
		t.Errorf("expected first model to be llama3.1:8b, got %s", models[0])
	}
}

func TestOllamaClient_Chat(t *testing.T) {
	expectedResponse := "Hello! I'm an AI assistant."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify request body
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)

		if req["model"] != "test-model" {
			t.Errorf("expected model test-model, got %v", req["model"])
		}

		// Send streaming response
		w.Header().Set("Content-Type", "application/x-ndjson")

		tokens := strings.Split(expectedResponse, " ")
		for i, token := range tokens {
			if i > 0 {
				token = " " + token
			}
			chunk := map[string]interface{}{
				"message": map[string]string{"content": token},
				"done":    i == len(tokens)-1,
			}
			json.NewEncoder(w).Encode(chunk)
		}
	}))
	defer server.Close()

	client := llm.NewOllamaClient(server.URL, "test-model", 4096)
	ctx := context.Background()

	messages := []llm.Message{
		{Role: "user", Content: "Hello"},
	}

	response, err := client.Chat(ctx, messages)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if response != expectedResponse {
		t.Errorf("Chat() = %q, want %q", response, expectedResponse)
	}
}

func TestOllamaClient_ChatStream(t *testing.T) {
	tokens := []string{"Hello", " there", "!"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")

		for i, token := range tokens {
			chunk := map[string]interface{}{
				"message": map[string]string{"content": token},
				"done":    i == len(tokens)-1,
			}
			json.NewEncoder(w).Encode(chunk)
		}
	}))
	defer server.Close()

	client := llm.NewOllamaClient(server.URL, "test-model", 4096)
	ctx := context.Background()

	messages := []llm.Message{
		{Role: "user", Content: "Hi"},
	}

	var receivedTokens []string
	var doneReceived bool

	err := client.ChatStream(ctx, messages, func(token string, done bool) {
		if token != "" {
			receivedTokens = append(receivedTokens, token)
		}
		if done {
			doneReceived = true
		}
	})

	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}

	if !doneReceived {
		t.Error("expected done to be true")
	}

	if len(receivedTokens) != len(tokens) {
		t.Errorf("expected %d tokens, got %d", len(tokens), len(receivedTokens))
	}
}

func TestOllamaClient_ChatStream_Cancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow streaming
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	client := llm.NewOllamaClient(server.URL, "test-model", 4096)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	messages := []llm.Message{
		{Role: "user", Content: "Hi"},
	}

	err := client.ChatStream(ctx, messages, func(token string, done bool) {})

	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

func TestOllamaClient_GetModel(t *testing.T) {
	client := llm.NewOllamaClient("http://localhost:11434", "my-model", 4096)

	if got := client.GetModel(); got != "my-model" {
		t.Errorf("GetModel() = %q, want %q", got, "my-model")
	}
}
