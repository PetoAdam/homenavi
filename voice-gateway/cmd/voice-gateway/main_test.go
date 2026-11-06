package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type testSocketConnection struct {
	*bytes.Reader
	written bytes.Buffer
}

func (connection *testSocketConnection) Write(data []byte) (int, error) {
	return connection.written.Write(data)
}

func (connection *testSocketConnection) Close() error { return nil }

func TestWebsocketHandlesFragmentationAndPing(t *testing.T) {
	connection := &testSocketConnection{Reader: bytes.NewReader([]byte{0x01, 2, '4', '2', 0x89, 1, 'x', 0x80, 2, '[', ']'})}
	socket := &websocket{connection: connection, reader: bufio.NewReader(connection)}
	message, err := socket.read()
	if err != nil || message != "42[]" {
		t.Fatalf("read fragmented message: %q, %v", message, err)
	}
	pong := connection.written.Bytes()
	if len(pong) != 7 || pong[0] != 0x8a || pong[1] != 0x81 || pong[6]^pong[2] != 'x' {
		t.Fatalf("invalid masked pong: %v", pong)
	}
}

func TestWebsocketRejectsInvalidFrames(t *testing.T) {
	for _, frame := range [][]byte{
		{0x81, 0x80},
		{0xc1, 0x00},
		{0x80, 0x00},
		{0x82, 0x00},
		{0x81, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x01},
	} {
		connection := &testSocketConnection{Reader: bytes.NewReader(frame)}
		socket := &websocket{connection: connection, reader: bufio.NewReader(connection)}
		if _, err := socket.read(); err == nil {
			t.Fatalf("accepted invalid websocket frame %v", frame)
		}
	}
}

func TestLiveOpenWebUISocketRejectsInvalidToken(t *testing.T) {
	baseURL := os.Getenv("OPEN_WEBUI_TEST_BASE_URL")
	if baseURL == "" {
		t.Skip("set OPEN_WEBUI_TEST_BASE_URL to test the installed Socket.IO server")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := openWebsocket(ctx, strings.TrimRight(baseURL, "/")+"/ws/socket.io/?EIO=4&transport=websocket")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.close()
	socket := &managedSocket{connection: connection}
	opening, err := socket.receive()
	if err != nil || !strings.HasPrefix(opening, "0") {
		t.Fatalf("invalid Engine.IO opening packet: %q, %v", opening, err)
	}
	if err := connection.write(1, []byte(`40{"token":"invalid-test-token"}`)); err != nil {
		t.Fatal(err)
	}
	connected, err := socket.receive()
	if err != nil || !strings.HasPrefix(connected, "40") {
		t.Fatalf("invalid Socket.IO connection packet: %q, %v", connected, err)
	}
	if err := connection.write(1, []byte(`421["user-join",{"auth":{"token":"invalid-test-token"}}]`)); err != nil {
		t.Fatal(err)
	}
	ack, err := socket.receive()
	if err != nil || ack != "431[]" {
		t.Fatalf("expected unauthenticated join rejection, got %q, %v", ack, err)
	}
}

func TestLiveManagedChatStreamsReadOnly(t *testing.T) {
	baseURL := os.Getenv("OPEN_WEBUI_TEST_BASE_URL")
	apiKey := os.Getenv("OPEN_WEBUI_TEST_API_KEY")
	socketToken := os.Getenv("OPEN_WEBUI_TEST_SOCKET_TOKEN")
	model := os.Getenv("OPEN_WEBUI_TEST_MODEL")
	if baseURL == "" || apiKey == "" || socketToken == "" || model == "" {
		t.Skip("set OPEN_WEBUI_TEST_BASE_URL, OPEN_WEBUI_TEST_API_KEY, OPEN_WEBUI_TEST_SOCKET_TOKEN, and OPEN_WEBUI_TEST_MODEL")
	}
	cfg := config{OpenWebUIURL: strings.TrimRight(baseURL, "/") + "/api/chat/completions", OpenWebUIAPIKey: apiKey, OpenWebUISocketToken: socketToken, OpenWebUIModel: model}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	socket, err := cfg.openManagedSocket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.close()
	messages := []map[string]string{{"role": "user", "content": "Reply with two short sentences about the color blue. Do not call tools."}}
	turnID := fmt.Sprintf("voice-test-%d", time.Now().UnixNano())
	userID, assistantID := turnID+"-user", turnID+"-assistant"
	chatID, err := cfg.createVoiceChat(ctx, messages, userID, assistantID)
	if err != nil {
		t.Fatal(err)
	}
	complete := false
	defer func() {
		if !complete {
			cfg.stopManagedChat(chatID)
		}
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		request, err := http.NewRequestWithContext(cleanup, http.MethodDelete, cfg.openWebUIBaseURL()+"/api/v1/chats/"+chatID, nil)
		if err != nil {
			t.Errorf("create test chat cleanup request: %v", err)
			return
		}
		cfg.authorize(request)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Errorf("delete test chat: %v", err)
			return
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("delete test chat returned %s", response.Status)
		}
	}()
	updates := socket.updates(ctx, chatID, assistantID)
	if err := cfg.startManagedChat(ctx, messages, socket, chatID, userID, assistantID); err != nil {
		t.Fatal(err)
	}
	previous := ""
	streamed := false
	for update := range updates {
		if update.err != nil {
			t.Fatal(update.err)
		}
		if !strings.HasPrefix(update.text, previous) {
			t.Fatal("managed response revised previously received text")
		}
		if !update.done && update.text != "" {
			streamed = true
		}
		previous = update.text
		if update.done {
			complete = true
			if !streamed || strings.TrimSpace(update.text) == "" {
				t.Fatal("no usable assistant text arrived before completion")
			}
			return
		}
	}
	t.Fatal("managed chat ended without completion")
}

func serveMockSocket(t *testing.T, writer http.ResponseWriter, request *http.Request, packets <-chan string, ack string, pongs chan<- struct{}) {
	t.Helper()
	if request.URL.Query().Get("EIO") != "4" || request.URL.Query().Get("transport") != "websocket" {
		t.Errorf("unexpected socket transport: %s", request.URL.RawQuery)
	}
	connection, buffer, err := writer.(http.Hijacker).Hijack()
	if err != nil {
		t.Errorf("hijack socket: %v", err)
		return
	}
	defer connection.Close()
	hash := sha1.Sum([]byte(request.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	_, _ = fmt.Fprintf(buffer, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(hash[:]))
	_ = buffer.Flush()
	write := func(packet string) error {
		frame := []byte{0x81}
		if len(packet) < 126 {
			frame = append(frame, byte(len(packet)))
		} else {
			frame = append(frame, 126, byte(len(packet)>>8), byte(len(packet)))
		}
		_, err := connection.Write(append(frame, []byte(packet)...))
		return err
	}
	read := func() (string, error) {
		var header [2]byte
		if _, err := io.ReadFull(buffer, header[:]); err != nil {
			return "", err
		}
		if header[0] != 0x81 || header[1]&0x80 == 0 {
			return "", fmt.Errorf("invalid client websocket frame")
		}
		length := int(header[1] & 0x7f)
		if length == 126 {
			var extended [2]byte
			if _, err := io.ReadFull(buffer, extended[:]); err != nil {
				return "", err
			}
			length = int(binary.BigEndian.Uint16(extended[:]))
		}
		var mask [4]byte
		if _, err := io.ReadFull(buffer, mask[:]); err != nil {
			return "", err
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(buffer, payload); err != nil {
			return "", err
		}
		for index := range payload {
			payload[index] ^= mask[index%4]
		}
		return string(payload), nil
	}
	if err := write(`0{"sid":"engine-id"}`); err != nil {
		return
	}
	if packet, err := read(); err != nil || packet != `40{"token":"socket-jwt"}` {
		t.Errorf("invalid socket authentication packet: %q, %v", packet, err)
		return
	}
	if err := write(`40{"sid":"socket-id"}`); err != nil {
		return
	}
	if packet, err := read(); err != nil || packet != `421["user-join",{"auth":{"token":"socket-jwt"}}]` {
		t.Errorf("invalid socket join packet: %q, %v", packet, err)
		return
	}
	if err := write(ack); err != nil {
		return
	}
	disconnected := make(chan struct{})
	go func() {
		defer close(disconnected)
		for {
			packet, err := read()
			if err != nil {
				return
			}
			if packet == "3" && pongs != nil {
				pongs <- struct{}{}
			}
		}
	}()
	for {
		select {
		case packet := <-packets:
			if err := write(packet); err != nil {
				return
			}
		case <-disconnected:
			return
		}
	}
}

func TestConversationStorePreservesSessionHistory(t *testing.T) {
	store := &conversationStore{ttl: time.Minute, entries: map[string]conversation{}}
	store.messages("living-room", "Turn on the lights")
	store.addAssistant("living-room", "The lights are on.")
	messages := store.messages("living-room", "What is still on?")

	if len(messages) != 4 {
		t.Fatalf("expected system plus three conversation messages, got %d", len(messages))
	}
	if messages[1]["content"] != "Turn on the lights" || messages[2]["content"] != "The lights are on." || messages[3]["content"] != "What is still on?" {
		t.Fatalf("unexpected conversation history: %#v", messages)
	}
}

func TestConversationStoreExpiresSessionHistory(t *testing.T) {
	store := &conversationStore{ttl: time.Minute, entries: map[string]conversation{}}
	store.messages("living-room", "First request")
	store.entries["living-room"] = conversation{messages: store.entries["living-room"].messages, expiresAt: time.Now().Add(-time.Second)}
	messages := store.messages("living-room", "New request")

	if len(messages) != 2 || messages[1]["content"] != "New request" {
		t.Fatalf("expected expired history to be discarded, got %#v", messages)
	}
}

func TestVoiceChatHistoryPreservesConversation(t *testing.T) {
	messages := []map[string]string{
		{"role": "system", "content": "Voice assistant"},
		{"role": "user", "content": "Turn on the lamps"},
		{"role": "assistant", "content": "The lamps are on."},
		{"role": "user", "content": "Confirm that."},
	}
	history := voiceChatHistory(messages, "current-user", "current-assistant", "orion")

	if len(history) != 4 {
		t.Fatalf("expected three conversation messages plus placeholder, got %#v", history)
	}
	priorUser, ok := history["current-user-history-1"].(map[string]any)
	if !ok || priorUser["content"] != "Turn on the lamps" {
		t.Fatalf("prior user turn was not preserved: %#v", history)
	}
	currentUser, ok := history["current-user"].(map[string]any)
	if !ok || currentUser["content"] != "Confirm that." || currentUser["childrenIds"].([]string)[0] != "current-assistant" {
		t.Fatalf("current turn was not linked to assistant placeholder: %#v", history)
	}
}

func TestVoiceChatHistoryDoesNotRequireSystemMessage(t *testing.T) {
	history := voiceChatHistory([]map[string]string{{"role": "user", "content": "Hello"}}, "current-user", "current-assistant", "orion")

	currentUser, ok := history["current-user"].(map[string]any)
	if !ok || currentUser["content"] != "Hello" || currentUser["childrenIds"].([]string)[0] != "current-assistant" {
		t.Fatalf("current user turn was not linked without a system message: %#v", history)
	}
}

func TestChatStreamsSingleAuthenticatedManagedTurn(t *testing.T) {
	packets := make(chan string, 10)
	pongs := make(chan struct{}, 1)
	requests := make(chan map[string]any, 1)
	event := func(chatID, messageID, eventType, data string) string {
		return `42["events",{"chat_id":"` + chatID + `","message_id":"` + messageID + `","data":{"type":"` + eventType + `","data":` + data + `}}]`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/ws/socket.io/" && request.Header.Get("Authorization") != "Bearer api-key" {
			t.Errorf("HTTP request missing API authorization")
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/v1/auths/":
			_, _ = writer.Write([]byte(`{"id":"user-id"}`))
		case "/api/v1/chats/new":
			_, _ = writer.Write([]byte(`{"id":"chat-id"}`))
		case "/api/chat/completions":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode managed request: %v", err)
				return
			}
			requests <- payload
			messageID, _ := payload["assistant_message_id"].(string)
			packets <- event("other-chat", messageID, "chat:completion", `{"type":"response.output_text.delta","item_id":"wrong","delta":"Wrong."}`)
			packets <- event("chat-id", messageID, "response:completion", `{"type":"response.reasoning_text.delta","item_id":"reasoning","delta":"Private."}`)
			packets <- event("chat-id", messageID, "response:completion", `{"type":"response.output_text.delta","item_id":"first","delta":"Checking."}`)
			_, _ = writer.Write([]byte(`{"status":true,"task_ids":["task"]}`))
		case "/ws/socket.io/":
			serveMockSocket(t, writer, request, packets, `431[{"id":"user-id"}]`, pongs)
		default:
			t.Errorf("unexpected request: %s", request.URL.Path)
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	cfg := config{OpenWebUIURL: server.URL + "/api/chat/completions", OpenWebUIAPIKey: "api-key", OpenWebUISocketToken: "socket-jwt", OpenWebUIModel: "orion", OpenWebUIToolIDs: []string{"server:mcp:homenavi-mcp"}, Conversations: &conversationStore{ttl: time.Minute, entries: map[string]conversation{}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var responses []string
	var payload map[string]any
	cfg.chat(ctx, "Check the lights.", "session", time.Now())(func(text string, err error) bool {
		if err != nil {
			t.Errorf("chat returned an error: %v", err)
			return false
		}
		responses = append(responses, text)
		if len(responses) == 1 {
			payload = <-requests
			messageID := payload["assistant_message_id"].(string)
			packets <- "2"
			packets <- event("chat-id", messageID, "chat:completion", `{"output":[{"type":"message","id":"first","role":"assistant","content":[{"type":"output_text","text":"Checking."}]},{"type":"function_call_output","output":"not speech"}]}`)
			packets <- event("chat-id", messageID, "chat:completion", `{"type":"response.output_text.delta","item_id":"second","delta":" Queued."}`)
			packets <- event("chat-id", messageID, "chat:completion", `{"done":true,"output":[{"type":"message","id":"first","role":"assistant","content":[{"type":"output_text","text":"Checking."}]},{"type":"message","id":"second","role":"assistant","content":[{"type":"output_text","text":" Queued."}]}]}`)
		}
		return true
	})
	if strings.Join(responses, " ") != "Checking. Queued." {
		t.Fatalf("unexpected streamed responses: %#v", responses)
	}
	if payload["stream"] != true || payload["session_id"] != "socket-id" || payload["chat_id"] != "chat-id" {
		t.Fatalf("invalid managed request: %#v", payload)
	}
	select {
	case duplicate := <-requests:
		t.Fatalf("unexpected duplicate completion request: %#v", duplicate)
	default:
	}
	select {
	case <-pongs:
	case <-time.After(time.Second):
		t.Fatal("socket did not answer the Engine.IO heartbeat")
	}
}

func TestTextprotoHeaderCopiesPCMFormat(t *testing.T) {
	responseHeaders := http.Header{
		"X-Audio-Sample-Rate":  {"22050"},
		"X-Audio-Sample-Width": {"2"},
		"X-Audio-Channels":     {"1"},
	}
	header := textprotoHeader("audio/L16", "Hello.", responseHeaders)
	values := http.Header(header)

	if values.Get("Content-Type") != "audio/L16" || values.Get("X-Voice-Text") != "Hello." {
		t.Fatalf("unexpected base headers: %#v", header)
	}
	if values.Get("X-Audio-Sample-Rate") != "22050" || values.Get("X-Audio-Sample-Width") != "2" || values.Get("X-Audio-Channels") != "1" {
		t.Fatalf("missing PCM headers: %#v", header)
	}
}

func TestNormalizeTranscriptCorrectsTurnOverCommand(t *testing.T) {
	if transcript := normalizeTranscript(" Turn over the bedroom wardrobe light. "); transcript != "turn off the bedroom wardrobe light." {
		t.Fatalf("unexpected normalized transcript: %q", transcript)
	}
	if transcript := normalizeTranscript("Turn over the page."); transcript != "Turn over the page." {
		t.Fatalf("unexpected unchanged transcript: %q", transcript)
	}
}

func TestCopyAndFlushStreamsAllAudio(t *testing.T) {
	recorder := httptest.NewRecorder()
	var destination bytes.Buffer
	payload := []byte("streamed-pcm")

	written, err := copyAndFlush(&destination, bytes.NewReader(payload), recorder, time.Now())
	if err != nil {
		t.Fatalf("copyAndFlush returned an error: %v", err)
	}
	if written != int64(len(payload)) || !bytes.Equal(destination.Bytes(), payload) {
		t.Fatalf("unexpected streamed payload: written=%d payload=%q", written, destination.Bytes())
	}
}

func TestNextClauseSplitsLongCommaClause(t *testing.T) {
	clause, rest := nextClause("The kitchen lights are now on, and the hallway remains off.")

	if clause != "The kitchen lights are now on," || rest != "and the hallway remains off." {
		t.Fatalf("unexpected clause split: clause=%q rest=%q", clause, rest)
	}
}

func TestNextClauseWaitsForShortPhraseCompletion(t *testing.T) {
	clause, rest := nextClause("Yes, I can help.")

	if clause != "Yes, I can help." || rest != "" {
		t.Fatalf("unexpected short clause split: clause=%q rest=%q", clause, rest)
	}
}

func TestManagedOutputTracksAssistantTextAndSnapshots(t *testing.T) {
	tracker := newManagedOutput()
	events := []struct {
		data string
		want string
		done bool
	}{
		{`{"type":"response.reasoning_text.delta","item_id":"reasoning","delta":"private"}`, "", false},
		{`{"type":"response.function_call_arguments.delta","delta":{"arguments":"private"}}`, "", false},
		{`{"type":"response.output_text.delta","item_id":"first","delta":"Checking."}`, "Checking.", false},
		{`{"output":[{"type":"message","id":"first","role":"assistant","content":[{"type":"output_text","text":"Checking."}]},{"type":"function_call_output","output":"secret"}]}`, "Checking.", false},
		{`{"type":"response.output_text.delta","item_id":"second","delta":" Queued."}`, "Checking. Queued.", false},
		{`{"done":true,"output":[{"type":"message","id":"first","role":"assistant","content":[{"type":"output_text","text":"Checking."}]},{"type":"message","id":"second","role":"assistant","content":[{"type":"output_text","text":" Queued."}]}]}`, "Checking. Queued.", true},
	}
	for _, event := range events {
		got, done, err := tracker.apply([]byte(event.data))
		if err != nil || got != event.want || done != event.done {
			t.Fatalf("apply(%s) = %q, %t, %v; want %q, %t", event.data, got, done, err, event.want, event.done)
		}
	}
}

func TestManagedSocketRejectsUnauthenticatedOrMismatchedUser(t *testing.T) {
	for _, test := range []struct {
		name string
		ack  string
		want string
	}{
		{name: "unauthenticated", ack: `431[]`, want: "authentication failed"},
		{name: "different user", ack: `431[{"id":"other-user"}]`, want: "different users"},
	} {
		t.Run(test.name, func(t *testing.T) {
			packets := make(chan string)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch {
				case request.URL.Path == "/api/v1/auths/":
					_, _ = writer.Write([]byte(`{"id":"user-id"}`))
				case request.URL.Path == "/ws/socket.io/":
					serveMockSocket(t, writer, request, packets, test.ack, nil)
				default:
					t.Errorf("unexpected request: %s", request.URL.Path)
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()
			cfg := config{OpenWebUIURL: server.URL + "/api/chat/completions", OpenWebUIAPIKey: "api-key", OpenWebUISocketToken: "socket-jwt"}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := cfg.openManagedSocket(ctx)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestManagedChatStopsTaskOnCancellation(t *testing.T) {
	packets := make(chan string)
	stopped := make(chan struct{}, 1)
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/auths/":
			_, _ = writer.Write([]byte(`{"id":"user-id"}`))
		case "/ws/socket.io/":
			serveMockSocket(t, writer, request, packets, `431[{"id":"user-id"}]`, nil)
		case "/api/v1/chats/new":
			_, _ = writer.Write([]byte(`{"id":"chat-id"}`))
		case "/api/chat/completions":
			started <- struct{}{}
			_, _ = writer.Write([]byte(`{"status":true,"task_ids":["task"]}`))
		case "/api/tasks/chat/chat-id/stop":
			if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer api-key" {
				t.Errorf("invalid stop request")
			}
			stopped <- struct{}{}
		default:
			t.Errorf("unexpected request: %s", request.URL.Path)
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	cfg := config{OpenWebUIURL: server.URL + "/api/chat/completions", OpenWebUIAPIKey: "api-key", OpenWebUISocketToken: "socket-jwt", OpenWebUIModel: "orion", Conversations: &conversationStore{ttl: time.Minute, entries: map[string]conversation{}}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	cfg.chat(ctx, "Hello", "session", time.Now())(func(_ string, _ error) bool { return false })
	select {
	case <-stopped:
	default:
		t.Fatal("unfinished managed chat was not stopped")
	}
}
