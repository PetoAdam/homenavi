package adapter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
)

func (s *Service) pairingInstructions() []string {
	instructions := []string{
		"Select QR, manual code, or on-network commissioning mode.",
		"On-network is the default path for first-time validation.",
	}
	if s.enableThread {
		if s.threadOperationalDataset != "" {
			instructions = append(instructions, "Thread path can use the configured OTBR dataset automatically.")
		} else {
			instructions = append(instructions, "For Thread path, provide a thread operational dataset.")
		}
	}
	if s.otbrBaseURL != "" {
		instructions = append(instructions, fmt.Sprintf("OTBR controller is configured at %s.", s.otbrBaseURL))
	}
	instructions = append(instructions, "Synthetic commissioning stages are published for integration testing.")
	return instructions
}

func (s *Service) pairingNotes() string {
	notes := []string{"Matter commissioning with on/off, level, and color-temp command support"}
	if s.otbrExpectedState != "" {
		notes = append(notes, fmt.Sprintf("OTBR expected state: %s", s.otbrExpectedState))
	}
	if s.threadBorderRouterHost != "" {
		port := s.threadBorderRouterPort
		if port <= 0 {
			port = 80
		}
		notes = append(notes, fmt.Sprintf("Thread border router: %s:%d", s.threadBorderRouterHost, port))
	}
	return strings.Join(notes, ". ")
}

func (s *Service) pairingFlow() map[string]any {
	defaultNetworkPath := s.defaultNetworkPath
	if defaultNetworkPath == "" {
		defaultNetworkPath = "on_network"
	}
	interfaceDefault := s.commissioningInterface
	if interfaceDefault == "" {
		interfaceDefault = "auto"
	}
	threadField := map[string]any{
		"id":          "thread_operational_dataset",
		"component":   "text",
		"label":       "Thread operational dataset",
		"placeholder": "hex:...",
	}
	if s.threadOperationalDataset != "" {
		threadField["description"] = fmt.Sprintf("Preconfigured from %s dataset source.", strings.TrimSpace(s.threadDatasetSource))
	}
	return map[string]any{
		"id":          "matter-commissioning-v1",
		"entry_modes": []string{"qr_code", "manual_code", "on_network"},
		"capabilities": map[string]any{
			"camera_scan":                             true,
			"manual_code":                             true,
			"thread_dataset_required_for_thread_path": s.threadOperationalDataset == "",
			"thread_dataset_preconfigured":            s.threadOperationalDataset != "",
			"on_network_discovery":                    s.enableOnNetwork,
			"ble_enabled":                             s.enableBLE,
			"thread_enabled":                          s.enableThread,
			"otbr_rest_configured":                    s.otbrBaseURL != "",
		},
		"controller": map[string]any{
			"mode":                      "thread_otbr_on_device",
			"otbr_base_url":             s.otbrBaseURL,
			"expected_state":            s.otbrExpectedState,
			"thread_dataset_source":     s.threadDatasetSource,
			"thread_border_router":      s.threadBorderRouterHost,
			"thread_border_router_port": s.threadBorderRouterPort,
		},
		"forms": []map[string]any{
			{
				"mode":  "qr_code",
				"label": "QR Code",
				"fields": []map[string]any{
					{"id": "onboarding_payload", "component": "qr_payload", "label": "Onboarding payload", "required": true, "placeholder": "MT:..."},
					{"id": "network_path", "component": "select", "label": "Network path", "options": []map[string]any{{"value": "on_network", "label": "Direct IP / On-network"}, {"value": "thread", "label": "Thread (requires dataset)"}}, "default": defaultNetworkPath},
					threadField,
				},
			},
			{
				"mode":  "manual_code",
				"label": "Manual Code",
				"description": "Use this when you only have the printed/manual setup code. This path uses BLE discovery and does not require entering the long discriminator separately.",
				"fields": []map[string]any{
					{"id": "manual_code", "component": "text", "label": "Manual setup code", "required": true, "placeholder": "01234-5678"},
					{"id": "discriminator", "component": "number", "label": "Discriminator", "min": 0, "max": 4095, "description": "Optional for this mode. Only provide it if you already know it."},
					{"id": "network_path", "component": "select", "label": "Network path", "options": []map[string]any{{"value": "on_network", "label": "Direct IP / On-network"}, {"value": "thread", "label": "Thread (requires dataset)"}}, "default": defaultNetworkPath},
					threadField,
				},
			},
			{
				"mode":  "on_network",
				"label": "On-network",
				"description": "Use this only if you have QR payload or the explicit long discriminator. Manual code alone is usually not enough for on-network discovery.",
				"fields": []map[string]any{
					{"id": "manual_code", "component": "text", "label": "Setup code", "required": true, "placeholder": "01234-5678"},
					{"id": "discriminator", "component": "number", "label": "Discriminator", "required": true, "min": 0, "max": 4095, "description": "Required for on-network commissioning when using manual setup code."},
					{"id": "commissioning_interface", "component": "text", "label": "Interface", "placeholder": interfaceDefault, "default": interfaceDefault},
					{"id": "network_path", "component": "text_block", "label": "On-network mode does not require thread dataset."},
				},
			},
		},
		"steps": []map[string]any{
			{"id": "discovery", "label": "Discovery", "stage": "discovery"},
			{"id": "pase", "label": "PASE", "stage": "pase"},
			{"id": "attestation", "label": "Attestation", "stage": "attestation"},
			{"id": "noc", "label": "Operational Credentials (NOC)", "stage": "noc"},
			{"id": "network_provisioning", "label": "Network Provisioning", "stage": "network_provisioning"},
			{"id": "operational_discovery", "label": "Operational Discovery", "stage": "operational_discovery"},
			{"id": "case", "label": "CASE", "stage": "case"},
			{"id": "commissioning_complete", "label": "Commissioning Complete", "stage": "commissioning_complete"},
		},
	}
}

func (s *Service) publishHello() {
	if s.adapterID == "" {
		return
	}
	payload := map[string]any{
		"schema":      hdp.SchemaV1,
		"type":        "hello",
		"adapter_id":  s.adapterID,
		"protocol":    "matter",
		"version":     s.version,
		"hdp_version": "1.0",
		"features": map[string]any{
			"supports_ack":         true,
			"supports_correlation": true,
			"supports_batch_state": false,
			"supports_pairing":     true,
			"supports_interview":   true,
		},
		"pairing": map[string]any{
			"schema_version":      "1.0",
			"label":               "Matter Adapter",
			"supported":           true,
			"supports_interview":  true,
			"default_timeout_sec": s.defaultTimeoutSec,
			"instructions":        s.pairingInstructions(),
			"cta_label":           "Start Matter pairing",
			"notes":               s.pairingNotes(),
			"flow":                s.pairingFlow(),
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
		"protocol":   "matter",
		"status":     status,
		"reason":     reason,
		"version":    s.version,
		"features": map[string]any{
			"supports_pairing":   true,
			"supports_interview": true,
		},
		"pairing": map[string]any{
			"schema_version":      "1.0",
			"label":               "Matter Adapter",
			"supported":           true,
			"supports_interview":  true,
			"default_timeout_sec": s.defaultTimeoutSec,
			"instructions":        s.pairingInstructions(),
			"cta_label":           "Start Matter pairing",
			"notes":               s.pairingNotes(),
			"flow":                s.pairingFlow(),
		},
		"ts": time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.PublishWith(hdp.AdapterStatusPrefix+s.adapterID, b, true)
	}
}

func (s *Service) publishDeviceMetadata(nodeID string, dev matterDevice) {
	hdpID := s.hdpDeviceID(nodeID)
	if hdpID == "" {
		return
	}
	payload := map[string]any{
		"schema":       hdp.SchemaV1,
		"type":         "metadata",
		"device_id":    hdpID,
		"protocol":     "matter",
		"manufacturer": dev.Manufacturer,
		"model":        dev.Model,
		"icon":         dev.Icon,
		"online":       true,
		"ts":           time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.PublishWith(hdp.MetadataPrefix+hdpID, b, true)
	}
}

func (s *Service) publishDeviceState(nodeID string, state map[string]any, corr string) {
	hdpID := s.hdpDeviceID(nodeID)
	if hdpID == "" || len(state) == 0 {
		return
	}
	payload := map[string]any{
		"schema":    hdp.SchemaV1,
		"type":      "state",
		"device_id": hdpID,
		"state":     state,
		"ts":        time.Now().UnixMilli(),
	}
	if corr != "" {
		payload["corr"] = corr
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.PublishWith(hdp.StatePrefix+hdpID, b, true)
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

func (s *Service) publishPairingProgress(stage, status, external string, extra map[string]any) {
	if stage == "" {
		return
	}
	payload := map[string]any{
		"schema":   hdp.SchemaV1,
		"type":     "pairing_progress",
		"protocol": "matter",
		"stage":    stage,
		"status":   status,
		"ts":       time.Now().UnixMilli(),
	}
	if external != "" {
		payload["external_id"] = external
	}
	for key, value := range extra {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		payload[key] = value
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = s.client.Publish(hdp.PairingProgressPrefix+"matter", b)
	}
}
