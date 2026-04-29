package adapter

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/PetoAdam/homenavi/shared/hdp"
	paho "github.com/eclipse/paho.mqtt.golang"
)

func (s *Service) handleDeviceCommand(_ paho.Client, m paho.Message) {
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
	s.publishCommandResult(deviceID, corr, false, "rejected", "mock adapter placeholder does not support device commands yet")
	slog.Info("mock adapter command rejected", "device_id", deviceID, "corr", corr)
}

func (s *Service) handlePairingCommand(_ paho.Client, m paho.Message) {
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
