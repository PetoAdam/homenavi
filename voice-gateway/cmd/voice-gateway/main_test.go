package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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

func TestAssistantMessageContentDoesNotReturnPriorAssistantTurn(t *testing.T) {
	data := []byte(`{"history":{"messages":{"prior-assistant":{"role":"assistant","content":"Previous response."},"current-assistant":{"role":"assistant","content":""}}}}`)

	if content, ok := assistantMessageContent(data, "current-assistant"); ok || content != "" {
		t.Fatalf("expected empty current response, got %q", content)
	}
	if content, ok := assistantMessageContent(data, "prior-assistant"); !ok || content != "Previous response." {
		t.Fatalf("expected the requested prior response, got %q", content)
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

func TestAppendDeliveredTextPreservesCompletionPrefix(t *testing.T) {
	delivered := appendDeliveredText("Not much.", "I'm here and ready to help.")
	completed := "Not much. I'm here and ready to help. What's on your mind?"

	if !strings.HasPrefix(completed, delivered) {
		t.Fatalf("delivered text is not a completion prefix: %q", delivered)
	}
}

func TestRecoverIncompleteFinalWaitsForPersistedCompletion(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		content := "Could you check the weather"
		if requests == 2 {
			content = "Could you check the weather for Budapest tomorrow?"
		}
		_, _ = writer.Write([]byte(`{"history":{"messages":{"assistant":{"role":"assistant","content":"` + content + `"}}}}`))
	}))
	defer server.Close()

	cfg := config{OpenWebUIURL: server.URL + "/api/chat/completions"}
	final := cfg.recoverIncompleteFinal(context.Background(), "chat", "assistant", "", "Could you check the weather")

	if final != "Could you check the weather for Budapest tomorrow?" {
		t.Fatalf("expected persisted completion, got %q", final)
	}
	if requests != 2 {
		t.Fatalf("expected two chat lookups, got %d", requests)
	}
}

func TestStreamDeltaSupportsOpenAIAndOpenWebUIEnvelopes(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "OpenAI", data: `{"choices":[{"delta":{"content":"Hello"}}]}`, want: "Hello"},
		{name: "Responses API", data: `{"type":"response.output_text.delta","delta":"Hello"}`, want: "Hello"},
		{name: "Open WebUI data", data: `{"type":"chat:completion","data":{"content":"Hello"}}`, want: "Hello"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			delta, _, ok := streamDelta([]byte(test.data))
			if !ok || delta != test.want {
				t.Fatalf("streamDelta(%s) = %q, %t", test.data, delta, ok)
			}
		})
	}
}
