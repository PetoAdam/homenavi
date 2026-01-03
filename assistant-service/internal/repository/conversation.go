package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Conversation represents a chat conversation
type Conversation struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConversationRepository handles conversation persistence
type ConversationRepository struct {
	db *DB
}

// NewConversationRepository creates a new conversation repository
func NewConversationRepository(db *DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

// Create creates a new conversation
func (r *ConversationRepository) Create(ctx context.Context, userID uuid.UUID, title string) (*Conversation, error) {
	conv := &Conversation{
		ID:        uuid.New(),
		UserID:    userID,
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO assistant_conversations (id, user_id, title, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		conv.ID, conv.UserID, conv.Title, conv.CreatedAt, conv.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return conv, nil
}

// GetByID retrieves a conversation by ID
func (r *ConversationRepository) GetByID(ctx context.Context, id uuid.UUID) (*Conversation, error) {
	conv := &Conversation{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, title, created_at, updated_at FROM assistant_conversations WHERE id = $1`,
		id,
	).Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return conv, nil
}

// ListByUser lists conversations for a user
func (r *ConversationRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Conversation, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, title, created_at, updated_at 
		 FROM assistant_conversations 
		 WHERE user_id = $1 
		 ORDER BY updated_at DESC 
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []*Conversation
	for rows.Next() {
		conv := &Conversation{}
		if err := rows.Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
			return nil, err
		}
		conversations = append(conversations, conv)
	}

	return conversations, rows.Err()
}

// UpdateTitle updates the conversation title
func (r *ConversationRepository) UpdateTitle(ctx context.Context, id uuid.UUID, title string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE assistant_conversations SET title = $1, updated_at = NOW() WHERE id = $2`,
		title, id,
	)
	return err
}

// Touch updates the updated_at timestamp
func (r *ConversationRepository) Touch(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE assistant_conversations SET updated_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}

// Delete deletes a conversation
func (r *ConversationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM assistant_conversations WHERE id = $1`,
		id,
	)
	return err
}

// Message represents a chat message
type Message struct {
	ID             uuid.UUID       `json:"id"`
	ConversationID uuid.UUID       `json:"conversation_id"`
	Role           string          `json:"role"` // "user", "assistant", "system", "tool"
	Content        string          `json:"content"`
	ToolCalls      json.RawMessage `json:"tool_calls,omitempty"`
	ToolResults    json.RawMessage `json:"tool_results,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// MessageRepository handles message persistence
type MessageRepository struct {
	db *DB
}

// NewMessageRepository creates a new message repository
func NewMessageRepository(db *DB) *MessageRepository {
	return &MessageRepository{db: db}
}

// Create creates a new message
func (r *MessageRepository) Create(ctx context.Context, conversationID uuid.UUID, role, content string, toolCalls, toolResults json.RawMessage) (*Message, error) {
	msg := &Message{
		ID:             uuid.New(),
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		ToolCalls:      toolCalls,
		ToolResults:    toolResults,
		CreatedAt:      time.Now(),
	}

	var toolCallsParam interface{}
	if len(toolCalls) > 0 {
		toolCallsParam = string(toolCalls)
	}
	var toolResultsParam interface{}
	if len(toolResults) > 0 {
		toolResultsParam = string(toolResults)
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO assistant_messages (id, conversation_id, role, content, tool_calls, tool_results, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		msg.ID, msg.ConversationID, msg.Role, msg.Content, toolCallsParam, toolResultsParam, msg.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// ListByConversation lists messages for a conversation
func (r *MessageRepository) ListByConversation(ctx context.Context, conversationID uuid.UUID) ([]*Message, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, tool_calls, tool_results, created_at 
		 FROM assistant_messages 
		 WHERE conversation_id = $1 
		 ORDER BY created_at ASC`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg := &Message{}
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ToolCalls, &msg.ToolResults, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

// GetLastN retrieves the last N messages from a conversation
func (r *MessageRepository) GetLastN(ctx context.Context, conversationID uuid.UUID, n int) ([]*Message, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, tool_calls, tool_results, created_at 
		 FROM assistant_messages 
		 WHERE conversation_id = $1 
		 ORDER BY created_at DESC 
		 LIMIT $2`,
		conversationID, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg := &Message{}
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ToolCalls, &msg.ToolResults, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, rows.Err()
}
