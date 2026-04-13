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
		slog.Debug("thread hdp command decode failed", "error", err)
		return
	}
	deviceID, _ := env["device_id"].(string)
	if deviceID == "" {
		deviceID = strings.TrimPrefix(m.Topic(), hdp.CommandPrefix)
	}
	proto, external := s.externalFromHDP(deviceID)
	if proto != "" && proto != "thread" {
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
	s.publishCommandResult(deviceID, corr, false, "rejected", "thread adapter placeholder does not support device commands yet")
	slog.Info("thread adapter command rejected", "device_id", deviceID, "corr", corr)
}

func (s *Service) handlePairingCommand(_ paho.Client, m paho.Message) {
	var env map[string]any
	if err := json.Unmarshal(m.Payload(), &env); err != nil {
		slog.Debug("thread pairing decode failed", "error", err)
		return
	}
	actionVal, _ := env["action"].(string)
	action := strings.ToLower(strings.TrimSpace(actionVal))
	if action == "" && strings.Contains(strings.ToLower(m.Topic()), "start") {
		action = "start"
	}
	switch action {
	case "start":
		s.publishPairingProgress("commissioning", "in_progress", "")
		s.publishPairingProgress("completed", "completed", "")
	case "stop":
		s.publishPairingProgress("stopped", "stopped", "")
	}
}
