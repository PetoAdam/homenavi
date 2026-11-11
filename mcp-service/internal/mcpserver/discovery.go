package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func validateDiscoveryInput(input discoveryInput, roomRequired bool) error {
	for _, value := range []string{input.RoomID, input.Room, input.Name, input.DeviceType, input.Capability} {
		if len(strings.TrimSpace(value)) > 128 {
			return fmt.Errorf("invalid discovery filter")
		}
	}
	roomID, room := strings.TrimSpace(input.RoomID), strings.TrimSpace(input.Room)
	if roomID != "" && room != "" || roomRequired && roomID == "" && room == "" || input.Limit < 0 || input.Limit > 100 || len(input.Cursor) > 2048 {
		return fmt.Errorf("invalid room, limit, or cursor")
	}
	if input.Detail != "" && input.Detail != "compact" && input.Detail != "verbose" || input.Detail != "" && len(input.Fields) > 0 {
		return fmt.Errorf("invalid or conflicting detail and fields")
	}
	allowed := map[string]bool{"device_id": true, "status": true, "name": true, "room": true, "room_id": true, "type": true, "online": true, "state": true, "state_updated_at": true, "last_seen": true}
	seen := map[string]bool{}
	for _, field := range input.Fields {
		if !allowed[field] || seen[field] {
			return fmt.Errorf("invalid or duplicate field %q", field)
		}
		seen[field] = true
	}
	if len(input.StateProperties) > 100 {
		return fmt.Errorf("too many state properties")
	}
	properties := map[string]bool{}
	for _, property := range input.StateProperties {
		if property == "" || len(property) > 128 || strings.TrimSpace(property) != property || properties[property] {
			return fmt.Errorf("invalid or duplicate state property")
		}
		properties[property] = true
	}
	if len(input.StateProperties) > 0 && len(input.Fields) > 0 && !seen["state"] {
		return fmt.Errorf("state_properties requires the state field")
	}
	if len(input.StateProperties) > 0 && len(input.Fields) == 0 && !roomRequired && input.Detail != "verbose" {
		return fmt.Errorf("state_properties requires the state field")
	}
	return nil
}

func (c *readClients) DiscoverDevices(ctx context.Context, input discoveryInput, roomState bool, token string) (any, error) {
	limit := input.Limit
	if limit == 0 {
		limit = 25
	}
	body, err := json.Marshal(map[string]any{"room_id": input.RoomID, "room": input.Room, "name": input.Name, "device_type": input.DeviceType, "capability": input.Capability, "limit": limit, "cursor": input.Cursor})
	if err != nil {
		return nil, err
	}
	path := "/api/mcp/ers/discovery:search"
	if roomState {
		path = "/api/mcp/ers/discovery:room-state"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gatewayURL+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("discovery service returned %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	var result struct {
		Devices    []map[string]any `json:"devices"`
		Room       any              `json:"room"`
		NextCursor string           `json:"next_cursor"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&result); err != nil || result.Devices == nil || len(result.Devices) > limit {
		return nil, fmt.Errorf("invalid discovery response")
	}
	fields := input.Fields
	if len(fields) == 0 {
		fields = []string{"name", "room", "type"}
		if roomState {
			fields = []string{"name", "type", "online", "state", "state_updated_at"}
		}
		if input.Detail == "verbose" {
			fields = []string{"name", "room", "room_id", "type", "online", "state", "state_updated_at", "last_seen"}
		}
	}
	selected := map[string]bool{"device_id": true, "status": true}
	stateFields := []string{}
	for _, field := range fields {
		selected[field] = true
		if field == "online" || field == "state" || field == "state_updated_at" || field == "last_seen" {
			stateFields = append(stateFields, field)
		}
	}
	ids := make([]string, 0, len(result.Devices))
	for _, item := range result.Devices {
		id, ok := item["device_id"].(string)
		if !ok || id == "" {
			return nil, fmt.Errorf("invalid discovery device ID")
		}
		ids = append(ids, id)
		item["status"] = "ok"
	}
	var readAt any
	if len(stateFields) > 0 && len(ids) > 0 {
		batch, err := c.GetDeviceStates(ctx, deviceStatesInput{DeviceIDs: ids, Fields: stateFields}, token)
		if err != nil {
			if errors.Is(err, errDeviceStateBatchTooLarge) && len(ids) > 1 {
				input.Limit = len(ids) / 2
				return c.DiscoverDevices(ctx, input, roomState, token)
			}
			return nil, err
		}
		batchMap, ok := batch.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid batch response")
		}
		readAt = batchMap["read_at"]
		batchResults, ok := batchMap["results"].([]any)
		if !ok || len(batchResults) != len(result.Devices) {
			return nil, fmt.Errorf("invalid batch results")
		}
		for index, raw := range batchResults {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid batch result")
			}
			for key, value := range item {
				result.Devices[index][key] = value
			}
		}
	}
	for _, item := range result.Devices {
		for key := range item {
			if !selected[key] {
				delete(item, key)
			}
		}
		if len(input.StateProperties) > 0 {
			if state, ok := item["state"].(map[string]any); ok {
				allowed := map[string]bool{}
				for _, property := range input.StateProperties {
					allowed[property] = true
				}
				for property := range state {
					if !allowed[property] {
						delete(state, property)
					}
				}
			}
		}
	}
	output := map[string]any{"devices": result.Devices}
	if result.Room != nil {
		output["room"] = result.Room
	}
	if result.NextCursor != "" {
		output["next_cursor"] = result.NextCursor
	}
	if readAt != nil {
		output["read_at"] = readAt
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("encode discovery response: %w", err)
	}
	if len(encoded) > 256<<10 && len(ids) > 1 {
		input.Limit = len(ids) / 2
		return c.DiscoverDevices(ctx, input, roomState, token)
	}
	if len(encoded) > 256<<10 {
		return nil, fmt.Errorf("discovery response exceeds size limit")
	}
	return output, nil
}
