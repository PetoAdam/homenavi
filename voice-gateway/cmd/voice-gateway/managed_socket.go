package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type managedSocket struct {
	connection *websocket
	sessionID  string
}

func (cfg config) openManagedSocket(ctx context.Context) (*managedSocket, error) {
	token := cfg.OpenWebUISocketToken
	if token == "" {
		token = cfg.OpenWebUIAPIKey
	}
	if token == "" {
		return nil, errors.New("OPEN_WEBUI_SOCKET_TOKEN must contain an Open WebUI user session token")
	}
	userRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.openWebUIBaseURL()+"/api/v1/auths/", nil)
	if err != nil {
		return nil, err
	}
	cfg.authorize(userRequest)
	userResponse, err := http.DefaultClient.Do(userRequest)
	if err != nil {
		return nil, err
	}
	defer userResponse.Body.Close()
	if userResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open WebUI user lookup returned %s", userResponse.Status)
	}
	var httpUser struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(userResponse.Body, 1<<20)).Decode(&httpUser); err != nil || httpUser.ID == "" {
		return nil, errors.New("Open WebUI user lookup returned no user ID")
	}
	endpoint, err := url.Parse(cfg.openWebUIBaseURL() + "/ws/socket.io/")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("EIO", "4")
	query.Set("transport", "websocket")
	endpoint.RawQuery = query.Encode()
	connection, err := openWebsocket(ctx, endpoint.String())
	if err != nil {
		return nil, err
	}
	socket := &managedSocket{connection: connection}
	authenticated := false
	defer func() {
		if !authenticated {
			socket.close()
		}
	}()
	opening, err := socket.receive()
	if err != nil {
		return nil, err
	}
	var handshake struct {
		SID string `json:"sid"`
	}
	if !strings.HasPrefix(opening, "0") || json.Unmarshal([]byte(opening[1:]), &handshake) != nil || handshake.SID == "" {
		return nil, errors.New("invalid Engine.IO handshake")
	}
	auth, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return nil, err
	}
	if err := connection.write(1, []byte("40"+string(auth))); err != nil {
		return nil, err
	}
	for socket.sessionID == "" {
		packet, err := socket.receive()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(packet, "44") {
			return nil, errors.New("Open WebUI Socket.IO connection rejected")
		}
		if strings.HasPrefix(packet, "40") {
			var connected struct {
				SID string `json:"sid"`
			}
			if err := json.Unmarshal([]byte(packet[2:]), &connected); err != nil || connected.SID == "" {
				return nil, errors.New("invalid Socket.IO session ID")
			}
			socket.sessionID = connected.SID
		}
	}
	join, err := json.Marshal([]any{"user-join", map[string]any{"auth": map[string]string{"token": token}}})
	if err != nil {
		return nil, err
	}
	if err := connection.write(1, []byte("421"+string(join))); err != nil {
		return nil, err
	}
	for {
		packet, err := socket.receive()
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(packet, "431") {
			continue
		}
		var result []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(packet[3:]), &result); err != nil || len(result) != 1 || result[0].ID == "" {
			return nil, errors.New("Open WebUI socket authentication failed; provide a valid OPEN_WEBUI_SOCKET_TOKEN")
		}
		if result[0].ID != httpUser.ID {
			return nil, errors.New("Open WebUI socket and API credentials belong to different users")
		}
		authenticated = true
		return socket, nil
	}
}

func (socket *managedSocket) receive() (string, error) {
	for {
		packet, err := socket.connection.read()
		if err != nil {
			return "", err
		}
		if packet == "2" {
			if err := socket.connection.write(1, []byte("3")); err != nil {
				return "", err
			}
			continue
		}
		if packet == "1" || packet == "41" {
			return "", errors.New("Open WebUI socket disconnected")
		}
		return packet, nil
	}
}

func (socket *managedSocket) close() {
	socket.connection.close()
}

type managedUpdate struct {
	text string
	done bool
	err  error
}

func (socket *managedSocket) updates(ctx context.Context, chatID, messageID string) <-chan managedUpdate {
	updates := make(chan managedUpdate, 1)
	go func() {
		defer close(updates)
		tracker := newManagedOutput()
		publish := func(update managedUpdate) {
			select {
			case updates <- update:
			default:
				select {
				case <-updates:
				default:
				}
				select {
				case updates <- update:
				case <-ctx.Done():
				}
			}
		}
		for {
			packet, err := socket.receive()
			if err != nil {
				publish(managedUpdate{err: err})
				return
			}
			if !strings.HasPrefix(packet, "42") {
				continue
			}
			var args []json.RawMessage
			if err := json.Unmarshal([]byte(packet[2:]), &args); err != nil || len(args) != 2 || string(args[0]) != `"events"` {
				continue
			}
			var envelope struct {
				ChatID    string `json:"chat_id"`
				MessageID string `json:"message_id"`
				Data      struct {
					Type string          `json:"type"`
					Data json.RawMessage `json:"data"`
				} `json:"data"`
			}
			if err := json.Unmarshal(args[1], &envelope); err != nil || envelope.ChatID != chatID || envelope.MessageID != messageID {
				continue
			}
			if envelope.Data.Type == "chat:message:error" {
				publish(managedUpdate{err: errors.New("Open WebUI managed chat reported an error")})
				return
			}
			if envelope.Data.Type != "chat:completion" && envelope.Data.Type != "response:completion" {
				continue
			}
			text, done, err := tracker.apply(envelope.Data.Data)
			if len(text) > 1<<20 {
				err = errors.New("Open WebUI response exceeds size limit")
			}
			publish(managedUpdate{text: text, done: done, err: err})
			if done || err != nil {
				return
			}
		}
	}()
	return updates
}

func (cfg config) startManagedChat(ctx context.Context, messages []map[string]string, socket *managedSocket, chatID, userMessageID, assistantMessageID string) error {
	payload := map[string]any{
		"model": cfg.OpenWebUIModel, "stream": true, "messages": messages,
		"id": assistantMessageID, "chat_id": chatID, "parent_id": userMessageID,
		"assistant_message_id": assistantMessageID, "session_id": socket.sessionID,
		"user_message": map[string]any{"id": userMessageID, "role": "user", "content": messages[len(messages)-1]["content"], "timestamp": time.Now().Unix()},
		"params":       map[string]string{"tool_approval_mode": "full"},
	}
	if len(cfg.OpenWebUIToolIDs) > 0 {
		payload["tool_ids"] = cfg.OpenWebUIToolIDs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.OpenWebUIURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	cfg.authorize(request)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Open WebUI managed chat returned %s", response.Status)
	}
	var result struct {
		Status bool `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil || !result.Status {
		return errors.New("Open WebUI did not accept the managed chat")
	}
	return nil
}

func (cfg config) stopManagedChat(chatID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.openWebUIBaseURL()+"/api/tasks/chat/"+url.PathEscape(chatID)+"/stop", nil)
	if err != nil {
		slog.Warn("could not create Open WebUI stop request", "error", err)
		return
	}
	cfg.authorize(request)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		slog.Warn("could not stop Open WebUI managed chat", "error", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		slog.Warn("Open WebUI managed chat stop failed", "status", response.StatusCode)
	}
}
