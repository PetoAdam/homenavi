package main

import (
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