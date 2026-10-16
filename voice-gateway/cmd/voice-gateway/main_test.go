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