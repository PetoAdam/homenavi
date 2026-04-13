package adapter

import (
	"encoding/json"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
)

func (s *Service) publishHello() {
	if s.adapterID == "" {
		return
	}
	payload := map[string]any{
		"schema":      hdp.SchemaV1,
		"type":        "hello",
		"adapter_id":  s.adapterID,
		"protocol":    "thread",
		"version":     s.version,
		"hdp_version": "1.0",
		"features": map[string]any{
			"supports_ack":         true,
			"supports_correlation": true,
			"supports_batch_state": false,
			"supports_pairing":     true,
			"supports_interview":   false,
		},
		"pairing": map[string]any{
			"label":               "Thread",
			"supported":           true,
			"supports_interview":  false,
			"default_timeout_sec": 60,
			"instructions": []string{
				"Ensure the Thread border router is online.",
				"Put the Thread device into commissioning mode.",
				"We will attach it when the adapter reports the join.",
			},
			"cta_label": "Start Thread pairing",
			"notes":     "Placeholder implementation",
		},
		"ts": time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.Publish(hdp.AdapterHelloTopic, b)
	}
}

func (s *Service) publishStatus(status, reason string) {
	if s.adapterID == "" {
		return
	}
	payload := map[string]any{
		"schema":     hdp.SchemaV1,
		"type":       "status",
		"adapter_id": s.adapterID,
		"protocol":   "thread",
		"status":     status,
		"reason":     reason,
		"version":    s.version,
		"features": map[string]any{
			"supports_pairing":   true,
			"supports_interview": false,
		},
		"pairing": map[string]any{
			"label":               "Thread",
			"supported":           true,
			"supports_interview":  false,
			"default_timeout_sec": 60,
			"cta_label":           "Start Thread pairing",
			"notes":               "Placeholder implementation",
		},
		"ts": time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.PublishWith(hdp.AdapterStatusPrefix+s.adapterID, b, true)
	}
}

func (s *Service) publishCommandResult(deviceID, corr string, success bool, status, errMsg string) {
	id := s.hdpDeviceID(deviceID)
	if id == "" || corr == "" {
		return
	}
	payload := map[string]any{
		"schema":    hdp.SchemaV1,
		"type":      "command_result",
		"origin":    "adapter",
		"device_id": id,
		"corr":      corr,
		"success":   success,
		"status":    status,
		"ts":        time.Now().UnixMilli(),
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.Publish(hdp.CommandResultPrefix+id, b)
	}
}

func (s *Service) publishPairingProgress(stage, status, external string) {
	if stage == "" {
		return
	}
	payload := map[string]any{
		"schema":   hdp.SchemaV1,
		"type":     "pairing_progress",
		"protocol": "thread",
		"stage":    stage,
		"status":   status,
		"ts":       time.Now().UnixMilli(),
	}
	if external != "" {
		payload["external_id"] = external
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.Publish(hdp.PairingProgressPrefix+"thread", b)
	}
}
