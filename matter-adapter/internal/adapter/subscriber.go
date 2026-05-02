package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/PetoAdam/homenavi/shared/hdp"
	paho "github.com/eclipse/paho.mqtt.golang"
)

var manualCodeSanitizer = regexp.MustCompile(`[^0-9]`)

func (s *Service) handleDeviceCommand(_ paho.Client, m paho.Message) {
	var env map[string]any
	if err := json.Unmarshal(m.Payload(), &env); err != nil {
		slog.Debug("matter hdp command decode failed", "error", err)
		return
	}
	deviceID, _ := env["device_id"].(string)
	if deviceID == "" {
		deviceID = strings.TrimPrefix(m.Topic(), hdp.CommandPrefix)
	}
	proto, external := s.externalFromHDP(deviceID)
	if proto != "" && proto != "matter" {
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

	newState, err := s.applyCommand(context.Background(), external, command, args)
	if err != nil {
		s.publishCommandResult(deviceID, corr, false, "rejected", err.Error())
		slog.Warn("matter adapter command rejected", "device_id", deviceID, "command", command, "error", err)
		return
	}
	s.publishDeviceState(external, newState, corr)
	s.publishCommandResult(deviceID, corr, true, "ok", "")
	slog.Info("matter adapter command applied", "device_id", deviceID, "command", command)
}

// applyCommand validates the command, executes it through the commissioner when
// available, and persists the resulting state snapshot.
func (s *Service) applyCommand(ctx context.Context, nodeID, command string, args map[string]any) (map[string]any, error) {
	raw, ok := s.devices.Load(nodeID)
	if !ok {
		return nil, fmt.Errorf("device %q not found", nodeID)
	}
	dev := raw.(matterDevice)

	state, err := applyCommandState(copyMap(dev.State), command, args)
	if err != nil {
		return nil, err
	}

	if s.commissionerAvailable() {
		response, err := s.runCommissionerCommand(ctx, nodeID, command, args)
		if err != nil {
			return nil, err
		}
		if response != nil && len(response.State) > 0 {
			for key, value := range response.State {
				state[key] = value
			}
		}
	}

	dev.State = state
	s.devices.Store(nodeID, dev)
	return state, nil
}

func applyCommandState(state map[string]any, command string, args map[string]any) (map[string]any, error) {
	switch command {
	case "turn_on":
		state["on"] = true
	case "turn_off":
		state["on"] = false
	case "toggle":
		on, _ := state["on"].(bool)
		state["on"] = !on
	case "set_level":
		if args == nil {
			return nil, fmt.Errorf("set_level requires args.level")
		}
		level, ok := toFloat64(args["level"])
		if !ok {
			return nil, fmt.Errorf("set_level: args.level must be a number 0-254")
		}
		if level < 0 || level > 254 {
			return nil, fmt.Errorf("set_level: level %v out of range [0,254]", level)
		}
		state["level"] = level
		if level > 0 {
			state["on"] = true
		}
	case "set_color_temp":
		if args == nil {
			return nil, fmt.Errorf("set_color_temp requires args.color_temp")
		}
		ct, ok := toFloat64(args["color_temp"])
		if !ok {
			return nil, fmt.Errorf("set_color_temp: args.color_temp must be a number (mireds)")
		}
		if ct < 153 || ct > 500 {
			return nil, fmt.Errorf("set_color_temp: color_temp %v out of range [153,500] mireds", ct)
		}
		state["color_temp"] = ct
		state["on"] = true
	default:
		return nil, fmt.Errorf("unsupported command %q", command)
	}
	return state, nil
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func (s *Service) handlePairingCommand(_ paho.Client, m paho.Message) {
	var env map[string]any
	if err := json.Unmarshal(m.Payload(), &env); err != nil {
		slog.Debug("matter pairing decode failed", "error", err)
		return
	}
	mode := strings.TrimSpace(strings.ToLower(asString(env["mode"])))
	if mode == "" {
		mode = "qr_code"
	}
	flowID := strings.TrimSpace(asString(env["flow_id"]))
	inputs, _ := env["inputs"].(map[string]any)
	if inputs == nil {
		inputs = map[string]any{}
	}
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
	simulateFailureStage := strings.TrimSpace(strings.ToLower(asString(inputs["simulate_failure_stage"])))
	simulateTimeoutStage := strings.TrimSpace(strings.ToLower(asString(inputs["simulate_timeout_stage"])))
	networkPath := strings.TrimSpace(strings.ToLower(asString(inputs["network_path"])))
	if networkPath == "" {
		networkPath = s.defaultNetworkPath
		if networkPath == "" {
			networkPath = "on_network"
		}
	}
	inputs["network_path"] = networkPath
	if (mode == "on_network" || networkPath == "on_network") && strings.TrimSpace(asString(inputs["commissioning_interface"])) == "" && strings.TrimSpace(s.commissioningInterface) != "" {
		inputs["commissioning_interface"] = s.commissioningInterface
	}
	extra["network_path"] = networkPath
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
			onboardingPayload := strings.TrimSpace(asString(inputs["onboarding_payload"]))
			if onboardingPayload == "" {
				needsInput := map[string]any{}
				for key, value := range extra {
					needsInput[key] = value
				}
				needsInput["message"] = "Onboarding payload is required for qr_code mode"
				needsInput["error_code"] = "ONBOARDING_PAYLOAD_MISSING"
				needsInput["required_inputs"] = []string{"onboarding_payload"}
				s.publishPairingProgress("discovery", "needs_input", "", needsInput)
				return
			}
			if !strings.HasPrefix(strings.ToUpper(onboardingPayload), "MT:") {
				failed := map[string]any{}
				for key, value := range extra {
					failed[key] = value
				}
				failed["message"] = "Onboarding payload format is invalid"
				failed["error_code"] = "ONBOARDING_PAYLOAD_INVALID"
				s.publishPairingProgress("discovery", "failed", "", failed)
				return
			}
		}

		if mode == "manual_code" || mode == "on_network" {
			manualCode := normalizeManualCode(inputs["manual_code"])
			if manualCode == "" {
				needsInput := map[string]any{}
				for key, value := range extra {
					needsInput[key] = value
				}
				needsInput["message"] = "Manual setup code is required"
				needsInput["error_code"] = "MANUAL_CODE_MISSING"
				needsInput["required_inputs"] = []string{"manual_code"}
				s.publishPairingProgress("discovery", "needs_input", "", needsInput)
				return
			}
			inputs["manual_code"] = manualCode
			if len(manualCode) < 8 || len(manualCode) > 11 {
				failed := map[string]any{}
				for key, value := range extra {
					failed[key] = value
				}
				failed["message"] = "Manual setup code format is invalid"
				failed["error_code"] = "MANUAL_CODE_INVALID"
				s.publishPairingProgress("discovery", "failed", "", failed)
				return
			}
		}

		threadDataset := strings.TrimSpace(asString(inputs["thread_operational_dataset"]))
		if networkPath == "thread" && strings.EqualFold(strings.TrimSpace(s.threadDatasetSource), "otbr") {
			diagnostics, err := s.fetchOTBRDiagnostics(context.Background())
			if err != nil {
				failed := map[string]any{}
				for key, value := range extra {
					failed[key] = value
				}
				failed["message"] = fmt.Sprintf("Unable to reach OTBR controller at %s", strings.TrimSpace(s.otbrBaseURL))
				failed["error_code"] = "BORDER_ROUTER_UNAVAILABLE"
				s.publishPairingProgress("network_provisioning", "failed", "", failed)
				return
			}
			expectedState := strings.TrimSpace(strings.ToLower(s.otbrExpectedState))
			if expectedState != "" && diagnostics.State != "" && diagnostics.State != expectedState {
				failed := map[string]any{}
				for key, value := range extra {
					failed[key] = value
				}
				failed["message"] = fmt.Sprintf("OTBR state %q does not match expected state %q", diagnostics.State, expectedState)
				failed["error_code"] = "BORDER_ROUTER_UNAVAILABLE"
				s.publishPairingProgress("network_provisioning", "failed", "", failed)
				return
			}
			if threadDataset == "" && diagnostics.Dataset != "" {
				threadDataset = diagnostics.Dataset
				inputs["thread_operational_dataset"] = threadDataset
				extra["inputs"] = inputs
			}
		}
		if networkPath == "thread" && threadDataset == "" && strings.TrimSpace(s.threadOperationalDataset) != "" {
			threadDataset = strings.TrimSpace(s.threadOperationalDataset)
			inputs["thread_operational_dataset"] = threadDataset
			extra["inputs"] = inputs
		}
		if networkPath == "thread" && threadDataset == "" {
			needsInput := map[string]any{}
			for key, value := range extra {
				needsInput[key] = value
			}
			needsInput["message"] = "Thread dataset is required for selected path"
			needsInput["error_code"] = "THREAD_DATASET_MISSING"
			needsInput["required_inputs"] = []string{"thread_operational_dataset"}
			s.publishPairingProgress("network_provisioning", "needs_input", "", needsInput)
			return
		}
		if networkPath == "thread" && !strings.HasPrefix(strings.ToLower(threadDataset), "hex:") {
			failed := map[string]any{}
			for key, value := range extra {
				failed[key] = value
			}
			failed["message"] = "Thread dataset must use hex: format"
			failed["error_code"] = "THREAD_DATASET_INVALID"
			s.publishPairingProgress("network_provisioning", "failed", "", failed)
			return
		}
		if asBool(inputs["simulate_border_router_unavailable"]) {
			failed := map[string]any{}
			for key, value := range extra {
				failed[key] = value
			}
			failed["message"] = "Thread border router is unavailable"
			failed["error_code"] = "BORDER_ROUTER_UNAVAILABLE"
			s.publishPairingProgress("network_provisioning", "failed", "", failed)
			return
		}

		inProgress := map[string]any{}
		for key, value := range extra {
			inProgress[key] = value
		}
		if simulateFailureStage != "" {
			failed := copyMap(inProgress)
			failed["message"] = "Simulated commissioning failure"
			failed["error_code"] = strings.ToUpper(strings.ReplaceAll(simulateFailureStage, "-", "_")) + "_FAILED"
			s.publishPairingProgress(simulateFailureStage, "failed", "", failed)
			return
		}
		if simulateTimeoutStage != "" {
			timeout := copyMap(inProgress)
			timeout["message"] = "Simulated commissioning timeout"
			timeout["error_code"] = strings.ToUpper(strings.ReplaceAll(simulateTimeoutStage, "-", "_")) + "_TIMEOUT"
			s.publishPairingProgress(simulateTimeoutStage, "timeout", "", timeout)
			return
		}
		if s.commissionerAvailable() {
			inProgress["message"] = "Matter discovery started"
			s.publishPairingProgress("discovery", "in_progress", "", inProgress)
			ctx, cancel := context.WithCancel(context.Background())
			s.setActivePairingCancel(cancel)
			response, err := s.runCommissioner(ctx, mode, flowID, inputs)
			s.setActivePairingCancel(nil)
			cancel()
			if err != nil {
				status := "failed"
				errorCode := "COMMISSIONER_EXECUTION_FAILED"
				message := err.Error()
				if err == context.Canceled {
					status = "stopped"
					errorCode = "PAIRING_STOPPED"
					message = "Pairing stopped"
				}
				failed := copyMap(inProgress)
				failed["message"] = message
				failed["error_code"] = errorCode
				s.publishPairingProgress("commissioning_complete", status, "", failed)
				return
			}

			nodeID := strings.TrimSpace(response.ExternalID)
			if nodeID == "" {
				nodeID = "matter-device-001"
			}
			devMeta := matterDevice{
				ExternalID:   nodeID,
				Type:         asString(response.Metadata["type"]),
				Manufacturer: asString(response.Metadata["manufacturer"]),
				Model:        asString(response.Metadata["model"]),
				Icon:         asString(response.Metadata["icon"]),
				State:        response.State,
			}
			if devMeta.Type == "" {
				devMeta.Type = "light"
			}
			if devMeta.Manufacturer == "" {
				devMeta.Manufacturer = "Matter"
			}
			if devMeta.Model == "" {
				devMeta.Model = "Commissioned Device"
			}
			if devMeta.Icon == "" {
				devMeta.Icon = "lightbulb"
			}
			if len(devMeta.State) == 0 {
				devMeta.State = map[string]any{"on": false}
			}
			s.devices.Store(nodeID, devMeta)
			s.publishDeviceMetadata(nodeID, devMeta)
			s.publishDeviceState(nodeID, devMeta.State, "")

			completed := map[string]any{}
			for key, value := range extra {
				completed[key] = value
			}
			completed["message"] = response.Message
			completed["device_id"] = s.hdpDeviceID(nodeID)
			completed["metadata"] = response.Metadata
			s.publishPairingProgress("commissioning_complete", "completed", nodeID, completed)
			return
		}
		inProgress["message"] = "Matter discovery started"
		s.publishPairingProgress("discovery", "in_progress", "matter-device-001", inProgress)

		pase := copyMap(inProgress)
		pase["message"] = "PASE established"
		s.publishPairingProgress("pase", "in_progress", "matter-device-001", pase)

		attestation := copyMap(inProgress)
		attestation["message"] = "Device attestation in progress"
		s.publishPairingProgress("attestation", "in_progress", "matter-device-001", attestation)

		noc := copyMap(inProgress)
		noc["message"] = "Operational credentials (NOC) provisioning"
		s.publishPairingProgress("noc", "in_progress", "matter-device-001", noc)

		if networkPath == "thread" {
			networkProvisioning := copyMap(inProgress)
			networkProvisioning["message"] = "Thread network provisioning"
			s.publishPairingProgress("network_provisioning", "in_progress", "matter-device-001", networkProvisioning)
		}

		operationalDiscovery := copyMap(inProgress)
		operationalDiscovery["message"] = "Operational discovery in progress"
		s.publishPairingProgress("operational_discovery", "in_progress", "matter-device-001", operationalDiscovery)

		caseStage := copyMap(inProgress)
		caseStage["message"] = "CASE session established"
		s.publishPairingProgress("case", "in_progress", "matter-device-001", caseStage)

		nodeID := "matter-device-001"
		devMeta := matterDevice{
			ExternalID:   nodeID,
			Type:         "light",
			Manufacturer: "Matter",
			Model:        "MVP Device",
			Icon:         "lightbulb",
			State:        map[string]any{"on": false, "level": 254, "color_temp": 370},
		}
		s.devices.Store(nodeID, devMeta)
		s.publishDeviceMetadata(nodeID, devMeta)
		s.publishDeviceState(nodeID, devMeta.State, "")

		completed := map[string]any{}
		for key, value := range extra {
			completed[key] = value
		}
		completed["message"] = "Matter commissioning complete"
		completed["device_id"] = s.hdpDeviceID(nodeID)
		completed["metadata"] = map[string]any{
			"type":         devMeta.Type,
			"manufacturer": devMeta.Manufacturer,
			"model":        devMeta.Model,
			"icon":         devMeta.Icon,
		}
		s.publishPairingProgress("commissioning_complete", "completed", nodeID, completed)
	case "stop":
		s.cancelActivePairing()
		s.publishPairingProgress("stopped", "stopped", "", extra)
	}
}

func copyMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func asString(v any) string {
	if value, ok := v.(string); ok {
		return value
	}
	return ""
}

func asBool(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		normalized := strings.TrimSpace(strings.ToLower(value))
		return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
	default:
		return false
	}
}

func normalizeManualCode(v any) string {
	if strings.TrimSpace(asString(v)) == "" {
		return ""
	}
	return manualCodeSanitizer.ReplaceAllString(asString(v), "")
}
