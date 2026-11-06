package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

func (cfg config) chat(parent context.Context, transcript, sessionID string, started time.Time) func(func(string, error) bool) {
	return func(yield func(string, error) bool) {
		ctx, cancel := context.WithTimeout(parent, 90*time.Second)
		defer cancel()
		messages := cfg.Conversations.messages(sessionID, transcript)
		slog.Info("voice conversation", "session_id", sessionID, "history_messages", len(messages)-2)
		socket, err := cfg.openManagedSocket(ctx)
		if err != nil {
			yield("", err)
			return
		}
		defer func() {
			cancel()
			socket.close()
		}()
		turnID := fmt.Sprintf("voice-%d", time.Now().UnixNano())
		userMessageID, assistantMessageID := turnID+"-user", turnID+"-assistant"
		chatID, err := cfg.createVoiceChat(ctx, messages, userMessageID, assistantMessageID)
		if err != nil {
			yield("", err)
			return
		}
		updates := socket.updates(ctx, chatID, assistantMessageID)
		completed := false
		defer func() {
			if !completed {
				cfg.stopManagedChat(chatID)
			}
		}()
		if err := cfg.startManagedChat(ctx, messages, socket, chatID, userMessageID, assistantMessageID); err != nil {
			yield("", err)
			return
		}
		spokenPrefix := ""
		firstText := true
		for update := range updates {
			if update.err != nil {
				yield("", update.err)
				return
			}
			if !strings.HasPrefix(update.text, spokenPrefix) {
				yield("", errors.New("Open WebUI revised already spoken text"))
				return
			}
			remaining := update.text[len(spokenPrefix):]
			if firstText && remaining != "" {
				slog.Info("voice provider timing", "provider", "llm", "phase", "first_text_delta", "duration", time.Since(started))
				firstText = false
			}
			for {
				clause, _ := nextClause(remaining)
				if clause == "" {
					break
				}
				end := strings.Index(remaining, clause) + len(clause)
				spokenPrefix += remaining[:end]
				remaining = remaining[end:]
				if !yield(clause, nil) {
					return
				}
			}
			if update.done {
				completed = true
				if final := strings.TrimSpace(remaining); final != "" {
					yield(final, nil)
				} else if spokenPrefix == "" {
					yield("I couldn't complete that request. Please try again.", nil)
				}
				return
			}
		}
		if err := ctx.Err(); err != nil {
			yield("", err)
		}
	}
}
