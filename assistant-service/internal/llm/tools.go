package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ToolDefinition represents a tool for Ollama function calling
type ToolDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction represents a function definition
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall represents a tool call from the model
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction represents the function being called
type ToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ChatRequestWithTools extends ChatRequest with tools support
type ChatRequestWithTools struct {
	Model    string                 `json:"model"`
	Messages []Message              `json:"messages"`
	Stream   bool                   `json:"stream"`
	Tools    []ToolDefinition       `json:"tools,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// ChatResponseWithTools includes tool calls
type ChatResponseWithTools struct {
	Model     string     `json:"model"`
	CreatedAt string     `json:"created_at"`
	Message   MessageExt `json:"message"`
	Done      bool       `json:"done"`
}

// MessageExt extends Message with tool_calls
type MessageExt struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolStreamCallback handles streaming with tool awareness
type ToolStreamCallback func(token string, toolCalls []ToolCall, done bool)

// ChatWithTools sends a chat request with tool definitions
func (c *OllamaClient) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, cb ToolStreamCallback) error {
	payload := ChatRequestWithTools{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
		Tools:    tools,
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
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var allToolCalls []ToolCall

	for scanner.Scan() {
		var chunk ChatResponseWithTools
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}

		// Collect tool calls
		if len(chunk.Message.ToolCalls) > 0 {
			allToolCalls = append(allToolCalls, chunk.Message.ToolCalls...)
		}

		// Stream content tokens
		if chunk.Message.Content != "" {
			cb(chunk.Message.Content, nil, false)
		}

		if chunk.Done {
			cb("", allToolCalls, true)
			break
		}
	}

	return scanner.Err()
}

// ChatWithToolsSync sends a chat request and returns the complete response with tool calls
func (c *OllamaClient) ChatWithToolsSync(ctx context.Context, messages []Message, tools []ToolDefinition) (string, []ToolCall, error) {
	var content string
	var toolCalls []ToolCall

	err := c.ChatWithTools(ctx, messages, tools, func(token string, calls []ToolCall, done bool) {
		content += token
		if len(calls) > 0 {
			toolCalls = calls
		}
	})

	return content, toolCalls, err
}
