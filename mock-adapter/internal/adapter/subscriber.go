package adapter

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/PetoAdam/homenavi/shared/hdp"
)

var (
	errUnsupportedDevice  = errors.New("unsupported mock demo device")
	errUnsupportedCommand = errors.New("unsupported mock demo command")
	errInvalidCommand     = errors.New("invalid mock demo command payload")
)

func (s *Service) handleDeviceCommand(m Message) {
	var env map[string]any
	if err := json.Unmarshal(m.Payload(), &env); err != nil {
		slog.Debug("mock hdp command decode failed", "error", err)
		return
	}
	deviceID, _ := env["device_id"].(string)
	if deviceID == "" {
		deviceID = strings.TrimPrefix(m.Topic(), hdp.CommandPrefix)
	}
	proto, external := s.externalFromHDP(deviceID)
	if proto != "" && proto != "mock" {
		return
	}
	corr, _ := env["corr"].(string)
	if corr == "" {
		if cid, ok := env["correlation_id"].(string); ok {
			corr = cid
		}
	}
	if corr == "" {
		corr = "ack-" + strings.ReplaceAll(external, "/", "-")
	}
	command := strings.ToLower(strings.TrimSpace(asString(env["command"])))
	args, _ := env["args"].(map[string]any)
	statePatch, _ := env["state"].(map[string]any)
	if command == "" && len(args) > 0 {
		command = "set_state"
	}
	if len(args) == 0 && len(statePatch) > 0 {
		args = statePatch
		if command == "" {
			command = "set_state"
		}
	}
	switch command {
	case "refresh":
		if snapshot, ok := s.demoSnapshot(deviceID); ok {
			s.publishMetadata(snapshot)
			s.publishState(snapshot.HDPDeviceID, snapshot.State, corr)
			s.publishCommandResult(snapshot.HDPDeviceID, corr, true, "applied", "")
			return
		}
		s.publishCommandResult(deviceID, corr, false, "rejected", errUnsupportedDevice.Error())
		return
	case "set_state":
		updatedState, err := s.applyDemoStatePatch(deviceID, args)
		if err != nil {
			status := "rejected"
			if errors.Is(err, errInvalidCommand) {
				status = "failed"
			}
			s.publishCommandResult(deviceID, corr, false, status, err.Error())
			slog.Info("mock adapter command rejected", "device_id", deviceID, "corr", corr, "error", err)
			return
		}
		canonical := s.hdpDeviceID(deviceID)
		s.publishState(canonical, updatedState, corr)
		s.publishCommandResult(canonical, corr, true, "applied", "")
		slog.Info("mock adapter command applied", "device_id", canonical, "corr", corr)
		return
	default:
		s.publishCommandResult(deviceID, corr, false, "rejected", "unsupported mock command")
		slog.Info("mock adapter command rejected", "device_id", deviceID, "corr", corr, "command", command)
	}
}

func (s *Service) handlePairingCommand(m Message) {
	var env map[string]any
	if err := json.Unmarshal(m.Payload(), &env); err != nil {
		slog.Debug("mock pairing decode failed", "error", err)
		return
	}
	mode := strings.TrimSpace(strings.ToLower(asString(env["mode"])))
	flowID := strings.TrimSpace(asString(env["flow_id"]))
	inputs, _ := env["inputs"].(map[string]any)
	extra := map[string]any{}
	if mode != "" {
		extra["mode"] = mode
	}
	if flowID != "" {
		extra["flow_id"] = flowID
	}
	if len(inputs) > 0 {
		extra["inputs"] = inputs
	}
	actionVal, _ := env["action"].(string)
	action := strings.ToLower(strings.TrimSpace(actionVal))
	if action == "" && strings.Contains(strings.ToLower(m.Topic()), "start") {
		action = "start"
	}
	switch action {
	case "start":
		if mode == "qr_code" {
			if strings.TrimSpace(asString(inputs["onboarding_payload"])) == "" {
				needsInput := map[string]any{}
				for key, value := range extra {
					needsInput[key] = value
				}
				needsInput["message"] = "Onboarding payload is required for qr_code mode"
				needsInput["error_code"] = "ONBOARDING_PAYLOAD_MISSING"
				needsInput["required_inputs"] = []string{"onboarding_payload"}
				s.publishPairingProgress("commissioning", "needs_input", "", needsInput)
				return
			}
		}
		inProgress := map[string]any{}
		for key, value := range extra {
			inProgress[key] = value
		}
		inProgress["message"] = "Mock commissioning in progress"
		s.publishPairingProgress("commissioning", "in_progress", "mock-device-001", inProgress)

		completed := map[string]any{}
		for key, value := range extra {
			completed[key] = value
		}
		completed["message"] = "Mock commissioning complete"
		completed["metadata"] = map[string]any{
			"type":         "light",
			"manufacturer": "Mock",
			"model":        "Adapter Device",
			"icon":         "lightbulb",
		}
		s.publishPairingProgress("completed", "completed", "mock-device-001", completed)
	case "stop":
		s.publishPairingProgress("stopped", "stopped", "", extra)
	}
}

func asString(v any) string {
	if value, ok := v.(string); ok {
		return value
	}
	return ""
}

func (s *Service) demoSnapshot(deviceID string) (demoSnapshot, bool) {
	canonical := normalizeDemoHDPDeviceID(deviceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	device, ok := s.devices[canonical]
	if !ok {
		return demoSnapshot{}, false
	}
	return demoSnapshot{
		HDPDeviceID:  device.HDPDeviceID,
		Type:         device.Type,
		Manufacturer: device.Manufacturer,
		Model:        device.Model,
		Description:  device.Description,
		Icon:         device.Icon,
		Capabilities: cloneArrayOfMaps(device.Capabilities),
		Inputs:       cloneArrayOfMaps(device.Inputs),
		State:        cloneAnyMap(device.State),
	}, true
}
