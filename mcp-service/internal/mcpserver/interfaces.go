package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
)

type observedProperty struct {
	Property     string `json:"property"`
	CapabilityID string `json:"capability_id,omitempty"`
	ValueType    string `json:"value_type,omitempty"`
	Unit         string `json:"unit,omitempty"`
	Access       string `json:"access"`
	Value        any    `json:"value"`
	HasValue     bool   `json:"has_value"`
	Error        string `json:"error,omitempty"`
}

type interfacesOutput struct {
	DeviceID       string                 `json:"device_id"`
	MetadataStatus string                 `json:"metadata_status"`
	MetadataReadAt time.Time              `json:"metadata_read_at"`
	StateStatus    string                 `json:"state_status"`
	StateUpdatedAt string                 `json:"state_updated_at,omitempty"`
	Observations   []observedProperty     `json:"observations"`
	Commands       []hdp.CommandInterface `json:"commands"`
	Errors         []string               `json:"errors,omitempty"`
}

func validateInterfaceDeviceID(id string) error {
	if id == "" || len(id) > 512 || strings.TrimSpace(id) != id {
		return fmt.Errorf("a canonical device_id is required")
	}
	parts := strings.Split(id, "/")
	if len(parts) < 2 || parts[0] != strings.ToLower(parts[0]) {
		return fmt.Errorf("a canonical device_id is required")
	}
	for _, part := range parts {
		if part == "" || strings.TrimSpace(part) != part {
			return fmt.Errorf("a canonical device_id is required")
		}
	}
	return nil
}

func (c *readClients) GetDeviceInterfaces(ctx context.Context, deviceID, token string) (interfacesOutput, error) {
	if err := validateInterfaceDeviceID(deviceID); err != nil {
		return interfacesOutput{}, err
	}
	var metadata struct {
		DeviceID     string          `json:"device_id"`
		Capabilities json.RawMessage `json:"capabilities"`
		Inputs       json.RawMessage `json:"inputs"`
	}
	body, err := json.Marshal(map[string]string{"device_id": deviceID})
	if err != nil {
		return interfacesOutput{}, fmt.Errorf("encode metadata lookup: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.gatewayURL, "/")+"/api/mcp/hdp/device-interfaces:lookup", bytes.NewReader(body))
	if err != nil {
		return interfacesOutput{}, fmt.Errorf("build metadata lookup: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.client.Do(request)
	if err != nil {
		return interfacesOutput{}, fmt.Errorf("perform metadata lookup: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return interfacesOutput{}, fmt.Errorf("metadata service returned %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&metadata); err != nil {
		return interfacesOutput{}, fmt.Errorf("decode metadata response: %w", err)
	}
	if metadata.DeviceID != deviceID {
		return interfacesOutput{}, fmt.Errorf("invalid device metadata response")
	}
	interfaces := hdp.DiscoverCommandInterfaces(metadata.Capabilities, metadata.Inputs)
	output := interfacesOutput{DeviceID: deviceID, MetadataStatus: interfaces.Status, MetadataReadAt: time.Now().UTC(), StateStatus: "unknown", Observations: []observedProperty{}, Commands: interfaces.Commands, Errors: interfaces.Errors}
	batch, err := c.GetDeviceStates(ctx, deviceStatesInput{DeviceIDs: []string{deviceID}, Fields: []string{"state", "state_updated_at"}}, token)
	if err != nil {
		return interfacesOutput{}, err
	}
	batchMap, ok := batch.(map[string]any)
	if !ok {
		return interfacesOutput{}, fmt.Errorf("invalid device state response")
	}
	items, ok := batchMap["results"].([]any)
	if !ok || len(items) != 1 {
		return interfacesOutput{}, fmt.Errorf("invalid device state response")
	}
	stateItem, ok := items[0].(map[string]any)
	if !ok {
		return interfacesOutput{}, fmt.Errorf("invalid device state response")
	}
	output.StateStatus, _ = stateItem["status"].(string)
	switch output.StateStatus {
	case "ok":
		output.StateUpdatedAt, _ = stateItem["state_updated_at"].(string)
		if _, err := time.Parse(time.RFC3339Nano, output.StateUpdatedAt); err != nil {
			return interfacesOutput{}, fmt.Errorf("invalid device state timestamp")
		}
	case "state_unavailable", "not_found":
	default:
		return interfacesOutput{}, fmt.Errorf("invalid device state status")
	}
	state, stateIsObject := stateItem["state"].(map[string]any)
	if output.StateStatus == "ok" && !stateIsObject {
		output.StateStatus = "state_unavailable"
		output.StateUpdatedAt = ""
		output.Errors = append(output.Errors, "device state is not an object")
	}
	properties := make(map[string]bool, len(state))
	for key := range state {
		properties[key] = true
	}
	for _, command := range interfaces.Commands {
		if command.Readable != nil && *command.Readable && command.Property != "" {
			properties[command.Property] = true
		}
	}
	if len(properties) > 128 {
		return interfacesOutput{}, fmt.Errorf("device observations exceed size limit")
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "" || len(key) > 128 || strings.TrimSpace(key) != key {
			return interfacesOutput{}, fmt.Errorf("invalid observation property")
		}
		value, hasValue := state[key]
		observation := observedProperty{Property: key, Access: "unknown", HasValue: hasValue}
		matches := 0
		for _, command := range interfaces.Commands {
			if command.Property == key {
				matches++
				observation.CapabilityID = command.CapabilityID
				observation.ValueType = command.ValueType
				observation.Unit = command.Unit
				observation.Access = command.Access
				if hasValue && command.Readable != nil && !*command.Readable {
					observation.Access = "unknown"
					observation.Error = "observed value contradicts read access"
				}
			}
		}
		if matches != 1 {
			observation.CapabilityID = ""
			observation.ValueType = ""
			observation.Unit = ""
			observation.Access = "unknown"
			observation.Error = "capability metadata unavailable or ambiguous"
		}
		if hasValue {
			encoded, err := json.Marshal(value)
			if err != nil || len(encoded) > 4096 {
				observation.HasValue = false
				observation.Error = "observed value exceeds size limit"
			} else {
				observation.Value = value
			}
		}
		output.Observations = append(output.Observations, observation)
	}
	encoded, err := json.Marshal(output)
	if err != nil || len(encoded) > 256<<10 {
		return interfacesOutput{}, fmt.Errorf("device interfaces exceed size limit")
	}
	return output, nil
}
