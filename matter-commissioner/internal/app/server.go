package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type PairRequest struct {
	Protocol   string                 `json:"protocol"`
	Mode       string                 `json:"mode"`
	FlowID     string                 `json:"flow_id,omitempty"`
	Inputs     map[string]any         `json:"inputs,omitempty"`
	Controller map[string]any         `json:"controller,omitempty"`
}

type CommandRequest struct {
	Protocol string         `json:"protocol"`
	DeviceID string         `json:"device_id"`
	Command  string         `json:"command"`
	Args     map[string]any `json:"args,omitempty"`
}

type PairResponse struct {
	ExternalID string                 `json:"external_id,omitempty"`
	DeviceID   string                 `json:"device_id,omitempty"`
	Message    string                 `json:"message,omitempty"`
	Metadata   map[string]any         `json:"metadata,omitempty"`
	State      map[string]any         `json:"state,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func Run(cfg Config) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/pair", pairHandler(cfg))
	mux.HandleFunc("/command", commandHandler(cfg))

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("matter commissioner started", "addr", server.Addr, "backend_configured", cfg.BackendCommand != "")
	return server.ListenAndServe()
}

func commandHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}

		var request CommandRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request payload"})
			return
		}

		response, err := commandDevice(r.Context(), cfg, request)
		if err != nil {
			slog.Warn("matter commissioner command failed", "error", err)
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func pairHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}

		var request PairRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request payload"})
			return
		}

		response, err := commission(r.Context(), cfg, request)
		if err != nil {
			slog.Warn("matter commissioner pairing failed", "error", err)
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func commission(ctx context.Context, cfg Config, request PairRequest) (PairResponse, error) {
	if strings.TrimSpace(cfg.BackendCommand) != "" {
		return runBackend(ctx, cfg, "pair", request)
	}
	return stubResponse(request), nil
}

func commandDevice(ctx context.Context, cfg Config, request CommandRequest) (PairResponse, error) {
	if strings.TrimSpace(cfg.BackendCommand) != "" {
		return runBackend(ctx, cfg, "command", request)
	}
	return stubCommandResponse(request)
}

func runBackend(ctx context.Context, cfg Config, subcommand string, request any) (PairResponse, error) {
	timeout := cfg.BackendTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := json.Marshal(request)
	if err != nil {
		return PairResponse{}, err
	}

	cmd := exec.CommandContext(ctx, cfg.BackendCommand, append(append([]string(nil), cfg.BackendArgs...), subcommand)...)
	cmd.Stdin = strings.NewReader(string(payload))
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return PairResponse{}, ctx.Err()
		}
		return PairResponse{}, errors.New(classifyBackendError(subcommand, output, err))
	}

	var response PairResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return PairResponse{}, fmt.Errorf("decode backend response: %w", err)
	}
	return normalizeResponse(response), nil
}

func classifyBackendError(subcommand string, output []byte, runErr error) string {
	clean := sanitizeBackendError(output)
	if clean == "" && runErr != nil {
		clean = strings.TrimSpace(runErr.Error())
	}
	if subcommand == "pair" {
		if strings.Contains(clean, "BLE adapter unavailable") && strings.Contains(clean, "Discovery timed out") {
			return "BLE adapter unavailable in commissioner runtime; Thread onboarding requires BLE discovery, so commissionable node discovery timed out"
		}
		if strings.Contains(clean, "BLE adapter unavailable") {
			return "BLE adapter unavailable in commissioner runtime"
		}
		if strings.Contains(clean, "Long discriminator is required") {
			return "On-network commissioning requires the long discriminator, which is not available from manual code alone; use QR/onboarding payload, provide the discriminator explicitly, or switch to Manual Code mode for BLE commissioning"
		}
		if strings.Contains(clean, "Integrity check failed") {
			return "The setup code or onboarding payload failed validation"
		}
		if strings.Contains(clean, "Discovery timed out") {
			return "Commissionable node discovery timed out"
		}
	}
	if clean == "" {
		return "commissioner backend execution failed"
	}
	return clean
}

func sanitizeBackendError(output []byte) string {
	clean := ansiEscapePattern.ReplaceAllString(string(output), "")
	lines := strings.Split(clean, "\n")
	trimmed := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		trimmed = append(trimmed, line)
	}
	return strings.Join(trimmed, "\n")
}

func stubResponse(request PairRequest) PairResponse {
	networkPath := asString(request.Inputs["network_path"])
	if networkPath == "" {
		networkPath = "on_network"
	}
	externalID := stableExternalID(request)
	return normalizeResponse(PairResponse{
		ExternalID: externalID,
		DeviceID:   externalID,
		Message:    "Matter commissioning complete",
		Metadata: map[string]any{
			"type":         "light",
			"manufacturer": "Matter",
			"model":        "Commissioned Device",
			"icon":         "lightbulb",
			"network_path": networkPath,
		},
		State: map[string]any{"on": false},
	})
}

func stubCommandResponse(request CommandRequest) (PairResponse, error) {
	deviceID := strings.TrimSpace(request.DeviceID)
	if deviceID == "" {
		return PairResponse{}, errors.New("device_id is required")
	}
	state, err := stubCommandState(request.Command, request.Args)
	if err != nil {
		return PairResponse{}, err
	}
	return normalizeResponse(PairResponse{
		ExternalID: deviceID,
		DeviceID:   deviceID,
		Message:    fmt.Sprintf("Matter command %s complete", strings.TrimSpace(request.Command)),
		State:      state,
	}), nil
}

func normalizeResponse(response PairResponse) PairResponse {
	if strings.TrimSpace(response.ExternalID) == "" {
		response.ExternalID = response.DeviceID
	}
	if strings.TrimSpace(response.DeviceID) == "" {
		response.DeviceID = response.ExternalID
	}
	if len(response.Metadata) == 0 {
		response.Metadata = map[string]any{}
	}
	if strings.TrimSpace(asString(response.Metadata["type"])) == "" {
		response.Metadata["type"] = "light"
	}
	if strings.TrimSpace(asString(response.Metadata["manufacturer"])) == "" {
		response.Metadata["manufacturer"] = "Matter"
	}
	if strings.TrimSpace(asString(response.Metadata["model"])) == "" {
		response.Metadata["model"] = "Commissioned Device"
	}
	if strings.TrimSpace(asString(response.Metadata["icon"])) == "" {
		response.Metadata["icon"] = "lightbulb"
	}
	if len(response.State) == 0 {
		response.State = map[string]any{"on": false}
	}
	if strings.TrimSpace(response.Message) == "" {
		response.Message = "Matter commissioning complete"
	}
	return response
}

func stubCommandState(command string, args map[string]any) (map[string]any, error) {
	state := map[string]any{}
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "turn_on":
		state["on"] = true
	case "turn_off":
		state["on"] = false
	case "toggle":
		state["on"] = true
	case "set_level":
		value, err := numericStubArg(args, "level")
		if err != nil {
			return nil, err
		}
		state["level"] = value
		state["on"] = true
	case "set_color_temp":
		value, err := numericStubArg(args, "color_temp")
		if err != nil {
			return nil, err
		}
		state["color_temp"] = value
		state["on"] = true
	default:
		return nil, fmt.Errorf("unsupported command %q", command)
	}
	return state, nil
}

func numericStubArg(args map[string]any, key string) (float64, error) {
	if args == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	raw, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	switch value := raw.(type) {
	case float64:
		return value, nil
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case json.Number:
		return value.Float64()
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be numeric", key)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be numeric", key)
	}
}

func stableExternalID(request PairRequest) string {
	parts := []string{
		strings.TrimSpace(request.Protocol),
		strings.TrimSpace(request.Mode),
		strings.TrimSpace(request.FlowID),
		asString(request.Inputs["network_path"]),
		asString(request.Inputs["manual_code"]),
		asString(request.Inputs["onboarding_payload"]),
	}
	seed := strings.Join(parts, "|")
	if strings.Trim(seed, "|") == "" {
		seed = fmt.Sprintf("fallback|%d", time.Now().UnixNano())
	}
	sum := sha1.Sum([]byte(seed))
	return "matter-" + hex.EncodeToString(sum[:6])
}

func asString(value any) string {
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}