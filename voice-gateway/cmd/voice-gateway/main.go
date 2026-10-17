package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type config struct {
	ListenAddr       string
	STTURL           string
	TTSURL           string
	OpenWebUIURL     string
	OpenWebUIAPIKey  string
	OpenWebUIModel   string
	OpenWebUIToolIDs []string
	AccessToken      string
	MaxAudioBytes    int64
	LogContent       bool
	Conversations    *conversationStore
}

type conversation struct {
	messages  []map[string]string
	expiresAt time.Time
}

type conversationStore struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]conversation
}

type chatResult struct {
	text string
	err  error
}

func main() {
	cfg := config{
		ListenAddr:       value("VOICE_GATEWAY_LISTEN_ADDR", ":8090"),
		STTURL:           value("STT_URL", "http://faster-whisper:8000/v1/audio/transcriptions"),
		TTSURL:           value("TTS_URL", "http://piper:8000/tts"),
		OpenWebUIURL:     os.Getenv("OPEN_WEBUI_CHAT_URL"),
		OpenWebUIAPIKey:  os.Getenv("OPEN_WEBUI_API_KEY"),
		OpenWebUIModel:   value("OPEN_WEBUI_MODEL", "orion"),
		OpenWebUIToolIDs: values("OPEN_WEBUI_TOOL_IDS"),
		AccessToken:      os.Getenv("VOICE_GATEWAY_ACCESS_TOKEN"),
		MaxAudioBytes:    15 << 20,
		LogContent:       enabled("VOICE_GATEWAY_LOG_CONTENT"),
		Conversations:    &conversationStore{ttl: duration("VOICE_CONVERSATION_TTL", 30*time.Second), entries: map[string]conversation{}},
	}
	if cfg.OpenWebUIURL == "" {
		slog.Error("OPEN_WEBUI_CHAT_URL is required")
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /voice/v1/turns", cfg.handleTurn)
	server := &http.Server{Addr: cfg.ListenAddr, Handler: logRequests(mux)}
	slog.Info("voice gateway starting", "addr", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("voice gateway stopped", "error", err)
		os.Exit(1)
	}
}

func (cfg config) handleTurn(writer http.ResponseWriter, request *http.Request) {
	if cfg.AccessToken != "" && request.Header.Get("Authorization") != "Bearer "+cfg.AccessToken {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	if request.Header.Get("Content-Type") != "audio/wav" {
		http.Error(writer, "Content-Type must be audio/wav", http.StatusUnsupportedMediaType)
		return
	}
	audio, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, cfg.MaxAudioBytes))
	if err != nil || len(audio) == 0 {
		http.Error(writer, "invalid or empty audio request", http.StatusBadRequest)
		return
	}
	sttStarted := time.Now()
	transcript, err := cfg.transcribe(request.Context(), audio)
	if err != nil {
		serverError(writer, "speech transcription failed", err)
		return
	}
	slog.Info("voice provider timing", "provider", "stt", "duration", time.Since(sttStarted), "audio_bytes", len(audio), "transcript_chars", len(transcript))
	if transcript == "" {
		http.Error(writer, "speech was not recognized", http.StatusUnprocessableEntity)
		return
	}
	if cfg.LogContent {
		slog.Info("voice transcript", "text", transcript)
	}

	boundary := "voice-" + fmt.Sprintf("%d", time.Now().UnixNano())
	writer.Header().Set("Content-Type", "multipart/mixed; boundary="+boundary)
	writer.Header().Set("Cache-Control", "no-cache")
	writer.WriteHeader(http.StatusOK)
	stream := multipart.NewWriter(writer)
	_ = stream.SetBoundary(boundary)
	defer stream.Close()
	sessionID := strings.TrimSpace(request.Header.Get("X-Orion-Session-ID"))
	var responseText strings.Builder
	llmStarted := time.Now()
	llmFirstResponse := true
	for result := range cfg.streamClauses(request.Context(), transcript, sessionID, llmStarted) {
		if result.err != nil {
			slog.Warn("chat stream ended", "error", result.err)
			writeStreamError(stream, "chat_unavailable")
			break
		}
		sentence := result.text
		spoken := speechText(sentence)
		if spoken == "" {
			continue
		}
		if llmFirstResponse {
			slog.Info("voice provider timing", "provider", "llm", "phase", "first_completed_sentence", "duration", time.Since(llmStarted))
			llmFirstResponse = false
		}
		ttsStarted := time.Now()
		ttsResponse, err := cfg.synthesize(request.Context(), spoken)
		if err != nil {
			slog.Warn("tts failed", "error", err)
			return
		}
		part, err := stream.CreatePart(textprotoHeader(ttsResponse.Header.Get("Content-Type"), spoken, ttsResponse.Header))
		if err != nil {
			ttsResponse.Body.Close()
			return
		}
		audioBytes, err := copyAndFlush(part, ttsResponse.Body, writer, ttsStarted)
		ttsResponse.Body.Close()
		if err != nil {
			slog.Warn("tts stream failed", "error", err)
			return
		}
		slog.Info("voice provider timing", "provider", "tts", "phase", "complete_response", "duration", time.Since(ttsStarted), "text_chars", len(spoken), "audio_bytes", audioBytes)
		if cfg.LogContent {
			slog.Info("voice response sentence", "text", spoken)
		}
		responseText.WriteString(spoken)
		responseText.WriteByte(' ')
	}
	slog.Info("voice provider timing", "provider", "llm", "phase", "complete_response", "duration", time.Since(llmStarted))
	cfg.Conversations.addAssistant(sessionID, responseText.String())
}

func (cfg config) streamClauses(ctx context.Context, transcript, sessionID string, started time.Time) <-chan chatResult {
	results := make(chan chatResult, 2)
	go func() {
		defer close(results)
		for text, err := range cfg.chat(ctx, transcript, sessionID, started) {
			select {
			case results <- chatResult{text: text, err: err}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return results
}

func writeStreamError(stream *multipart.Writer, code string) {
	part, err := stream.CreatePart(textprotoHeader("application/json", ""))
	if err != nil {
		return
	}
	_, _ = part.Write([]byte(`{"error":"` + code + `"}`))
}

func (cfg config) transcribe(ctx context.Context, audio []byte) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "utterance.wav")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	_ = form.WriteField("response_format", "json")
	if err := form.Close(); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.STTURL, &body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("STT returned %s", response.Status)
	}
	var decoded struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return "", err
	}
	return strings.TrimSpace(decoded.Text), nil
}

func (cfg config) chat(ctx context.Context, transcript, sessionID string, started time.Time) func(func(string, error) bool) {
	return func(yield func(string, error) bool) {
		turnID := fmt.Sprintf("voice-%d", time.Now().UnixNano())
		userMessageID := turnID + "-user"
		assistantMessageID := turnID + "-assistant"
		messages := cfg.Conversations.messages(sessionID, transcript)
		slog.Info("voice conversation", "session_id", sessionID, "history_messages", len(messages)-2)
		chatCreateStarted := time.Now()
		chatID, err := cfg.createVoiceChat(ctx, messages, userMessageID, assistantMessageID)
		if err != nil {
			yield("", err)
			return
		}
		slog.Info("voice provider timing", "provider", "llm", "phase", "chat_create", "duration", time.Since(chatCreateStarted))
		payload := map[string]any{
			"model":                cfg.OpenWebUIModel,
			"stream":               true,
			"messages":             messages,
			"id":                   assistantMessageID,
			"chat_id":              chatID,
			"parent_id":            userMessageID,
			"assistant_message_id": assistantMessageID,
			"session_id":           turnID,
			"user_message": map[string]any{
				"id":        userMessageID,
				"role":      "user",
				"content":   transcript,
				"timestamp": time.Now().Unix(),
			},
			"params": map[string]string{"tool_approval_mode": "full"},
		}
		if len(cfg.OpenWebUIToolIDs) > 0 {
			payload["tool_ids"] = cfg.OpenWebUIToolIDs
		}
		body, err := json.Marshal(payload)
		if err != nil {
			yield("", err)
			return
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.OpenWebUIURL, bytes.NewReader(body))
		if err != nil {
			yield("", err)
			return
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "text/event-stream")
		if cfg.OpenWebUIAPIKey != "" {
			request.Header.Set("Authorization", "Bearer "+cfg.OpenWebUIAPIKey)
		}
		chatConnectionStarted := time.Now()
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			yield("", err)
			return
		}
		defer response.Body.Close()
		slog.Info("voice provider timing", "provider", "llm", "phase", "chat_connection", "duration", time.Since(chatConnectionStarted), "content_type", response.Header.Get("Content-Type"))
		if response.StatusCode != http.StatusOK {
			yield("", fmt.Errorf("Open WebUI returned %s", response.Status))
			return
		}
		mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if mediaType == "application/json" {
			data, err := io.ReadAll(response.Body)
			if err != nil {
				yield("", err)
				return
			}
			text, err := completionText(data)
			if err != nil {
				yield("", err)
				return
			}
			if text != "" {
				yield(text, nil)
				return
			}
			text, err = cfg.chatMessage(ctx, chatID, assistantMessageID)
			if err != nil {
				slog.Warn("Open WebUI completion unavailable", "error", err)
				yield("I couldn't complete that request. Please try again.", nil)
				return
			}
			if text != "" {
				yield(text, nil)
			}
			return
		}
		buffer := ""
		delivered := ""
		sentText := false
		firstTextDelta := true
		firstSSEFrame := true
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				break
			}
			if firstSSEFrame {
				slog.Info("voice provider timing", "provider", "llm", "phase", "first_sse_frame", "duration", time.Since(started))
				firstSSEFrame = false
			}
			delta, eventType, ok := streamDelta([]byte(data))
			if !ok {
				continue
			}
			if cfg.LogContent && eventType != "" {
				slog.Info("Open WebUI stream event", "type", eventType)
			}
			if delta == "" {
				continue
			}
			if firstTextDelta {
				slog.Info("voice provider timing", "provider", "llm", "phase", "first_text_delta", "duration", time.Since(started))
				firstTextDelta = false
			}
			buffer += delta
			for {
				sentence, rest := nextClause(buffer)
				if sentence == "" {
					break
				}
				buffer = rest
				sentText = true
				delivered = appendDeliveredText(delivered, sentence)
				if !yield(sentence, nil) {
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			yield("", err)
			return
		}
		if final := strings.TrimSpace(buffer); final != "" {
			if !strings.HasSuffix(final, ".") && !strings.HasSuffix(final, "!") && !strings.HasSuffix(final, "?") {
				if recovered := cfg.recoverIncompleteFinal(ctx, chatID, assistantMessageID, delivered, final); recovered != "" {
					final = recovered
				}
			}
			sentText = true
			yield(final, nil)
		}
		if !sentText {
			text, err := cfg.chatMessage(ctx, chatID, assistantMessageID)
			if err != nil {
				slog.Warn("Open WebUI completion unavailable", "error", err)
				yield("I couldn't complete that request. Please try again.", nil)
				return
			}
			if text != "" {
				yield(text, nil)
			}
		}
	}
}

func (store *conversationStore) messages(sessionID, transcript string) []map[string]string {
	store.mu.Lock()
	defer store.mu.Unlock()
	messages := []map[string]string{{"role": "system", "content": "Respond for a voice assistant. Use concise plain spoken text only. Do not use Markdown, lists, headings, code formatting, links, or emoji."}}
	if sessionID == "" {
		return append(messages, map[string]string{"role": "user", "content": transcript})
	}
	now := time.Now()
	for id, entry := range store.entries {
		if !now.Before(entry.expiresAt) {
			delete(store.entries, id)
		}
	}
	history := []map[string]string{}
	if prior, ok := store.entries[sessionID]; ok {
		history = append(history, prior.messages...)
	}
	user := map[string]string{"role": "user", "content": transcript}
	messages = append(messages, history...)
	messages = append(messages, user)
	store.entries[sessionID] = conversation{messages: append(history, user), expiresAt: now.Add(store.ttl)}
	return messages
}

func (store *conversationStore) addAssistant(sessionID, text string) {
	if sessionID == "" || strings.TrimSpace(text) == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, ok := store.entries[sessionID]
	if !ok || !time.Now().Before(entry.expiresAt) {
		delete(store.entries, sessionID)
		return
	}
	entry.messages = append(entry.messages, map[string]string{"role": "assistant", "content": strings.TrimSpace(text)})
	entry.expiresAt = time.Now().Add(store.ttl)
	store.entries[sessionID] = entry
}

func (cfg config) createVoiceChat(ctx context.Context, messages []map[string]string, userMessageID, assistantMessageID string) (string, error) {
	historyMessages := voiceChatHistory(messages, userMessageID, assistantMessageID, cfg.OpenWebUIModel)
	chat := map[string]any{
		"title":   "Orion Voice",
		"models":  []string{cfg.OpenWebUIModel},
		"params":  map[string]string{"tool_approval_mode": "full"},
		"history": map[string]any{"currentId": assistantMessageID, "messages": historyMessages},
	}
	body, err := json.Marshal(map[string]any{"chat": chat})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.openWebUIBaseURL()+"/api/v1/chats/new", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	cfg.authorize(request)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Open WebUI chat creation returned %s", response.Status)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", errors.New("Open WebUI chat creation returned no ID")
	}
	return created.ID, nil
}

func voiceChatHistory(messages []map[string]string, userMessageID, assistantMessageID, model string) map[string]any {
	history := map[string]any{}
	parentID := ""
	messageNumber := 0
	conversationMessages := 0
	for _, message := range messages {
		if message["role"] != "system" {
			conversationMessages++
		}
	}
	for _, message := range messages {
		if message["role"] == "system" {
			continue
		}
		messageNumber++
		messageID := fmt.Sprintf("%s-history-%d", userMessageID, messageNumber)
		if messageNumber == conversationMessages {
			messageID = userMessageID
		}
		node := map[string]any{"id": messageID, "role": message["role"], "content": message["content"], "childrenIds": []string{}}
		if parentID != "" {
			node["parentId"] = parentID
			parent := history[parentID].(map[string]any)
			parent["childrenIds"] = []string{messageID}
		}
		history[messageID] = node
		parentID = messageID
	}
	history[assistantMessageID] = map[string]any{"id": assistantMessageID, "role": "assistant", "content": "", "parentId": userMessageID, "childrenIds": []string{}, "done": false, "model": model}
	if user, ok := history[userMessageID].(map[string]any); ok {
		user["childrenIds"] = []string{assistantMessageID}
	}
	return history
}

func (cfg config) chatMessage(ctx context.Context, chatID, messageID string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 60; attempt++ {
		text, err := cfg.chatMessageOnce(ctx, chatID, messageID)
		if err == nil && text != "" {
			return text, nil
		}
		lastErr = err
		if attempt == 59 {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return "", lastErr
}

func (cfg config) recoverIncompleteFinal(ctx context.Context, chatID, messageID, delivered, final string) string {
	for attempt := 0; attempt < 5; attempt++ {
		text, err := cfg.chatMessageOnce(ctx, chatID, messageID)
		if err == nil && strings.HasPrefix(text, delivered) {
			if remaining := strings.TrimSpace(strings.TrimPrefix(text, delivered)); len(remaining) > len(final) {
				return remaining
			}
		}
		if attempt == 4 {
			break
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(200 * time.Millisecond):
		}
	}
	return ""
}

func (cfg config) chatMessageOnce(ctx context.Context, chatID, messageID string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.openWebUIBaseURL()+"/api/v1/chats/"+chatID, nil)
	if err != nil {
		return "", err
	}
	cfg.authorize(request)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Open WebUI chat lookup returned %s", response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if content, ok := assistantMessageContent(data, messageID); ok {
		return content, nil
	}
	return "", fmt.Errorf("Open WebUI chat %s has no assistant message %s", chatID, messageID)
}

func assistantMessageContent(data []byte, messageID string) (string, bool) {
	var envelope struct {
		Chat json.RawMessage `json:"chat"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", false
	}
	for _, candidate := range []json.RawMessage{data, envelope.Chat} {
		if len(candidate) == 0 {
			continue
		}
		var chat struct {
			History struct {
				Messages map[string]struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			} `json:"history"`
		}
		if err := json.Unmarshal(candidate, &chat); err != nil {
			continue
		}
		if message, ok := chat.History.Messages[messageID]; ok {
			if message.Role == "assistant" {
				if content := strings.TrimSpace(message.Content); content != "" {
					return content, true
				}
			}
		}
	}
	return "", false
}

func (cfg config) openWebUIBaseURL() string {
	return strings.TrimSuffix(cfg.OpenWebUIURL, "/api/chat/completions")
}

func (cfg config) authorize(request *http.Request) {
	if cfg.OpenWebUIAPIKey != "" {
		request.Header.Set("Authorization", "Bearer "+cfg.OpenWebUIAPIKey)
	}
}

func (cfg config) synthesize(ctx context.Context, text string) (*http.Response, error) {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TTSURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("TTS returned %s", response.Status)
	}
	return response, nil
}

func copyAndFlush(destination io.Writer, source io.Reader, responseWriter http.ResponseWriter, started time.Time) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	firstAudioByte := true
	for {
		read, readErr := source.Read(buffer)
		if read > 0 {
			if firstAudioByte {
				slog.Info("voice provider timing", "provider", "tts", "phase", "first_audio_byte", "duration", time.Since(started))
				firstAudioByte = false
			}
			bytesWritten, writeErr := destination.Write(buffer[:read])
			written += int64(bytesWritten)
			if writeErr != nil {
				return written, writeErr
			}
			if flusher, ok := responseWriter.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func nextClause(text string) (string, string) {
	for index, runeValue := range text {
		if strings.ContainsRune(".!?", runeValue) || (strings.ContainsRune(",;:", runeValue) && len(strings.TrimSpace(text[:index+1])) >= 24) {
			return strings.TrimSpace(text[:index+1]), strings.TrimSpace(text[index+1:])
		}
	}
	return "", text
}

func appendDeliveredText(delivered, sentence string) string {
	if delivered == "" {
		return sentence
	}
	return delivered + " " + sentence
}

func streamDelta(data []byte) (string, string, bool) {
	var event struct {
		Type    string          `json:"type"`
		Delta   json.RawMessage `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Data    json.RawMessage `json:"data"`
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return "", "", false
	}
	if len(event.Choices) > 0 && event.Choices[0].Delta.Content != "" {
		return event.Choices[0].Delta.Content, event.Type, true
	}
	if text := jsonText(event.Delta); text != "" {
		return text, event.Type, true
	}
	if event.Message.Content != "" {
		return event.Message.Content, event.Type, true
	}
	if text := jsonText(event.Data); text != "" {
		return text, event.Type, true
	}
	return "", event.Type, false
}

func jsonText(data json.RawMessage) string {
	if len(data) == 0 || string(data) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(data, &text) == nil {
		return text
	}
	var value struct {
		Content string `json:"content"`
		Text    string `json:"text"`
		Delta   string `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	for _, text := range []string{value.Content, value.Text, value.Delta, value.Message.Content} {
		if text != "" {
			return text
		}
	}
	return ""
}

func speechText(text string) string {
	text = markdownLink.ReplaceAllString(text, "$1")
	text = markdownPrefix.ReplaceAllString(text, "")
	text = strings.NewReplacer("**", "", "__", "", "`", "", "#", "", "\n", " ").Replace(text)
	return strings.Join(strings.Fields(text), " ")
}

var (
	markdownLink   = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	markdownPrefix = regexp.MustCompile(`(?m)(^|\s)(?:[-*+]\s+|\d+[.)]\s+)`)
)

func textprotoHeader(contentType, text string, sourceHeaders ...http.Header) map[string][]string {
	header := map[string][]string{"Content-Type": {contentType}, "X-Voice-Text": {text}}
	if len(sourceHeaders) == 0 {
		return header
	}
	for _, name := range []string{"X-Audio-Sample-Rate", "X-Audio-Sample-Width", "X-Audio-Channels"} {
		if value := sourceHeaders[0].Get(name); value != "" {
			header[name] = []string{value}
		}
	}
	return header
}

func value(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key)))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func enabled(key string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes"
}

func values(key string) []string {
	var values []string
	for _, value := range strings.Split(os.Getenv(key), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func completionText(data []byte) (string, error) {
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &completion); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, choice := range completion.Choices {
		text.WriteString(choice.Message.Content)
	}
	for _, output := range completion.Output {
		for _, content := range output.Content {
			text.WriteString(content.Text)
		}
	}
	return strings.TrimSpace(text.String()), nil
}

func serverError(writer http.ResponseWriter, message string, err error) {
	slog.Warn(message, "error", err)
	http.Error(writer, message, http.StatusBadGateway)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		next.ServeHTTP(writer, request)
		slog.Info("voice request", "method", request.Method, "path", request.URL.Path, "duration", time.Since(started))
	})
}

var _ = mime.FormatMediaType
