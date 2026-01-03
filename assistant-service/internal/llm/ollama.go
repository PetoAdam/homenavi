package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant", "tool"
	Content string `json:"content"`
}

// StreamCallback is called for each token in a streaming response
type StreamCallback func(token string, done bool)

// Client interface for LLM interactions
type Client interface {
	Chat(ctx context.Context, messages []Message) (string, error)
	ChatStream(ctx context.Context, messages []Message, cb StreamCallback) error
	Available(ctx context.Context) (bool, error)
	Models(ctx context.Context) ([]string, error)
}

// OllamaClient implements the Client interface for Ollama
type OllamaClient struct {
	baseURL       string
	model         string
	contextLength int
	httpClient    *http.Client
}

// NewOllamaClient creates a new Ollama client
func NewOllamaClient(baseURL, model string, contextLength int) *OllamaClient {
	return &OllamaClient{
		baseURL:       baseURL,
		model:         model,
		contextLength: contextLength,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for generation
		},
	}
}

// ChatRequest represents an Ollama chat request
type ChatRequest struct {
	Model    string                 `json:"model"`
	Messages []Message              `json:"messages"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// ChatResponse represents an Ollama chat response
type ChatResponse struct {
	Model     string  `json:"model"`
	CreatedAt string  `json:"created_at"`
	Message   Message `json:"message"`
	Done      bool    `json:"done"`
}

// Chat sends messages and returns the complete response
func (c *OllamaClient) Chat(ctx context.Context, messages []Message) (string, error) {
	var response string
	err := c.ChatStream(ctx, messages, func(token string, done bool) {
		response += token
	})
	return response, err
}

// ChatStream sends messages and streams the response via callback
func (c *OllamaClient) ChatStream(ctx context.Context, messages []Message, cb StreamCallback) error {
	payload := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
		Options: map[string]interface{}{
			"num_ctx": c.contextLength,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Increase scanner buffer for long responses
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var chunk ChatResponse
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		cb(chunk.Message.Content, chunk.Done)
		if chunk.Done {
			break
		}
	}

	return scanner.Err()
}

// Available checks if Ollama is reachable
func (c *OllamaClient) Available(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// ModelsResponse represents the response from /api/tags
type ModelsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// Models returns a list of available models
func (c *OllamaClient) Models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var modelsResp ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, err
	}

	models := make([]string, len(modelsResp.Models))
	for i, m := range modelsResp.Models {
		models[i] = m.Name
	}
	return models, nil
}

// GetModel returns the current model name
func (c *OllamaClient) GetModel() string {
	return c.model
}
