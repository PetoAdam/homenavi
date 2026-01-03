# AI Assistant Service Integration Roadmap

## Overview

This roadmap outlines the complete integration of an AI assistant service into the Homenavi home automation platform. The assistant will leverage locally-run Ollama for LLM inference, with the flexibility to offload to a remote machine in production.

---

## Table of Contents

1. [Architecture Decision: MCP vs REST + WebSocket](#1-architecture-decision)
2. [Hardware Considerations](#2-hardware-considerations)
3. [Phase 1: Infrastructure Setup](#3-phase-1-infrastructure-setup)
4. [Phase 2: Backend Service Development](#4-phase-2-backend-service-development)
5. [Phase 3: API Gateway Integration](#5-phase-3-api-gateway-integration)
6. [Phase 4: Frontend Integration](#6-phase-4-frontend-integration)
7. [Phase 5: Tool Integration & Context Awareness](#7-phase-5-tool-integration)
8. [Phase 6: Security & Authorization](#8-phase-6-security-authorization)
9. [Phase 7: Testing & Optimization](#9-phase-7-testing-optimization)
10. [Future Enhancements](#10-future-enhancements)

---

## 1. Architecture Decision: MCP vs REST + WebSocket {#1-architecture-decision}

### Recommendation: **REST + WebSocket Hybrid**

After analyzing the current Homenavi architecture, I recommend using a **REST + WebSocket hybrid approach** rather than MCP (Model Context Protocol) for the following reasons:

#### Why REST + WebSocket (Recommended)

| Aspect | REST + WebSocket | MCP |
|--------|------------------|-----|
| **Consistency** | Matches existing architecture (ERS, Automation use WS) | Would introduce a new protocol pattern |
| **Complexity** | Lower - reuses existing patterns | Higher - requires MCP server/client setup |
| **Streaming** | WebSocket provides real-time token streaming | MCP also supports streaming but adds overhead |
| **JWT Integration** | Direct - same auth flow as other services | Requires additional bridge/adapter |
| **Frontend Integration** | Familiar patterns for React devs | Requires MCP client library |
| **Debugging** | Standard browser DevTools | Specialized MCP tooling |

#### API Design

```
REST Endpoints:
  POST /api/assistant/chat           - Start new conversation
  GET  /api/assistant/conversations  - List user's conversations
  GET  /api/assistant/conversations/:id - Get conversation history
  DELETE /api/assistant/conversations/:id - Delete conversation

WebSocket:
  WS /ws/assistant - Real-time chat with streaming responses
```

#### When to Consider MCP (Future)

- If integrating with external AI tools that natively support MCP
- For plugin/extension ecosystems
- If Claude/Anthropic tooling becomes more MCP-centric

---

## 2. Hardware Considerations {#2-hardware-considerations}

### Development Machine: RTX 4070 SUPER (12GB VRAM)

**Recommended Models:**
- **Primary**: `llama3.1:8b` or `mistral:7b` (fast, capable)
- **Alternative**: `llama3.1:70b-q4` (better quality, slower)
- **Coding**: `deepseek-coder:6.7b` or `codellama:13b`

**Expected Performance:**
- 7B models: ~50-80 tokens/sec
- 13B models: ~25-40 tokens/sec
- Context window: 8K-32K tokens easily

### Production Server: GTX 1060 6GB

**Recommended Models:**
- **Primary**: `llama3.2:3b` or `phi3:mini` (3.8B)
- **Alternative**: `gemma2:2b` (very fast, basic)
- **Quantized**: `mistral:7b-q4` (fits in 6GB with quantization)

**Strategies for 6GB VRAM:**
1. Use 4-bit quantized models (`q4_0`, `q4_K_M`)
2. Limit context window (2K-4K tokens)
3. Implement request queuing to prevent OOM
4. Consider CPU offloading for larger models

### Flexible Configuration (Environment Variables)

```yaml
# docker-compose.yml
environment:
  - OLLAMA_HOST=ollama:11434          # Local container
  - OLLAMA_HOST=http://192.168.1.X:11434  # Remote machine
  - OLLAMA_MODEL=llama3.1:8b          # Dev
  - OLLAMA_MODEL=llama3.2:3b          # Prod
  - OLLAMA_CONTEXT_LENGTH=4096
  - OLLAMA_NUM_GPU=1
```

---

## 3. Phase 1: Infrastructure Setup {#3-phase-1-infrastructure-setup}

### 3.1 Add Ollama to Docker Compose

**File**: `docker-compose.yml`

```yaml
ollama:
  image: ollama/ollama:latest
  restart: unless-stopped
  networks:
    - homenavi-network
  volumes:
    - ollama_models:/root/.ollama
  environment:
    - OLLAMA_HOST=0.0.0.0:11434
    - OLLAMA_NUM_PARALLEL=${OLLAMA_NUM_PARALLEL:-2}
    - OLLAMA_MAX_LOADED_MODELS=${OLLAMA_MAX_LOADED_MODELS:-1}
  # For GPU support (uncomment based on your setup):
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: 1
            capabilities: [gpu]
  # DEV: expose for direct testing
  # ports:
  #   - "11434:11434"
```

Add to volumes:
```yaml
volumes:
  ollama_models:
```

### 3.2 Environment Variables

**File**: `.env`

```bash
# Ollama Configuration
OLLAMA_HOST=ollama
OLLAMA_PORT=11434
OLLAMA_MODEL=llama3.1:8b
OLLAMA_CONTEXT_LENGTH=4096
OLLAMA_NUM_PARALLEL=2
OLLAMA_MAX_LOADED_MODELS=1

# Assistant Service
ASSISTANT_SERVICE_PORT=8096
ASSISTANT_MAX_TOKENS=2048
ASSISTANT_TEMPERATURE=0.7
ASSISTANT_SYSTEM_PROMPT_PATH=/app/prompts/system.txt
```

### 3.3 Model Pull Script

Create `scripts/pull-ollama-models.sh`:

```bash
#!/bin/bash
# Pull required models after Ollama container starts

MODELS="${OLLAMA_MODELS:-llama3.1:8b}"

for model in $MODELS; do
  echo "Pulling model: $model"
  docker compose exec ollama ollama pull "$model"
done
```

---

## 4. Phase 2: Backend Service Development {#4-phase-2-backend-service-development}

### 4.1 Service Structure

```
assistant-service/
├── cmd/
│   └── assistant-service/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go           # Configuration loading
│   ├── httpapi/
│   │   ├── router.go           # HTTP/WS router setup
│   │   ├── handlers.go         # REST handlers
│   │   └── websocket.go        # WebSocket handler
│   ├── llm/
│   │   ├── client.go           # Ollama client interface
│   │   ├── ollama.go           # Ollama implementation
│   │   └── streaming.go        # Token streaming logic
│   ├── tools/
│   │   ├── registry.go         # Tool registration
│   │   ├── device_control.go   # Device control tools
│   │   ├── entity_query.go     # Entity registry queries
│   │   ├── automation.go       # Automation tools
│   │   └── history.go          # History queries
│   ├── context/
│   │   ├── builder.go          # Context assembly
│   │   └── memory.go           # Conversation memory
│   ├── auth/
│   │   └── jwt.go              # JWT validation
│   └── models/
│       ├── conversation.go     # Conversation model
│       └── message.go          # Message model
├── Dockerfile
└── go.mod
```

### 4.2 Core Components

#### LLM Client Interface

```go
// internal/llm/client.go
package llm

import "context"

type Message struct {
    Role    string `json:"role"`    // "system", "user", "assistant"
    Content string `json:"content"`
}

type StreamCallback func(token string, done bool)

type Client interface {
    // Chat sends a message and returns the full response
    Chat(ctx context.Context, messages []Message) (string, error)
    
    // ChatStream sends a message and streams tokens via callback
    ChatStream(ctx context.Context, messages []Message, cb StreamCallback) error
    
    // Available checks if the LLM backend is available
    Available(ctx context.Context) (bool, error)
    
    // Models lists available models
    Models(ctx context.Context) ([]string, error)
}
```

#### Ollama Implementation

```go
// internal/llm/ollama.go
package llm

import (
    "bufio"
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

type OllamaClient struct {
    baseURL    string
    model      string
    httpClient *http.Client
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
    return &OllamaClient{
        baseURL:    baseURL,
        model:      model,
        httpClient: &http.Client{},
    }
}

func (c *OllamaClient) ChatStream(ctx context.Context, messages []Message, cb StreamCallback) error {
    payload := map[string]interface{}{
        "model":    c.model,
        "messages": messages,
        "stream":   true,
    }
    
    body, _ := json.Marshal(payload)
    req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(body))
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        var chunk struct {
            Message struct {
                Content string `json:"content"`
            } `json:"message"`
            Done bool `json:"done"`
        }
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
```

#### WebSocket Handler

```go
// internal/httpapi/websocket.go
package httpapi

import (
    "encoding/json"
    "net/http"
    
    "github.com/gorilla/websocket"
)

type WSMessage struct {
    Type    string          `json:"type"`    // "message", "typing", "error", "tool_call"
    Content string          `json:"content,omitempty"`
    Data    json.RawMessage `json:"data,omitempty"`
}

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
    // JWT claims already validated by gateway middleware
    claims := r.Context().Value("claims").(*Claims)
    
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()
    
    // Create session with user context
    session := h.sessionManager.Create(claims.Subject, claims.Role)
    defer h.sessionManager.Close(session.ID)
    
    for {
        _, msg, err := conn.ReadMessage()
        if err != nil {
            break
        }
        
        var wsMsg WSMessage
        if err := json.Unmarshal(msg, &wsMsg); err != nil {
            continue
        }
        
        switch wsMsg.Type {
        case "message":
            h.handleUserMessage(conn, session, wsMsg.Content, claims)
        case "cancel":
            session.CancelCurrentRequest()
        }
    }
}
```

### 4.3 Database Schema

```sql
-- migrations/001_create_conversations.sql

CREATE TABLE IF NOT EXISTS assistant_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS assistant_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES assistant_conversations(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL, -- 'user', 'assistant', 'system', 'tool'
    content TEXT NOT NULL,
    tool_calls JSONB,
    tool_results JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_conversations_user ON assistant_conversations(user_id);
CREATE INDEX idx_messages_conversation ON assistant_messages(conversation_id);
```

### 4.4 Dockerfile

```dockerfile
# assistant-service/Dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o assistant-service ./cmd/assistant-service

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/assistant-service .
COPY prompts/ ./prompts/

EXPOSE 8096
CMD ["./assistant-service"]
```

---

## 5. Phase 3: API Gateway Integration {#5-phase-3-api-gateway-integration}

### 5.1 Route Configuration

Create `api-gateway/config/routes/assistant.yaml`:

```yaml
routes:
  # Health check
  - path: /api/assistant/health
    upstream: http://assistant-service:8096/health
    methods: [GET]
    access: public
    type: rest

  # REST endpoints (require authentication)
  - path: /api/assistant/conversations
    upstream: http://assistant-service:8096/api/assistant/conversations
    methods: [GET, POST]
    access: resident
    type: rest

  - path: /api/assistant/conversations/*
    upstream: http://assistant-service:8096/api/assistant/conversations/*
    methods: [GET, DELETE]
    access: resident
    type: rest

  # WebSocket for real-time chat (requires authentication)
  - path: /ws/assistant
    upstream: ws://assistant-service:8096/ws/assistant
    methods: [GET]
    access: resident
    type: websocket

  # Admin endpoints for model management
  - path: /api/assistant/admin/*
    upstream: http://assistant-service:8096/api/assistant/admin/*
    methods: [GET, POST, PUT, DELETE]
    access: admin
    type: rest
```

### 5.2 Docker Compose Service Entry

Add to `docker-compose.yml`:

```yaml
assistant-service:
  build:
    context: ./assistant-service
  restart: unless-stopped
  depends_on:
    - ollama
    - postgres
    - redis
  environment:
    - ASSISTANT_SERVICE_PORT=${ASSISTANT_SERVICE_PORT:-8096}
    - OLLAMA_HOST=http://ollama:11434
    - OLLAMA_MODEL=${OLLAMA_MODEL:-llama3.1:8b}
    - OLLAMA_CONTEXT_LENGTH=${OLLAMA_CONTEXT_LENGTH:-4096}
    - POSTGRES_USER=${POSTGRES_USER}
    - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
    - POSTGRES_DB=${POSTGRES_DB}
    - POSTGRES_HOST=${POSTGRES_HOST}
    - POSTGRES_PORT=${POSTGRES_PORT}
    - REDIS_ADDR=redis:${REDIS_PORT}
    - JWT_PUBLIC_KEY_PATH=/app/keys/jwt_public.pem
    - DEVICE_HUB_URL=http://device-hub:${DEVICE_HUB_PORT}
    - ERS_URL=http://entity-registry-service:${ENTITY_REGISTRY_PORT}
    - AUTOMATION_URL=http://automation-service:${AUTOMATION_SERVICE_PORT}
    - HISTORY_URL=http://history-service:${HISTORY_SERVICE_PORT}
    - LOG_LEVEL=${LOG_LEVEL}
    - JAEGER_ENDPOINT=http://jaeger:${JAEGER_COLLECTOR_PORT}/api/traces
  volumes:
    - ${JWT_PUBLIC_KEY_PATH}:/app/keys/jwt_public.pem:ro
  networks:
    - homenavi-network
```

---

## 6. Phase 4: Frontend Integration {#6-phase-4-frontend-integration}

### 6.1 Component Structure

```
frontend/src/components/Assistant/
├── AssistantButton.jsx       # Floating AI button
├── AssistantButton.css       # Button styles with animations
├── AssistantPanel.jsx        # Chat panel (slide-in drawer)
├── AssistantPanel.css        # Panel styles
├── ChatMessage.jsx           # Individual message component
├── ChatMessage.css           # Message styles
├── ChatInput.jsx             # Input with send button
├── TypingIndicator.jsx       # AI typing animation
└── index.js                  # Exports
```

### 6.2 AI Floating Button Design

**Apple Intelligence-inspired Design Specifications:**

```css
/* AssistantButton.css */

.assistant-button {
  position: fixed;
  bottom: 24px;
  left: 24px;
  z-index: 1000;
  
  /* Size */
  width: 56px;
  height: 56px;
  border-radius: 50%;
  
  /* Glass morphism background */
  background: linear-gradient(
    135deg,
    rgba(var(--color-success-rgb), 0.25) 0%,
    rgba(var(--color-primary), 0.15) 50%,
    rgba(120, 80, 220, 0.2) 100%
  );
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  
  /* Border with gradient */
  border: 1.5px solid transparent;
  background-clip: padding-box;
  
  /* Shadow with glow */
  box-shadow:
    0 4px 24px rgba(var(--color-success-rgb), 0.3),
    0 0 40px rgba(var(--color-success-rgb), 0.1),
    inset 0 1px 1px rgba(255, 255, 255, 0.1);
  
  /* Cursor & transition */
  cursor: pointer;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.assistant-button:hover {
  transform: scale(1.08);
  box-shadow:
    0 6px 32px rgba(var(--color-success-rgb), 0.4),
    0 0 60px rgba(var(--color-success-rgb), 0.15);
}

.assistant-button:active {
  transform: scale(0.95);
}

/* Animated gradient border */
.assistant-button::before {
  content: '';
  position: absolute;
  inset: -2px;
  border-radius: 50%;
  background: linear-gradient(
    var(--gradient-angle, 0deg),
    rgba(var(--color-success-rgb), 0.6),
    rgba(120, 80, 220, 0.5),
    rgba(59, 130, 246, 0.5),
    rgba(var(--color-success-rgb), 0.6)
  );
  z-index: -1;
  animation: rotate-gradient 4s linear infinite;
}

@keyframes rotate-gradient {
  from { --gradient-angle: 0deg; }
  to { --gradient-angle: 360deg; }
}

@property --gradient-angle {
  syntax: '<angle>';
  inherits: false;
  initial-value: 0deg;
}

/* AI Icon animation */
.assistant-button-icon {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.ai-orb {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.9) 0%,
    rgba(var(--color-success-rgb), 0.8) 50%,
    rgba(120, 80, 220, 0.7) 100%
  );
  animation: pulse-glow 2s ease-in-out infinite;
}

@keyframes pulse-glow {
  0%, 100% {
    transform: scale(1);
    box-shadow: 0 0 0 0 rgba(var(--color-success-rgb), 0.4);
  }
  50% {
    transform: scale(1.05);
    box-shadow: 0 0 20px 4px rgba(var(--color-success-rgb), 0.2);
  }
}

/* Processing state */
.assistant-button.processing .ai-orb {
  animation: processing-spin 1.5s linear infinite;
}

@keyframes processing-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* Notification badge */
.assistant-badge {
  position: absolute;
  top: -4px;
  right: -4px;
  width: 18px;
  height: 18px;
  background: var(--color-primary);
  border-radius: 50%;
  border: 2px solid var(--color-bg);
  font-size: 10px;
  font-weight: 600;
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  animation: badge-pop 0.3s cubic-bezier(0.68, -0.55, 0.265, 1.55);
}

@keyframes badge-pop {
  from { transform: scale(0); }
  to { transform: scale(1); }
}
```

### 6.3 AssistantButton Component

```jsx
// AssistantButton.jsx
import React, { useState, useContext } from 'react';
import { AuthContext } from '../../context/AuthContext';
import AssistantPanel from './AssistantPanel';
import './AssistantButton.css';

// SVG for Apple Intelligence-style AI icon
const AIIcon = ({ processing }) => (
  <svg 
    viewBox="0 0 24 24" 
    fill="none" 
    className={`ai-icon ${processing ? 'processing' : ''}`}
  >
    <defs>
      <linearGradient id="ai-gradient" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%" stopColor="#22c55e" />
        <stop offset="50%" stopColor="#7850dc" />
        <stop offset="100%" stopColor="#3b82f6" />
      </linearGradient>
    </defs>
    {/* Stylized neural/sparkle pattern */}
    <circle cx="12" cy="12" r="3" fill="url(#ai-gradient)" />
    <path 
      d="M12 2L12 6M12 18L12 22M2 12L6 12M18 12L22 12" 
      stroke="url(#ai-gradient)" 
      strokeWidth="2" 
      strokeLinecap="round"
      opacity="0.7"
    />
    <path 
      d="M5.64 5.64L8.17 8.17M15.83 15.83L18.36 18.36M5.64 18.36L8.17 15.83M15.83 8.17L18.36 5.64" 
      stroke="url(#ai-gradient)" 
      strokeWidth="1.5" 
      strokeLinecap="round"
      opacity="0.5"
    />
  </svg>
);

export default function AssistantButton() {
  const [isOpen, setIsOpen] = useState(false);
  const [isProcessing, setIsProcessing] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);
  const { user, accessToken } = useContext(AuthContext);

  // Only show for authenticated users with appropriate role
  if (!user || !['resident', 'admin'].includes(user.role)) {
    return null;
  }

  return (
    <>
      <button
        className={`assistant-button ${isProcessing ? 'processing' : ''}`}
        onClick={() => setIsOpen(!isOpen)}
        aria-label="Open AI Assistant"
      >
        <div className="assistant-button-icon">
          <AIIcon processing={isProcessing} />
        </div>
        {unreadCount > 0 && (
          <span className="assistant-badge">{unreadCount > 9 ? '9+' : unreadCount}</span>
        )}
      </button>

      <AssistantPanel
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        onProcessingChange={setIsProcessing}
        accessToken={accessToken}
      />
    </>
  );
}
```

### 6.4 Chat Panel

```jsx
// AssistantPanel.jsx
import React, { useState, useEffect, useRef, useCallback } from 'react';
import ChatMessage from './ChatMessage';
import ChatInput from './ChatInput';
import TypingIndicator from './TypingIndicator';
import './AssistantPanel.css';

const WS_URL = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws/assistant`;

export default function AssistantPanel({ isOpen, onClose, onProcessingChange, accessToken }) {
  const [messages, setMessages] = useState([]);
  const [isConnected, setIsConnected] = useState(false);
  const [isTyping, setIsTyping] = useState(false);
  const wsRef = useRef(null);
  const messagesEndRef = useRef(null);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(scrollToBottom, [messages, isTyping]);

  // WebSocket connection management
  useEffect(() => {
    if (!isOpen || !accessToken) return;

    const ws = new WebSocket(WS_URL);
    wsRef.current = ws;

    ws.onopen = () => {
      setIsConnected(true);
      // Auth is handled via cookie set by authCookie.js
    };

    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      
      switch (data.type) {
        case 'token':
          // Streaming token
          setMessages(prev => {
            const last = prev[prev.length - 1];
            if (last?.role === 'assistant' && last.streaming) {
              return [...prev.slice(0, -1), { ...last, content: last.content + data.content }];
            }
            return [...prev, { role: 'assistant', content: data.content, streaming: true }];
          });
          break;
        case 'done':
          // Stream complete
          setMessages(prev => {
            const last = prev[prev.length - 1];
            if (last?.streaming) {
              return [...prev.slice(0, -1), { ...last, streaming: false }];
            }
            return prev;
          });
          setIsTyping(false);
          onProcessingChange(false);
          break;
        case 'error':
          setMessages(prev => [...prev, { role: 'system', content: data.content, error: true }]);
          setIsTyping(false);
          onProcessingChange(false);
          break;
        case 'tool_call':
          setMessages(prev => [...prev, { role: 'tool', content: data.content, tool: data.data }]);
          break;
      }
    };

    ws.onclose = () => setIsConnected(false);
    ws.onerror = () => setIsConnected(false);

    return () => ws.close();
  }, [isOpen, accessToken, onProcessingChange]);

  const sendMessage = useCallback((content) => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
    
    setMessages(prev => [...prev, { role: 'user', content }]);
    setIsTyping(true);
    onProcessingChange(true);
    
    wsRef.current.send(JSON.stringify({ type: 'message', content }));
  }, [onProcessingChange]);

  return (
    <div className={`assistant-panel ${isOpen ? 'open' : ''}`}>
      <div className="assistant-panel-header">
        <div className="assistant-panel-title">
          <span className="ai-title-icon">✨</span>
          Homenavi AI
        </div>
        <button className="assistant-close-btn" onClick={onClose}>×</button>
      </div>

      <div className="assistant-messages">
        {messages.length === 0 && (
          <div className="assistant-welcome">
            <div className="welcome-icon">🏠</div>
            <h3>Welcome to Homenavi AI</h3>
            <p>I can help you control devices, create automations, and answer questions about your home.</p>
            <div className="quick-actions">
              <button onClick={() => sendMessage("What devices are currently on?")}>
                📱 Active devices
              </button>
              <button onClick={() => sendMessage("Create a new automation")}>
                ⚡ New automation
              </button>
              <button onClick={() => sendMessage("Show energy usage today")}>
                📊 Energy report
              </button>
            </div>
          </div>
        )}
        
        {messages.map((msg, i) => (
          <ChatMessage key={i} message={msg} />
        ))}
        
        {isTyping && <TypingIndicator />}
        <div ref={messagesEndRef} />
      </div>

      <ChatInput 
        onSend={sendMessage} 
        disabled={!isConnected || isTyping}
        placeholder={isConnected ? "Ask me anything..." : "Connecting..."}
      />
    </div>
  );
}
```

### 6.5 Panel Styles

```css
/* AssistantPanel.css */

.assistant-panel {
  position: fixed;
  bottom: 100px;
  left: 24px;
  width: 400px;
  max-width: calc(100vw - 48px);
  height: 600px;
  max-height: calc(100vh - 150px);
  
  /* Glass morphism */
  background: var(--color-modal-surface);
  backdrop-filter: blur(40px);
  -webkit-backdrop-filter: blur(40px);
  border: 1px solid var(--color-glass-border-light);
  border-radius: 24px;
  
  /* Shadow */
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.5),
    0 0 0 1px rgba(255, 255, 255, 0.05);
  
  /* Layout */
  display: flex;
  flex-direction: column;
  overflow: hidden;
  
  /* Animation */
  opacity: 0;
  transform: translateY(20px) scale(0.95);
  pointer-events: none;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  z-index: 999;
}

.assistant-panel.open {
  opacity: 1;
  transform: translateY(0) scale(1);
  pointer-events: auto;
}

.assistant-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--color-glass-border-xlight);
  background: rgba(255, 255, 255, 0.03);
}

.assistant-panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: var(--color-white);
}

.ai-title-icon {
  font-size: 20px;
}

.assistant-close-btn {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--color-glass-bg-light);
  border: 1px solid var(--color-glass-border-xlight);
  color: var(--color-white);
  font-size: 20px;
  cursor: pointer;
  transition: all 0.2s;
}

.assistant-close-btn:hover {
  background: var(--color-glass-bg);
  transform: scale(1.05);
}

.assistant-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.assistant-welcome {
  text-align: center;
  padding: 40px 20px;
  color: var(--color-secondary-light);
}

.welcome-icon {
  font-size: 48px;
  margin-bottom: 16px;
}

.assistant-welcome h3 {
  color: var(--color-white);
  margin-bottom: 8px;
}

.quick-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: center;
  margin-top: 24px;
}

.quick-actions button {
  padding: 8px 16px;
  border-radius: 20px;
  background: var(--color-glass-bg);
  border: 1px solid var(--color-glass-border-xlight);
  color: var(--color-white);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s;
}

.quick-actions button:hover {
  background: var(--color-glass-bg-strong);
  transform: translateY(-2px);
}

/* Typing indicator */
.typing-indicator {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 12px 16px;
  background: var(--color-glass-bg-light);
  border-radius: 16px;
  width: fit-content;
}

.typing-indicator span {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-success);
  animation: typing-bounce 1.4s infinite ease-in-out;
}

.typing-indicator span:nth-child(1) { animation-delay: -0.32s; }
.typing-indicator span:nth-child(2) { animation-delay: -0.16s; }

@keyframes typing-bounce {
  0%, 80%, 100% { transform: scale(0.6); opacity: 0.4; }
  40% { transform: scale(1); opacity: 1; }
}

/* Mobile responsive */
@media (max-width: 480px) {
  .assistant-panel {
    left: 8px;
    right: 8px;
    bottom: 90px;
    width: auto;
    max-width: none;
    height: calc(100vh - 120px);
    border-radius: 20px;
  }
  
  .assistant-button {
    bottom: 16px;
    left: 16px;
    width: 52px;
    height: 52px;
  }
}
```

### 6.6 Service Layer

```javascript
// services/assistantService.js
import * as http from './httpClient';

const BASE = '/api/assistant';

export async function getConversations(token) {
  return http.get(`${BASE}/conversations`, { token });
}

export async function getConversation(token, conversationId) {
  return http.get(`${BASE}/conversations/${conversationId}`, { token });
}

export async function deleteConversation(token, conversationId) {
  return http.del(`${BASE}/conversations/${conversationId}`, { token });
}

export async function createConversation(token, title) {
  return http.post(`${BASE}/conversations`, { title }, { token });
}

// Health check
export async function checkHealth() {
  return http.get(`${BASE}/health`);
}
```

### 6.7 Integration into App.jsx

```jsx
// Add to App.jsx
import AssistantButton from './components/Assistant/AssistantButton';

// Inside the App component return, add before closing </AuthProvider>:
<AssistantButton />
```

---

## 7. Phase 5: Tool Integration & Context Awareness {#7-phase-5-tool-integration}

### 7.1 System Prompt Template

Create `assistant-service/prompts/system.txt`:

```
You are Homenavi AI, an intelligent home automation assistant for the Homenavi smart home platform.

## Your Capabilities
- Control smart devices (lights, switches, thermostats, sensors)
- Query device states and history
- Create and manage automations
- Answer questions about energy usage and patterns
- Provide home security insights

## Current User Context
- User: {{.UserName}}
- Role: {{.UserRole}}
- Home: {{.HomeName}}
- Current Time: {{.CurrentTime}}
- Active Devices: {{.ActiveDeviceCount}}

## Guidelines
1. Be concise but helpful
2. When controlling devices, confirm the action
3. For dangerous operations (e.g., unlocking doors), always ask for confirmation
4. Respect the user's role permissions
5. Format responses with markdown when appropriate
6. If uncertain, ask clarifying questions

## Available Tools
{{range .Tools}}
- {{.Name}}: {{.Description}}
{{end}}

Respond naturally and helpfully. Use tools when needed to fulfill requests.
```

### 7.2 Tool Definitions

```go
// internal/tools/registry.go
package tools

type Tool struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"`
    RequiredRole string                `json:"-"` // Minimum role to use this tool
}

type ToolRegistry struct {
    tools map[string]Tool
    handlers map[string]ToolHandler
}

type ToolHandler func(ctx context.Context, params map[string]interface{}, userRole string) (interface{}, error)

func NewRegistry() *ToolRegistry {
    r := &ToolRegistry{
        tools: make(map[string]Tool),
        handlers: make(map[string]ToolHandler),
    }
    r.registerBuiltinTools()
    return r
}

func (r *ToolRegistry) registerBuiltinTools() {
    // Device control
    r.Register(Tool{
        Name: "control_device",
        Description: "Turn a device on/off or set its state",
        RequiredRole: "resident",
        Parameters: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "device_id": map[string]string{"type": "string", "description": "Device ID or friendly name"},
                "action": map[string]string{"type": "string", "enum": "on,off,toggle,set"},
                "value": map[string]string{"type": "any", "description": "Value for 'set' action"},
            },
            "required": []string{"device_id", "action"},
        },
    }, r.handleControlDevice)

    // Query devices
    r.Register(Tool{
        Name: "list_devices",
        Description: "List all devices or filter by type, room, or state",
        RequiredRole: "resident",
        Parameters: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "type": map[string]string{"type": "string", "description": "Filter by device type"},
                "room": map[string]string{"type": "string", "description": "Filter by room"},
                "state": map[string]string{"type": "string", "enum": "on,off,any"},
            },
        },
    }, r.handleListDevices)

    // History query
    r.Register(Tool{
        Name: "query_history",
        Description: "Query historical data for devices",
        RequiredRole: "resident",
        Parameters: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "device_id": map[string]string{"type": "string"},
                "metric": map[string]string{"type": "string", "description": "e.g., temperature, power, state"},
                "period": map[string]string{"type": "string", "description": "e.g., 1h, 24h, 7d, 30d"},
            },
            "required": []string{"device_id", "period"},
        },
    }, r.handleQueryHistory)

    // Create automation
    r.Register(Tool{
        Name: "create_automation",
        Description: "Create a new automation rule",
        RequiredRole: "resident",
        Parameters: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "name": map[string]string{"type": "string"},
                "trigger": map[string]string{"type": "object", "description": "Trigger configuration"},
                "actions": map[string]string{"type": "array", "description": "List of actions"},
            },
            "required": []string{"name", "trigger", "actions"},
        },
    }, r.handleCreateAutomation)
}
```

### 7.3 Role-Based Tool Access

```go
// internal/tools/access.go
package tools

import "errors"

var ErrInsufficientRole = errors.New("insufficient role for this action")

var roleHierarchy = map[string]int{
    "user":     1,
    "resident": 2,
    "admin":    3,
    "service":  4,
}

func (r *ToolRegistry) FilterByRole(userRole string) []Tool {
    userLevel := roleHierarchy[userRole]
    var allowed []Tool
    
    for _, tool := range r.tools {
        requiredLevel := roleHierarchy[tool.RequiredRole]
        if userLevel >= requiredLevel {
            allowed = append(allowed, tool)
        }
    }
    return allowed
}

func (r *ToolRegistry) CanUse(toolName, userRole string) bool {
    tool, exists := r.tools[toolName]
    if !exists {
        return false
    }
    return roleHierarchy[userRole] >= roleHierarchy[tool.RequiredRole]
}
```

---

## 8. Phase 6: Security & Authorization {#8-phase-6-security-authorization}

### 8.1 JWT Validation in Assistant Service

```go
// internal/auth/jwt.go
package auth

import (
    "crypto/rsa"
    "errors"
    "os"

    "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
    Role   string `json:"role"`
    Name   string `json:"name"`
    HomeID string `json:"home_id,omitempty"`
    jwt.RegisteredClaims
}

type Validator struct {
    publicKey *rsa.PublicKey
}

func NewValidator(keyPath string) (*Validator, error) {
    keyData, err := os.ReadFile(keyPath)
    if err != nil {
        return nil, err
    }
    
    pubKey, err := jwt.ParseRSAPublicKeyFromPEM(keyData)
    if err != nil {
        return nil, err
    }
    
    return &Validator{publicKey: pubKey}, nil
}

func (v *Validator) Validate(tokenStr string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
        return v.publicKey, nil
    })
    
    if err != nil || !token.Valid {
        return nil, errors.New("invalid token")
    }
    
    claims, ok := token.Claims.(*Claims)
    if !ok {
        return nil, errors.New("invalid claims")
    }
    
    return claims, nil
}
```

### 8.2 Request Authorization Flow

```
┌─────────────┐     ┌─────────────┐     ┌──────────────────┐     ┌─────────┐
│   Browser   │────>│ API Gateway │────>│ Assistant Service│────>│  Ollama │
└─────────────┘     └─────────────┘     └──────────────────┘     └─────────┘
       │                   │                      │
       │ JWT Token         │ Validate JWT         │
       │ (Header/Cookie)   │ Check Role ≥ resident│
       │                   │                      │
       │                   │ Forward claims       │
       │                   │ in X-User-* headers  │
       │                   │                      │
       │                   │                      │ Extract user context
       │                   │                      │ Filter tools by role
       │                   │                      │ Inject system prompt
       │                   │                      │
       │                   │                      │ Service-to-service
       │                   │                      │ calls use internal
       │                   │                      │ service token
```

### 8.3 Sensitive Operation Handling

```go
// internal/tools/sensitive.go
package tools

// SensitiveOperations require explicit user confirmation
var SensitiveOperations = map[string]bool{
    "unlock_door":       true,
    "disable_alarm":     true,
    "delete_automation": true,
    "grant_access":      true,
}

type ConfirmationRequest struct {
    Operation   string                 `json:"operation"`
    Description string                 `json:"description"`
    Params      map[string]interface{} `json:"params"`
    Token       string                 `json:"token"` // One-time confirmation token
    ExpiresAt   time.Time              `json:"expires_at"`
}

func (r *ToolRegistry) RequiresConfirmation(toolName string) bool {
    return SensitiveOperations[toolName]
}
```

---

## 9. Phase 7: Testing & Optimization {#9-phase-7-testing-optimization}

### 9.1 Testing Strategy

```
tests/
├── unit/
│   ├── llm_client_test.go      # Mock Ollama responses
│   ├── tool_registry_test.go   # Tool permission tests
│   └── auth_test.go            # JWT validation tests
├── integration/
│   ├── assistant_api_test.go   # REST endpoint tests
│   ├── websocket_test.go       # WebSocket flow tests
│   └── tool_execution_test.go  # End-to-end tool tests
└── e2e/
    └── test_assistant_chat.py  # Playwright/Selenium tests
```

### 9.2 Performance Optimization

| Optimization | Description |
|--------------|-------------|
| **Connection Pooling** | Reuse HTTP connections to Ollama |
| **Response Caching** | Cache common queries (device lists, room names) in Redis |
| **Context Truncation** | Implement sliding window for long conversations |
| **Model Preloading** | Keep model warm in Ollama to reduce first-token latency |
| **Request Queuing** | Limit concurrent LLM requests to prevent OOM |

### 9.3 Monitoring

Add to Prometheus:

```yaml
# prometheus/prometheus.yml
- job_name: 'assistant-service'
  static_configs:
    - targets: ['assistant-service:8096']
```

Metrics to track:
- `assistant_requests_total` - Request count by type
- `assistant_response_latency_seconds` - Time to first token, total time
- `assistant_tokens_generated_total` - Token usage
- `assistant_tool_calls_total` - Tool usage by type
- `ollama_model_load_time_seconds` - Model loading latency

---

## 10. Future Enhancements {#10-future-enhancements}

### 10.1 Remote Ollama Support

For offloading to a different machine:

```yaml
# .env.production
OLLAMA_HOST=http://192.168.1.100:11434  # Remote GPU server
OLLAMA_MODEL=llama3.1:70b
```

### 10.2 Multi-Model Support

```go
// Route requests to different models based on complexity
type ModelRouter struct {
    fastModel    string // Quick responses
    smartModel   string // Complex reasoning
    codeModel    string // Automation scripting
}

func (r *ModelRouter) SelectModel(query string) string {
    if isCodeRelated(query) {
        return r.codeModel
    }
    if requiresReasoning(query) {
        return r.smartModel
    }
    return r.fastModel
}
```

### 10.3 Voice Integration

- Add speech-to-text for voice commands
- Text-to-speech for responses
- Wake word detection ("Hey Homenavi")

### 10.4 MCP Migration Path

If MCP becomes more prevalent:

1. Create MCP server wrapper around existing tools
2. Expose MCP endpoint alongside REST/WS
3. Gradually migrate clients to MCP protocol
4. Maintain REST/WS for backward compatibility

---

## Implementation Timeline

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| **Phase 1** | 1 day | Ollama in docker-compose, env config |
| **Phase 2** | 3-5 days | Core service, LLM client, basic chat |
| **Phase 3** | 1 day | Gateway routes, auth integration |
| **Phase 4** | 3-4 days | Frontend button, panel, WebSocket |
| **Phase 5** | 3-4 days | Tools, context builder, ERS/DeviceHub integration |
| **Phase 6** | 1-2 days | Security hardening, role checks |
| **Phase 7** | 2-3 days | Testing, monitoring, optimization |

**Total Estimated Time**: 2-3 weeks

---

## Quick Start Commands

```bash
# 1. Start Ollama and pull model
docker compose up -d ollama
docker compose exec ollama ollama pull llama3.1:8b

# 2. Verify Ollama is working
curl http://localhost:11434/api/tags

# 3. Start all services
docker compose up -d

# 4. Test assistant endpoint
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/assistant/health
```

---

## Appendix: Hardware Comparison

| Spec | RTX 4070 SUPER (Dev) | GTX 1060 6GB (Prod) |
|------|---------------------|---------------------|
| VRAM | 12 GB | 6 GB |
| CUDA Cores | 7168 | 1280 |
| Memory Bandwidth | 504 GB/s | 192 GB/s |
| Recommended Model | llama3.1:8b | llama3.2:3b / phi3:mini |
| Expected Speed | 50-80 tok/s | 15-25 tok/s |
| Max Context | 8K-32K | 2K-4K |
