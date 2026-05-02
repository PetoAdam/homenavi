package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var manualCodeSanitizer = regexp.MustCompile(`[^0-9]`)
var passcodePattern = regexp.MustCompile(`(?m)Passcode:\s+([0-9]+)`)
var discriminatorPattern = regexp.MustCompile(`(?m)(?:Short|Long) discriminator:\s+([0-9]+)`)

type pairRequest struct {
	Protocol string         `json:"protocol"`
	Mode     string         `json:"mode"`
	FlowID   string         `json:"flow_id,omitempty"`
	Inputs   map[string]any `json:"inputs,omitempty"`
}

type commandRequest struct {
	Protocol string         `json:"protocol"`
	DeviceID string         `json:"device_id"`
	Command  string         `json:"command"`
	Args     map[string]any `json:"args,omitempty"`
}

type pairResponse struct {
	ExternalID string         `json:"external_id,omitempty"`
	DeviceID   string         `json:"device_id,omitempty"`
	Message    string         `json:"message,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	State      map[string]any `json:"state,omitempty"`
}

type config struct {
	Binary                    string
	StorageDir                string
	CommissionerName          string
	CommissionerNodeID        string
	PAATrustStorePath         string
	CDTrustStorePath          string
	BypassAttestationVerifier bool
	ExtraArgs                 []string
}

func main() {
	if len(os.Args) < 2 {
		fail(fmt.Errorf("unsupported command: %s", strings.Join(os.Args[1:], " ")))
	}
	commandName := strings.TrimSpace(os.Args[1])
	switch commandName {
	case "pair":
		var request pairRequest
		if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
			fail(fmt.Errorf("decode request: %w", err))
		}

		response, err := runPair(loadConfig(), request, execCommand)
		if err != nil {
			fail(err)
		}

		if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
			fail(fmt.Errorf("encode response: %w", err))
		}
	case "command":
		var request commandRequest
		if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
			fail(fmt.Errorf("decode request: %w", err))
		}

		response, err := runCommand(loadConfig(), request, execCommand)
		if err != nil {
			fail(err)
		}

		if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
			fail(fmt.Errorf("encode response: %w", err))
		}
	default:
		fail(fmt.Errorf("unsupported command: %s", strings.Join(os.Args[1:], " ")))
	}
}

func loadConfig() config {
	return config{
		Binary:                    getEnv("CHIP_TOOL_BINARY", "/usr/bin/chip-tool"),
		StorageDir:                getEnv("CHIP_TOOL_STORAGE_DIR", "/var/lib/matter-commissioner/chip-tool"),
		CommissionerName:          strings.TrimSpace(os.Getenv("CHIP_TOOL_COMMISSIONER_NAME")),
		CommissionerNodeID:        strings.TrimSpace(os.Getenv("CHIP_TOOL_COMMISSIONER_NODEID")),
		PAATrustStorePath:         strings.TrimSpace(os.Getenv("CHIP_TOOL_PAA_TRUST_STORE_PATH")),
		CDTrustStorePath:          strings.TrimSpace(os.Getenv("CHIP_TOOL_CD_TRUST_STORE_PATH")),
		BypassAttestationVerifier: strings.EqualFold(strings.TrimSpace(os.Getenv("CHIP_TOOL_BYPASS_ATTESTATION_VERIFIER")), "true"),
		ExtraArgs:                 strings.Fields(os.Getenv("CHIP_TOOL_EXTRA_ARGS")),
	}
}

type commandRunner func(name string, args ...string) *exec.Cmd

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func runPair(cfg config, request pairRequest, runner commandRunner) (pairResponse, error) {
	if runner == nil {
		runner = execCommand
	}
	if err := ensureExecutable(cfg.Binary); err != nil {
		return pairResponse{}, err
	}
	if err := os.MkdirAll(cfg.StorageDir, 0o755); err != nil {
		return pairResponse{}, fmt.Errorf("create chip-tool storage dir: %w", err)
	}
	preparedRequest, err := preparePairRequest(cfg, request, runner)
	if err != nil {
		return pairResponse{}, err
	}

	nodeID := deriveNodeID(preparedRequest)
	args, networkPath, err := buildPairArgs(cfg, preparedRequest, nodeID)
	if err != nil {
		return pairResponse{}, err
	}

	cmd := runner(cfg.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	output := bytes.TrimSpace(append(stdout.Bytes(), stderr.Bytes()...))
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return pairResponse{}, fmt.Errorf("chip-tool pair failed: %s", message)
	}

	message := strings.TrimSpace(string(output))
	if message == "" {
		message = "Matter commissioning complete via chip-tool"
	}

	return pairResponse{
		ExternalID: nodeID,
		DeviceID:   nodeID,
		Message:    message,
		Metadata: map[string]any{
			"type":                  "light",
			"manufacturer":          "Matter",
			"model":                 "Commissioned Device",
			"icon":                  "lightbulb",
			"network_path":          networkPath,
			"commissioning_backend": "chip-tool",
		},
		State: map[string]any{"on": false},
	}, nil
}

func preparePairRequest(cfg config, request pairRequest, runner commandRunner) (pairRequest, error) {
	networkPath := normalizedInput(request.Inputs, "network_path")
	if networkPath == "" {
		networkPath = "on_network"
	}
	if networkPath != "thread" {
		return request, nil
	}
	if payload := normalizedInput(request.Inputs, "onboarding_payload"); payload != "" {
		return request, nil
	}
	manualCode := normalizeManualCode(normalizedInput(request.Inputs, "manual_code"))
	if manualCode == "" {
		return request, nil
	}
	passcode, discriminator, err := decodeManualCode(cfg, manualCode, runner)
	if err != nil {
		return pairRequest{}, err
	}
	prepared := request
	prepared.Inputs = copyInputs(request.Inputs)
	prepared.Inputs["manual_code"] = manualCode
	prepared.Inputs["setup_pin_code"] = passcode
	if normalizedInput(prepared.Inputs, "discriminator") == "" {
		prepared.Inputs["discriminator"] = discriminator
	}
	return prepared, nil
}

func runCommand(cfg config, request commandRequest, runner commandRunner) (pairResponse, error) {
	if runner == nil {
		runner = execCommand
	}
	if err := ensureExecutable(cfg.Binary); err != nil {
		return pairResponse{}, err
	}
	if err := os.MkdirAll(cfg.StorageDir, 0o755); err != nil {
		return pairResponse{}, fmt.Errorf("create chip-tool storage dir: %w", err)
	}

	nodeID, err := normalizeNodeID(request.DeviceID)
	if err != nil {
		return pairResponse{}, err
	}
	args, state, err := buildCommandArgs(cfg, request, nodeID)
	if err != nil {
		return pairResponse{}, err
	}

	cmd := runner(cfg.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	output := bytes.TrimSpace(append(stdout.Bytes(), stderr.Bytes()...))
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return pairResponse{}, fmt.Errorf("chip-tool command failed: %s", message)
	}

	message := strings.TrimSpace(string(output))
	if message == "" {
		message = fmt.Sprintf("Matter command %s complete via chip-tool", strings.TrimSpace(request.Command))
	}

	return pairResponse{
		ExternalID: nodeID,
		DeviceID:   nodeID,
		Message:    message,
		State:      state,
	}, nil
}

func buildPairArgs(cfg config, request pairRequest, nodeID string) ([]string, string, error) {
	networkPath := normalizedInput(request.Inputs, "network_path")
	if networkPath == "" {
		networkPath = "on_network"
	}
	onboardingPayload := normalizedInput(request.Inputs, "onboarding_payload")
	payload := preferredPayload(request)
	manualCode := normalizedInput(request.Inputs, "manual_code")
	manualCode = normalizeManualCode(manualCode)

	var args []string
	switch {
	case networkPath == "thread":
		dataset := normalizedInput(request.Inputs, "thread_operational_dataset")
		if dataset == "" {
			return nil, "", errors.New("thread_operational_dataset is required for thread commissioning")
		}
		if onboardingPayload != "" {
			args = []string{"pairing", "code-thread", nodeID, dataset, onboardingPayload}
		} else if setupPINCode := normalizedInput(request.Inputs, "setup_pin_code"); setupPINCode != "" {
			discriminator := normalizedInput(request.Inputs, "discriminator")
			if discriminator == "" {
				return nil, "", errors.New("discriminator is required for thread commissioning with manual setup code")
			}
			args = []string{"pairing", "ble-thread", nodeID, dataset, setupPINCode, discriminator}
		} else {
			return nil, "", errors.New("onboarding payload or manual code is required for thread commissioning")
		}
	case strings.EqualFold(strings.TrimSpace(request.Mode), "on_network"):
		if manualCode == "" {
			return nil, "", errors.New("manual_code is required for on-network commissioning")
		}
		if discriminator := normalizedInput(request.Inputs, "discriminator"); discriminator != "" {
			args = []string{"pairing", "onnetwork-long", nodeID, manualCode, discriminator}
		} else {
			args = []string{"pairing", "onnetwork", nodeID, manualCode}
		}
	default:
		if payload == "" {
			return nil, "", errors.New("onboarding payload or manual code is required for chip-tool pairing")
		}
		args = []string{"pairing", "code", nodeID, payload}
	}

	if cfg.StorageDir != "" {
		args = append(args, "--storage-directory", cfg.StorageDir)
	}
	if cfg.CommissionerName != "" {
		args = append(args, "--commissioner-name", cfg.CommissionerName)
	}
	if cfg.CommissionerNodeID != "" {
		args = append(args, "--commissioner-nodeid", cfg.CommissionerNodeID)
	}
	if cfg.PAATrustStorePath != "" {
		args = append(args, "--paa-trust-store-path", cfg.PAATrustStorePath)
	}
	if cfg.CDTrustStorePath != "" {
		args = append(args, "--cd-trust-store-path", cfg.CDTrustStorePath)
	}
	if cfg.BypassAttestationVerifier {
		args = append(args, "--bypass-attestation-verifier", "true")
	}
	args = append(args, cfg.ExtraArgs...)
	return args, networkPath, nil
}

func buildCommandArgs(cfg config, request commandRequest, nodeID string) ([]string, map[string]any, error) {
	command := strings.ToLower(strings.TrimSpace(request.Command))
	endpoint := normalizedInput(request.Args, "endpoint")
	if endpoint == "" {
		endpoint = "1"
	}
	state := map[string]any{}
	var args []string
	switch command {
	case "turn_on":
		args = []string{"onoff", "on", nodeID, endpoint}
		state["on"] = true
	case "turn_off":
		args = []string{"onoff", "off", nodeID, endpoint}
		state["on"] = false
	case "toggle":
		args = []string{"onoff", "toggle", nodeID, endpoint}
		state["on"] = true
	case "set_level":
		level, err := numericArg(request.Args, "level", 0, 254)
		if err != nil {
			return nil, nil, err
		}
		args = []string{"levelcontrol", "move-to-level", strconv.Itoa(level), "0", "0", "0", nodeID, endpoint}
		state["level"] = level
		state["on"] = level > 0
	case "set_color_temp":
		colorTemp, err := numericArg(request.Args, "color_temp", 153, 500)
		if err != nil {
			return nil, nil, err
		}
		args = []string{"colorcontrol", "move-to-color-temperature", strconv.Itoa(colorTemp), "0", "0", "0", nodeID, endpoint}
		state["color_temp"] = colorTemp
		state["on"] = true
	default:
		return nil, nil, fmt.Errorf("unsupported command %q", request.Command)
	}

	if cfg.StorageDir != "" {
		args = append(args, "--storage-directory", cfg.StorageDir)
	}
	if cfg.CommissionerName != "" {
		args = append(args, "--commissioner-name", cfg.CommissionerName)
	}
	if cfg.CommissionerNodeID != "" {
		args = append(args, "--commissioner-nodeid", cfg.CommissionerNodeID)
	}
	if cfg.PAATrustStorePath != "" {
		args = append(args, "--paa-trust-store-path", cfg.PAATrustStorePath)
	}
	if cfg.CDTrustStorePath != "" {
		args = append(args, "--cd-trust-store-path", cfg.CDTrustStorePath)
	}
	if cfg.BypassAttestationVerifier {
		args = append(args, "--bypass-attestation-verifier", "true")
	}
	args = append(args, cfg.ExtraArgs...)
	return args, state, nil
}

func preferredPayload(request pairRequest) string {
	if payload := normalizedInput(request.Inputs, "onboarding_payload"); payload != "" {
		return payload
	}
	return normalizeManualCode(normalizedInput(request.Inputs, "manual_code"))
}

func deriveNodeID(request pairRequest) string {
	seed := strings.Join([]string{
		strings.TrimSpace(request.Protocol),
		strings.TrimSpace(request.Mode),
		strings.TrimSpace(request.FlowID),
		normalizedInput(request.Inputs, "network_path"),
		normalizedInput(request.Inputs, "manual_code"),
		normalizedInput(request.Inputs, "onboarding_payload"),
	}, "|")
	if strings.Trim(seed, "|") == "" {
		seed = "matter-default-node"
	}
	sum := sha1.Sum([]byte(seed))
	value := binary.BigEndian.Uint64(sum[:8])
	value = 100000 + (value % 9000000000)
	return fmt.Sprintf("%d", value)
}

func normalizedInput(inputs map[string]any, key string) string {
	if inputs == nil {
		return ""
	}
	value, ok := inputs[key]
	if !ok {
		return ""
	}
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func numericArg(inputs map[string]any, key string, min, max int) (int, error) {
	value := normalizedInput(inputs, key)
	if value == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be numeric", key)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("%s %d out of range [%d,%d]", key, parsed, min, max)
	}
	return parsed, nil
}

func normalizeNodeID(deviceID string) (string, error) {
	trimmed := strings.Trim(strings.TrimSpace(deviceID), "/")
	if trimmed == "" {
		return "", errors.New("device_id is required")
	}
	parts := strings.Split(trimmed, "/")
	nodeID := strings.TrimSpace(parts[len(parts)-1])
	if nodeID == "" {
		return "", errors.New("device_id is invalid")
	}
	if _, err := strconv.ParseUint(nodeID, 10, 64); err != nil {
		return "", fmt.Errorf("device_id %q is not a numeric Matter node id", deviceID)
	}
	return nodeID, nil
}

func decodeManualCode(cfg config, manualCode string, runner commandRunner) (string, string, error) {
	if runner == nil {
		runner = execCommand
	}
	cmd := runner(cfg.Binary, "payload", "parse-setup-payload", manualCode)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := stdout.String() + stderr.String()
	if err != nil {
		message := strings.TrimSpace(output)
		if strings.Contains(message, "Integrity check failed") {
			return "", "", errors.New("manual setup code failed validation")
		}
		if message == "" {
			message = err.Error()
		}
		return "", "", fmt.Errorf("decode manual setup code: %s", message)
	}
	passcodeMatch := passcodePattern.FindStringSubmatch(output)
	discriminatorMatch := discriminatorPattern.FindStringSubmatch(output)
	if len(passcodeMatch) < 2 || len(discriminatorMatch) < 2 {
		return "", "", errors.New("manual setup code decode did not return passcode and discriminator")
	}
	return strings.TrimSpace(passcodeMatch[1]), strings.TrimSpace(discriminatorMatch[1]), nil
}

func copyInputs(inputs map[string]any) map[string]any {
	if len(inputs) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(inputs))
	for key, value := range inputs {
		cloned[key] = value
	}
	return cloned
}

func ensureExecutable(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("CHIP_TOOL_BINARY is not configured")
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return fmt.Errorf("chip-tool not found at %s: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("stat chip-tool binary: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("chip-tool path %s is a directory", resolved)
	}
	return nil
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func normalizeManualCode(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return manualCodeSanitizer.ReplaceAllString(strings.TrimSpace(value), "")
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}