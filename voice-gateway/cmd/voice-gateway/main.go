package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type config struct {
	ListenAddr           string
	STTURL               string
	TTSURL               string
	OpenWebUIURL         string
	OpenWebUIAPIKey      string
	OpenWebUISocketToken string
	OpenWebUIModel       string
	OpenWebUIToolIDs     []string
	AccessToken          string
	MaxAudioBytes        int64
	LogContent           bool
	Conversations        *conversationStore
}

type conversation struct {
	messages  []map[string]string
	expiresAt time.Time
	updatedAt time.Time
}

type conversationStore struct {
	mu      sync.Mutex
	ttl     time.Duration
	shared  bool
	entries map[string]conversation
	turns   map[string]*conversationTurn
	path    string
}

type storedConversation struct {
	Messages  []map[string]string `json:"messages"`
	ExpiresAt time.Time           `json:"expires_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type conversationTurn struct {
	busy chan struct{}
	refs int
}

type chatResult struct {
	text string
	err  error
}

func main() {
	ttl, err := time.ParseDuration(value("VOICE_CONVERSATION_TTL", "0s"))
	if err != nil || ttl < 0 {
		slog.Error("VOICE_CONVERSATION_TTL must be a non-negative duration")
		os.Exit(1)
	}
	scope := value("VOICE_CONVERSATION_SCOPE", "shared")
	if scope != "shared" && scope != "session" {
		slog.Error("VOICE_CONVERSATION_SCOPE must be shared or session")
		os.Exit(1)
	}
	cfg := config{
		ListenAddr:           value("VOICE_GATEWAY_LISTEN_ADDR", ":8090"),
		STTURL:               value("STT_URL", "http://faster-whisper:8000/v1/audio/transcriptions"),
		TTSURL:               value("TTS_URL", "http://piper:8000/tts"),
		OpenWebUIURL:         os.Getenv("OPEN_WEBUI_CHAT_URL"),
		OpenWebUIAPIKey:      os.Getenv("OPEN_WEBUI_API_KEY"),
		OpenWebUISocketToken: os.Getenv("OPEN_WEBUI_SOCKET_TOKEN"),
		OpenWebUIModel:       value("OPEN_WEBUI_MODEL", "orion"),
		OpenWebUIToolIDs:     values("OPEN_WEBUI_TOOL_IDS"),
		AccessToken:          os.Getenv("VOICE_GATEWAY_ACCESS_TOKEN"),
		MaxAudioBytes:        15 << 20,
		LogContent:           enabled("VOICE_GATEWAY_LOG_CONTENT"),
		Conversations:        &conversationStore{ttl: ttl, shared: scope == "shared", entries: map[string]conversation{}, path: strings.TrimSpace(os.Getenv("VOICE_CONVERSATION_FILE"))},
	}
	if err := cfg.Conversations.load(); err != nil {
		slog.Error("cannot load voice conversation history", "error", err)
		os.Exit(1)
	}
	if err := cfg.Conversations.save(); err != nil {
		slog.Error("cannot initialize voice conversation history", "error", err)
		os.Exit(1)
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
	transcript = normalizeTranscript(transcript)
	slog.Info("voice provider timing", "provider", "stt", "duration", time.Since(sttStarted), "audio_bytes", len(audio), "transcript_chars", len(transcript))
	if transcript == "" {
		http.Error(writer, "speech was not recognized", http.StatusUnprocessableEntity)
		return
	}
	if cfg.LogContent {
		slog.Info("voice transcript", "text", transcript)
	}

	sessionID := strings.TrimSpace(request.Header.Get("X-Orion-Session-ID"))
	release, err := cfg.Conversations.acquireTurn(request.Context(), sessionID)
	if err != nil {
		return
	}
	defer release()
	boundary := "voice-" + fmt.Sprintf("%d", time.Now().UnixNano())
	writer.Header().Set("Content-Type", "multipart/mixed; boundary="+boundary)
	writer.Header().Set("Cache-Control", "no-cache")
	writer.WriteHeader(http.StatusOK)
	stream := multipart.NewWriter(writer)
	_ = stream.SetBoundary(boundary)
	defer stream.Close()
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

func (store *conversationStore) key(sessionID string) string {
	if store.shared {
		return "shared"
	}
	if sessionID == "" {
		return ""
	}
	return "session:" + sessionID
}

func (store *conversationStore) load() error {
	if store.path == "" {
		return nil
	}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved map[string]storedConversation
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	for key, entry := range saved {
		expiresAt := entry.ExpiresAt
		if !entry.UpdatedAt.IsZero() {
			expiresAt = store.expiry(entry.UpdatedAt)
		}
		store.entries[key] = conversation{messages: entry.Messages, expiresAt: expiresAt, updatedAt: entry.UpdatedAt}
	}
	return nil
}

func (store *conversationStore) save() error {
	if store.path == "" {
		return nil
	}
	saved := make(map[string]storedConversation, len(store.entries))
	for key, entry := range store.entries {
		saved[key] = storedConversation{Messages: entry.messages, ExpiresAt: entry.expiresAt, UpdatedAt: entry.updatedAt}
	}
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(store.path), ".voice-conversations-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), store.path)
}

func (store *conversationStore) saveWithWarning() {
	if err := store.save(); err != nil {
		slog.Error("cannot save voice conversation history", "error", err)
	}
}

func (store *conversationStore) acquireTurn(ctx context.Context, sessionID string) (func(), error) {
	key := store.key(sessionID)
	if key == "" {
		return func() {}, nil
	}
	store.mu.Lock()
	if store.turns == nil {
		store.turns = make(map[string]*conversationTurn)
	}
	turn := store.turns[key]
	if turn == nil {
		turn = &conversationTurn{busy: make(chan struct{}, 1)}
		store.turns[key] = turn
	}
	turn.refs++
	store.mu.Unlock()
	cleanup := func(acquired bool) {
		store.mu.Lock()
		if acquired {
			<-turn.busy
		}
		turn.refs--
		if turn.refs == 0 {
			delete(store.turns, key)
		}
		store.mu.Unlock()
	}
	select {
	case turn.busy <- struct{}{}:
		return func() { cleanup(true) }, nil
	case <-ctx.Done():
		cleanup(false)
		return nil, ctx.Err()
	}
}

func (store *conversationStore) messages(sessionID, transcript string) []map[string]string {
	store.mu.Lock()
	defer store.mu.Unlock()
	messages := []map[string]string{{"role": "system", "content": "Respond for a voice assistant. Use concise plain spoken text only. Do not use Markdown, lists, headings, code formatting, links, or emoji. For device write commands, report only that the command was sent or queued; do not claim the requested state was achieved. Report errors and partial failures accurately."}}
	sessionID = store.key(sessionID)
	if sessionID == "" {
		return append(messages, map[string]string{"role": "user", "content": transcript})
	}
	now := time.Now()
	for id, entry := range store.entries {
		if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) && (id == sessionID || store.turns[id] == nil) {
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
	store.entries[sessionID] = conversation{messages: append(history, user), expiresAt: store.expiry(now), updatedAt: now}
	store.saveWithWarning()
	return messages
}

func (store *conversationStore) expiry(now time.Time) time.Time {
	if store.ttl <= 0 {
		return time.Time{}
	}
	return now.Add(store.ttl)
}

func (store *conversationStore) addAssistant(sessionID, text string) {
	sessionID = store.key(sessionID)
	if sessionID == "" || strings.TrimSpace(text) == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, ok := store.entries[sessionID]
	if !ok || (!entry.expiresAt.IsZero() && !time.Now().Before(entry.expiresAt) && store.turns[sessionID] == nil) {
		delete(store.entries, sessionID)
		return
	}
	entry.messages = append(entry.messages, map[string]string{"role": "assistant", "content": strings.TrimSpace(text)})
	entry.updatedAt = time.Now()
	entry.expiresAt = store.expiry(entry.updatedAt)
	store.entries[sessionID] = entry
	store.saveWithWarning()
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

func speechText(text string) string {
	text = markdownLink.ReplaceAllString(text, "$1")
	text = markdownPrefix.ReplaceAllString(text, "")
	text = strings.NewReplacer("**", "", "__", "", "`", "", "#", "", "\n", " ").Replace(text)
	return strings.Join(strings.Fields(text), " ")
}

var (
	leadingTurnOver = regexp.MustCompile(`(?i)^\s*turn over\b`)
	lightingTarget  = regexp.MustCompile(`(?i)\b(?:light|lights|lamp|lamps)\b`)
)

func normalizeTranscript(transcript string) string {
	transcript = strings.TrimSpace(transcript)
	if leadingTurnOver.MatchString(transcript) && lightingTarget.MatchString(transcript) {
		return leadingTurnOver.ReplaceAllString(transcript, "turn off")
	}
	return transcript
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
