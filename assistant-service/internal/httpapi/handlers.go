package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/homenavi/assistant-service/internal/config"
	"github.com/homenavi/assistant-service/internal/llm"
	"github.com/homenavi/assistant-service/internal/repository"
	"github.com/homenavi/assistant-service/internal/tools"
)

type Handler struct {
	config           *config.Config
	llmClient        *llm.OllamaClient
	toolRegistry     *tools.Registry
	conversationRepo *repository.ConversationRepository
	messageRepo      *repository.MessageRepository
}

func NewHandler(cfg *config.Config, llmClient *llm.OllamaClient, toolRegistry *tools.Registry, conversationRepo *repository.ConversationRepository, messageRepo *repository.MessageRepository) *Handler {
	return &Handler{
		config:           cfg,
		llmClient:        llmClient,
		toolRegistry:     toolRegistry,
		conversationRepo: conversationRepo,
		messageRepo:      messageRepo,
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{"error": message})
}

func parseIntQuery(r *http.Request, key string, def int) int {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	available := false
	if h.llmClient != nil {
		ok, _ := h.llmClient.Available(ctx)
		available = ok
	}

	model := ""
	if h.config != nil {
		model = h.config.OllamaModel
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "ok",
		"service":          "assistant-service",
		"model":            model,
		"ollama_available": available,
	})
}

type CreateConversationRequest struct {
	Title string `json:"title"`
}

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.conversationRepo == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "conversation store not configured")
		return
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user")
		return
	}

	limit := parseIntQuery(r, "limit", 50)
	offset := parseIntQuery(r, "offset", 0)

	conversations, err := h.conversationRepo.ListByUser(r.Context(), userID, limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list conversations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"conversations": conversations})
}

func (h *Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.conversationRepo == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "conversation store not configured")
		return
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user")
		return
	}

	var req CreateConversationRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if strings.TrimSpace(req.Title) == "" {
		req.Title = "New conversation"
	}

	conv, err := h.conversationRepo.Create(r.Context(), userID, req.Title)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create conversation")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"conversation": conv})
}

func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.conversationRepo == nil || h.messageRepo == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "conversation store not configured")
		return
	}

	convID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	conv, err := h.conversationRepo.GetByID(r.Context(), convID)
	if err != nil || conv == nil {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}

	userID, _ := uuid.Parse(claims.Subject)
	if conv.UserID != userID && claims.Role != "admin" {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	messages, err := h.messageRepo.ListByConversation(r.Context(), convID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"conversation": conv,
		"messages":     messages,
	})
}

func (h *Handler) UpdateConversation(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.conversationRepo == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "conversation store not configured")
		return
	}

	convID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	conv, err := h.conversationRepo.GetByID(r.Context(), convID)
	if err != nil || conv == nil {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}

	userID, _ := uuid.Parse(claims.Subject)
	if conv.UserID != userID && claims.Role != "admin" {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req CreateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.conversationRepo.UpdateTitle(r.Context(), convID, req.Title); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update conversation")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.conversationRepo == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "conversation store not configured")
		return
	}

	convID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid conversation ID")
		return
	}

	conv, err := h.conversationRepo.GetByID(r.Context(), convID)
	if err != nil || conv == nil {
		writeJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}

	userID, _ := uuid.Parse(claims.Subject)
	if conv.UserID != userID && claims.Role != "admin" {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := h.conversationRepo.Delete(r.Context(), convID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete conversation")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ListDevicesMerged(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.toolRegistry == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "tool registry not configured")
		return
	}

	params := map[string]interface{}{}
	if v := strings.TrimSpace(r.URL.Query().Get("type")); v != "" {
		params["type"] = v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("room")); v != "" {
		params["room"] = v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("state")); v != "" {
		params["state"] = v
	}

	res, err := h.toolRegistry.Execute(r.Context(), "list_devices", params, claims.Role, claims.Subject)
	if err != nil {
		writeJSONError(w, http.StatusForbidden, err.Error())
		return
	}
	if res == nil || !res.Success {
		writeJSONError(w, http.StatusBadGateway, "failed to list devices")
		return
	}

	writeJSON(w, http.StatusOK, res.Data)
}

func (h *Handler) ListRooms(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.toolRegistry == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "tool registry not configured")
		return
	}

	res, err := h.toolRegistry.Execute(r.Context(), "list_rooms", map[string]interface{}{}, claims.Role, claims.Subject)
	if err != nil {
		writeJSONError(w, http.StatusForbidden, err.Error())
		return
	}
	if res == nil || !res.Success {
		writeJSONError(w, http.StatusBadGateway, "failed to list rooms")
		return
	}

	writeJSON(w, http.StatusOK, res.Data)
}

func (h *Handler) AdminStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	available := false
	if h.llmClient != nil {
		ok, _ := h.llmClient.Available(ctx)
		available = ok
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"service":          "assistant-service",
		"ollama_host":      h.config.OllamaHost,
		"ollama_model":     h.config.OllamaModel,
		"ollama_available": available,
		"context_length":   h.config.OllamaContextLength,
		"max_tokens":       h.config.MaxTokens,
	})
}

func (h *Handler) AdminListModels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	models, err := h.llmClient.Models(ctx)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list models: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"models":        models,
		"current_model": h.llmClient.GetModel(),
	})
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type WSMessage struct {
	Type    string          `json:"type"`
	Content string          `json:"content,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r.Context())
	if claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	userID, _ := uuid.Parse(claims.Subject)
	session := &ChatSession{
		conn:     conn,
		userID:   userID,
		userName: claims.Name,
		userRole: claims.Role,
		handler:  h,
	}

	session.run()
}

type ChatSession struct {
	conn           *websocket.Conn
	userID         uuid.UUID
	userName       string
	userRole       string
	conversationID *uuid.UUID
	lastDeviceID   string
	handler        *Handler
	writeMu        sync.Mutex
	cancelFunc     context.CancelFunc
}

func (s *ChatSession) run() {
	for {
		_, msgBytes, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			s.sendError("Invalid message format")
			continue
		}

		switch msg.Type {
		case "message":
			s.handleMessage(msg.Content)
		case "cancel":
			if s.cancelFunc != nil {
				s.cancelFunc()
			}
		case "new_conversation":
			s.conversationID = nil
			s.send(WSMessage{Type: "conversation_cleared"})
		}
	}
}

func (s *ChatSession) handleMessage(content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	if s.handler == nil || s.handler.llmClient == nil || s.handler.toolRegistry == nil {
		s.sendError("assistant not configured")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	s.cancelFunc = cancel
	defer cancel()

	if s.conversationID == nil {
		if s.handler.conversationRepo == nil {
			s.sendError("conversation store not configured")
			return
		}
		conv, err := s.handler.conversationRepo.Create(ctx, s.userID, generateTitle(content))
		if err != nil {
			s.sendError("Failed to create conversation")
			return
		}
		s.conversationID = &conv.ID
		s.send(WSMessage{Type: "conversation_created", Content: conv.ID.String()})
	}

	if s.handler.messageRepo != nil {
		_, err := s.handler.messageRepo.Create(ctx, *s.conversationID, "user", content, nil, nil)
		if err != nil {
			log.Printf("Failed to save user message: %v", err)
		}
	}

	history := []*repository.Message{}
	if s.handler.messageRepo != nil {
		msgs, err := s.handler.messageRepo.GetLastN(ctx, *s.conversationID, 20)
		if err != nil {
			log.Printf("Failed to get history: %v", err)
		} else {
			history = msgs
		}
	}

	s.send(WSMessage{Type: "typing"})

	snapshotJSON := s.buildLiveSnapshot(ctx)
	snapshotAvailable := strings.TrimSpace(snapshotJSON) != ""
	messages := s.buildMessages(history, content, snapshotJSON)

	toolDefs := s.handler.toolRegistry.GetAllToolDefinitions()
	toolsDefs := make([]llm.ToolDefinition, 0, len(toolDefs))
	for _, td := range toolDefs {
		fnAny, ok := td["function"]
		if !ok {
			continue
		}
		fn, ok := fnAny.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]interface{})
		if name == "" {
			continue
		}
		toolsDefs = append(toolsDefs, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}

	maxToolCalls := 5
	isAction := s.messageLikelyIsAction(content)
	qLower := strings.ToLower(strings.TrimSpace(content))
	isRoomMetricQuery := strings.Contains(qLower, "temperature") || strings.Contains(qLower, "humidity")
	isDeviceListQuery := (strings.Contains(qLower, "list") || strings.Contains(qLower, "show")) && strings.Contains(qLower, "device")
	controlAttempted := false
	controlSucceeded := false
	lastControlErr := ""
	for i := 0; i < maxToolCalls; i++ {
		var fullResponse strings.Builder
		var toolCalls []llm.ToolCall

		err := s.handler.llmClient.ChatWithTools(ctx, messages, toolsDefs, func(token string, calls []llm.ToolCall, done bool) {
			if token != "" {
				fullResponse.WriteString(token)
			}
			if len(calls) > 0 {
				toolCalls = calls
			}
		})
		if err != nil {
			if ctx.Err() == context.Canceled {
				s.send(WSMessage{Type: "cancelled"})
			} else {
				s.sendError("Failed to generate response: " + err.Error())
			}
			return
		}

		assistantText := fullResponse.String()
		containsToolCallText := strings.Contains(strings.ToLower(assistantText), "<tool_call>")

		if len(toolCalls) == 0 {
			requiresToolCall := s.messageLikelyNeedsTools(content)
			// If we already provided a live snapshot, allow grounded, non-action answers without requiring tool calls.
			// We still require tool calls for actions (control_device) and for room metrics (temperature/humidity).
			if snapshotAvailable && !isAction && !isRoomMetricQuery {
				requiresToolCall = false
			}
			// Device listing should always be done via list_devices to avoid stale/partial snapshot output.
			if isDeviceListQuery {
				requiresToolCall = true
			}
			// If the model emits '<tool_call>' text, treat it as a tool-required interaction and retry.
			if containsToolCallText {
				requiresToolCall = true
			}

			// If the user is asking for a live action/state answer, do not allow an ungrounded response.
			if requiresToolCall {
				// No direct fallback. Instead, retry with explicit tool guidance.
				if i == 0 {
					messages = append(messages, llm.Message{Role: "system", Content: s.buildToolForcingInstruction(content)})
					continue
				}
				if i == 1 {
					messages = append(messages, llm.Message{Role: "system", Content: "Do NOT output '<tool_call>' blocks or JSON as plain text. Use the tool calling mechanism. Your next response MUST be a tool call (no narrative). If unsure which device/room, call find_devices or list_rooms first."})
					continue
				}
				s.sendAssistantAndPersist(ctx, "I couldn't verify or execute that right now. Please try again.")
				return
			}
			if isAction && controlAttempted && !controlSucceeded {
				msg := "I tried, but I couldn't confirm that change on the device."
				if strings.TrimSpace(lastControlErr) != "" {
					msg = msg + " " + strings.TrimSpace(lastControlErr)
				}
				s.sendAssistantAndPersist(ctx, msg)
				return
			}
			s.sendAssistantAndPersist(ctx, assistantText)
			return
		}

		s.send(WSMessage{Type: "typing"})
		for _, tc := range toolCalls {
			params := make(map[string]interface{})
			if len(tc.Function.Arguments) > 0 {
				_ = json.Unmarshal(tc.Function.Arguments, &params)
			}

			// Capture "it" context by remembering the last explicitly referenced device_id.
			if did, ok := params["device_id"].(string); ok {
				did = strings.TrimSpace(did)
				if did != "" && (tc.Function.Name == "control_device" || tc.Function.Name == "get_device_state") {
					s.lastDeviceID = did
				}
			}

			result, execErr := s.handler.toolRegistry.Execute(ctx, tc.Function.Name, params, s.userRole, s.userID.String())

			if tc.Function.Name == "control_device" {
				controlAttempted = true
				if execErr != nil {
					lastControlErr = execErr.Error()
				} else if result != nil {
					if result.Success {
						controlSucceeded = true
					} else {
						lastControlErr = result.Error
					}
				}
			}

			toolMsg := map[string]interface{}{
				"tool": tc.Function.Name,
			}
			if execErr != nil {
				toolMsg["success"] = false
				toolMsg["error"] = execErr.Error()
			} else if result != nil {
				toolMsg["success"] = result.Success
				if result.Success {
					toolMsg["data"] = result.Data
				} else {
					toolMsg["error"] = result.Error
					if result.Data != nil {
						toolMsg["data"] = result.Data
					}
				}
			} else {
				toolMsg["success"] = false
				toolMsg["error"] = "tool returned nil result"
			}
			b, _ := json.Marshal(toolMsg)
			toolResultContent := string(b)

			messages = append(messages, llm.Message{Role: "tool", Content: toolResultContent})
		}

		// If this was an action and we attempted control but it didn't succeed,
		// be explicit for the model before next iteration.
		if isAction && controlAttempted && !controlSucceeded {
			messages = append(messages, llm.Message{Role: "system", Content: "The last control_device attempt did NOT succeed or could not be confirmed. You MUST NOT claim the action succeeded."})
		}
	}

	s.send(WSMessage{Type: "done"})
}

func (s *ChatSession) buildToolForcingInstruction(userText string) string {
	q := strings.ToLower(strings.TrimSpace(userText))
	if q == "" {
		return "You MUST call at least one tool before answering."
	}

	// Temperature/humidity questions
	if strings.Contains(q, "temperature") || strings.Contains(q, "humidity") {
		if strings.Contains(q, "humidity") {
			return "You MUST call get_room_metric(room, metric) before answering, with metric='humidity'. Use the room mentioned by the user (name or slug). If the room is ambiguous, call list_rooms first."
		}
		return "You MUST call get_room_metric(room, metric) before answering, with metric='temperature'. Use the room mentioned by the user (name or slug). If the room is ambiguous, call list_rooms first."
	}

	// Device list
	if (strings.Contains(q, "list") || strings.Contains(q, "show")) && strings.Contains(q, "device") {
		return "You MUST call list_devices before answering. Then summarize devices with name, room, and state."
	}

	// Light control / device control
	if strings.Contains(q, "turn") || strings.Contains(q, "switch") || strings.Contains(q, "set") || strings.Contains(q, "make") || strings.Contains(q, "toggle") || strings.Contains(q, "dim") || strings.Contains(q, "bright") {
		return "You MUST control devices using tools. Steps: (1) If you don't know the exact device_id, call find_devices(query, room). (2) Call control_device(device_id, state). (3) Only then confirm the outcome."
	}

	return "You MUST call at least one tool before answering. If unsure which, start with list_devices or list_rooms."
}

func (s *ChatSession) sendAssistantAndPersist(ctx context.Context, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		s.send(WSMessage{Type: "done"})
		return
	}

	s.send(WSMessage{Type: "token", Content: text})
	if s.handler != nil && s.handler.messageRepo != nil && s.conversationID != nil {
		_, err := s.handler.messageRepo.Create(ctx, *s.conversationID, "assistant", text, nil, nil)
		if err != nil {
			log.Printf("Failed to save assistant message: %v", err)
		}
	}
	if s.handler != nil && s.handler.conversationRepo != nil && s.conversationID != nil {
		_ = s.handler.conversationRepo.Touch(ctx, *s.conversationID)
	}
	s.send(WSMessage{Type: "done"})
}

func (s *ChatSession) buildMessages(history []*repository.Message, currentMessage string, liveSnapshotJSON string) []llm.Message {
	messages := []llm.Message{{Role: "system", Content: s.buildSystemPrompt()}}
	if strings.TrimSpace(liveSnapshotJSON) != "" {
		messages = append(messages, llm.Message{Role: "system", Content: "LIVE_HOME_SNAPSHOT_JSON: " + liveSnapshotJSON})
	}
	if strings.TrimSpace(s.lastDeviceID) != "" {
		messages = append(messages, llm.Message{Role: "system", Content: "LAST_REFERENCED_DEVICE_ID: " + strings.TrimSpace(s.lastDeviceID)})
	}

	for _, msg := range history {
		if msg.Role == "user" && msg.Content == currentMessage {
			continue
		}
		messages = append(messages, llm.Message{Role: msg.Role, Content: msg.Content})
	}

	messages = append(messages, llm.Message{Role: "user", Content: currentMessage})
	return messages
}

func (s *ChatSession) buildSystemPrompt() string {
	return fmt.Sprintf(`You are Homenavi AI, an intelligent home automation assistant for the Homenavi smart home platform.

## User Visibility
The user cannot see LIVE_HOME_SNAPSHOT_JSON or any tool outputs.
When you answer, you MUST include the relevant facts you used (e.g., device name, room, current state) and clearly state which device you targeted for actions.
Do NOT say "based on the snapshot" or refer to hidden JSON; just present the facts plainly.

## CRITICAL: Always Use Tools for Real Data
You have access to tools that fetch REAL data from the smart home system. You MUST use these tools (and/or the live snapshot) to get actual device information.
NEVER make up or hallucinate device data, IDs, states, or any other information.

## CRITICAL: Do Not Mention Tools
Do NOT mention tool names, function calls, or internal steps. Do NOT say things like "I'll call control_device" or "Tool result:".
Only communicate user-relevant outcomes (what you observed, what you changed, or what failed to confirm).

Do NOT output tool-call markup like "<tool_call>" or any tool-call JSON. If you need tools, use the tool-calling mechanism.

If an action or query cannot be performed due to permissions, explain clearly that the current account needs the "resident" role (or "admin") to do that.

When a user asks about devices, rooms, automations, or system status:
1. Use the live snapshot and/or call the appropriate tool first
2. Wait for the tool result
3. Then respond based on ACTUAL data

## Current Context
- User: %s
- Role: %s
- Time: %s
- Timezone: %s

## IMPORTANT: Device IDs
Device IDs look like "zigbee/0xABCDEF123456" - they include a protocol prefix and a hex address.
When using get_device_state or control_device, you MUST use the EXACT device_id from the live snapshot or list_devices.
NEVER invent or guess device IDs.

## Live Snapshot
Each user message includes a LIVE_HOME_SNAPSHOT_JSON system message with a compact, real-time view of rooms and devices (from ERS+HDP).
Use it to ground answers and to find the correct device_id.
If you need to search, call find_devices(query, room).
If you need details for one device, call get_device_state(device_id).
If the user asks you to change something (turn on/off, set brightness/color, etc.), you MUST call control_device and wait for its result before you say it happened.

## Pronouns ("it", "that")
If the user refers to "it" or "that", and a LAST_REFERENCED_DEVICE_ID is available, treat it as the target device_id unless the user indicates otherwise.

## Room Questions (Temperature/Humidity)
If the user asks for temperature or humidity "in a room", prefer calling get_room_metric(room, metric).

## Guidelines
1. Be concise but helpful
2. Never invent device names/IDs/state
3. For actions, call tools and reflect results
4. For dangerous operations (e.g., unlocking doors), ask for confirmation first
5. Use markdown when appropriate
6. If a tool fails, explain briefly and suggest a next step
`, s.userName, s.userRole, time.Now().Local().Format("2006-01-02 15:04:05"), time.Now().Local().Location().String())
}

func (s *ChatSession) messageLikelyNeedsTools(userText string) bool {
	q := strings.ToLower(strings.TrimSpace(userText))
	if q == "" {
		return false
	}
	// Any device/room/status question should be grounded in live data.
	deviceHints := []string{"device", "devices", "room", "rooms", "lamp", "light", "lights", "switch", "sensor", "thermostat", "plug", "outlet", "status", "on", "off", "brightness", "color", "rgb"}
	for _, h := range deviceHints {
		if strings.Contains(q, h) {
			return true
		}
	}
	actionHints := []string{"turn ", "switch ", "set ", "make ", "dim ", "bright", "toggle", "open ", "close ", "lock", "unlock"}
	for _, h := range actionHints {
		if strings.Contains(q, h) {
			return true
		}
	}
	if strings.Contains(q, "temperature") || strings.Contains(q, "humidity") {
		return true
	}
	return false
}

func (s *ChatSession) messageLikelyIsAction(userText string) bool {
	q := strings.ToLower(strings.TrimSpace(userText))
	if q == "" {
		return false
	}
	actionHints := []string{"turn ", "switch ", "set ", "make ", "dim ", "bright", "toggle", "open ", "close ", "lock", "unlock"}
	for _, h := range actionHints {
		if strings.Contains(q, h) {
			return true
		}
	}
	return false
}

func (s *ChatSession) buildLiveSnapshot(ctx context.Context) string {
	if s.handler == nil || s.handler.toolRegistry == nil {
		return ""
	}

	roomsRes, _ := s.handler.toolRegistry.Execute(ctx, "list_rooms", map[string]interface{}{}, s.userRole, s.userID.String())
	devsRes, _ := s.handler.toolRegistry.Execute(ctx, "list_devices", map[string]interface{}{}, s.userRole, s.userID.String())

	rooms := make([]map[string]interface{}, 0)
	if roomsRes != nil && roomsRes.Success {
		switch v := roomsRes.Data.(type) {
		case []map[string]interface{}:
			rooms = v
		case []interface{}:
			for _, it := range v {
				if m, ok := it.(map[string]interface{}); ok {
					rooms = append(rooms, m)
				}
			}
		}
	}

	devicesRaw := make([]map[string]interface{}, 0)
	if devsRes != nil && devsRes.Success {
		switch v := devsRes.Data.(type) {
		case []map[string]interface{}:
			devicesRaw = v
		case []interface{}:
			for _, it := range v {
				if m, ok := it.(map[string]interface{}); ok {
					devicesRaw = append(devicesRaw, m)
				}
			}
		}
	}

	sort.Slice(rooms, func(i, j int) bool {
		return strings.ToLower(fmt.Sprintf("%v", rooms[i]["name"])) < strings.ToLower(fmt.Sprintf("%v", rooms[j]["name"]))
	})
	sort.Slice(devicesRaw, func(i, j int) bool {
		ri := strings.ToLower(fmt.Sprintf("%v", devicesRaw[i]["room"]))
		rj := strings.ToLower(fmt.Sprintf("%v", devicesRaw[j]["room"]))
		if ri != rj {
			return ri < rj
		}
		ni := strings.ToLower(fmt.Sprintf("%v", devicesRaw[i]["name"]))
		nj := strings.ToLower(fmt.Sprintf("%v", devicesRaw[j]["name"]))
		if ni != nj {
			return ni < nj
		}
		return strings.ToLower(fmt.Sprintf("%v", devicesRaw[i]["device_id"])) < strings.ToLower(fmt.Sprintf("%v", devicesRaw[j]["device_id"]))
	})

	maxDevices := 60
	truncated := false
	if len(devicesRaw) > maxDevices {
		devicesRaw = devicesRaw[:maxDevices]
		truncated = true
	}

	devices := make([]map[string]interface{}, 0, len(devicesRaw))
	for _, d := range devicesRaw {
		item := map[string]interface{}{
			"device_id":   d["device_id"],
			"name":        d["name"],
			"description": d["description"],
			"type":        d["type"],
			"room":        d["room"],
			"room_slug":   d["room_slug"],
			"online":      d["online"],
			"inputs":      d["inputs"],
		}
		if st, ok := d["state"].(map[string]interface{}); ok {
			item["state"] = compactState(st)
		}
		devices = append(devices, item)
	}

	snapshot := map[string]interface{}{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"rooms":        rooms,
		"devices":      devices,
		"truncated":    truncated,
	}
	buf, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	return string(buf)
}

func compactState(state map[string]interface{}) map[string]interface{} {
	keep := []string{"state", "brightness", "color", "color_temp", "temperature", "humidity", "power", "battery", "occupancy", "contact"}
	out := make(map[string]interface{})
	for _, k := range keep {
		if v, ok := state[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		keys := make([]string, 0, len(state))
		for k := range state {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i := 0; i < len(keys) && i < 6; i++ {
			out[keys[i]] = state[keys[i]]
		}
	}
	return out
}

func (s *ChatSession) send(msg WSMessage) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.WriteJSON(msg)
}

func (s *ChatSession) sendError(message string) {
	s.send(WSMessage{Type: "error", Content: message})
}

func generateTitle(content string) string {
	title := strings.TrimSpace(content)
	if len(title) > 50 {
		title = title[:47] + "..."
	}
	if title == "" {
		return "New conversation"
	}
	return title
}
