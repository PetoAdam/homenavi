package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	mqttinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/mqtt"
)

const defaultCommandLifecycleTimeout = 45 * time.Second

const (
	commandStatusAccepted   = "accepted"
	commandStatusQueued     = "queued"
	commandStatusInProgress = "in_progress"
	commandStatusApplied    = "applied"
	commandStatusRejected   = "rejected"
	commandStatusFailed     = "failed"
	commandStatusTimeout    = "timeout"
)

type pendingCommand struct {
	DeviceID   string
	Corr       string
	Expected   map[string]any
	Baseline   map[string]any
	StartedAt  int64
	LastStatus string
	Timer      *time.Timer
}

func (s *Server) loadBaselineState(ctx context.Context, deviceUUID string) map[string]any {
	if s == nil || s.repo == nil || strings.TrimSpace(deviceUUID) == "" {
		return nil
	}
	raw, err := s.repo.GetDeviceState(ctx, deviceUUID)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil
	}
	return cloneAnyMap(state)
}

func (s *Server) beginCommandLifecycle(deviceID, corr string, expected, baseline map[string]any) {
	if s == nil || strings.TrimSpace(deviceID) == "" || strings.TrimSpace(corr) == "" {
		return
	}
	entry := &pendingCommand{
		DeviceID: strings.TrimSpace(deviceID),
		Corr:     strings.TrimSpace(corr),
		Expected: cloneAnyMap(expected),
		Baseline: cloneAnyMap(baseline),
		StartedAt: time.Now().UnixMilli(),
	}
	entry.Timer = time.AfterFunc(s.commandTimeout, func() {
		s.handleCommandTimeout(entry.DeviceID, entry.Corr)
	})

	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	if prev := s.commandsByDevice[entry.DeviceID]; prev != nil {
		if prev.Timer != nil {
			prev.Timer.Stop()
		}
		delete(s.commandsByCorr, prev.Corr)
	}
	s.commandsByDevice[entry.DeviceID] = entry
	s.commandsByCorr[entry.Corr] = entry
}

func (s *Server) handleCommandTimeout(deviceID, corr string) {
	entry := s.removePendingCommand(deviceID, corr)
	if entry == nil {
		return
	}
	s.publishCommandLifecycle(entry.DeviceID, entry.Corr, commandStatusTimeout, "command lifecycle timed out", nil)
}

func (s *Server) removePendingCommand(deviceID, corr string) *pendingCommand {
	if s == nil {
		return nil
	}
	deviceID = strings.TrimSpace(deviceID)
	corr = strings.TrimSpace(corr)

	s.commandMu.Lock()
	defer s.commandMu.Unlock()

	var entry *pendingCommand
	if corr != "" {
		entry = s.commandsByCorr[corr]
	}
	if entry == nil && deviceID != "" {
		entry = s.commandsByDevice[deviceID]
	}
	if entry == nil {
		return nil
	}
	if deviceID != "" && entry.DeviceID != deviceID {
		return nil
	}
	if corr != "" && entry.Corr != corr {
		return nil
	}
	if entry.Timer != nil {
		entry.Timer.Stop()
	}
	delete(s.commandsByCorr, entry.Corr)
	delete(s.commandsByDevice, entry.DeviceID)
	return entry
}

func (s *Server) getPendingCommand(deviceID, corr string) *pendingCommand {
	if s == nil {
		return nil
	}
	deviceID = strings.TrimSpace(deviceID)
	corr = strings.TrimSpace(corr)

	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	if corr != "" {
		if entry := s.commandsByCorr[corr]; entry != nil {
			return entry
		}
	}
	if deviceID != "" {
		return s.commandsByDevice[deviceID]
	}
	return nil
}

func (s *Server) ensurePassivePendingCommand(deviceID, corr string) *pendingCommand {
	if existing := s.getPendingCommand(deviceID, corr); existing != nil {
		return existing
	}
	s.beginCommandLifecycle(deviceID, corr, nil, nil)
	return s.getPendingCommand(deviceID, corr)
}

func (s *Server) publishCommandLifecycle(deviceID, corr, status, errMsg string, extra map[string]any) {
	if s == nil || s.mqtt == nil {
		return
	}
	status = normalizeLifecycleStatus(status, errMsg == "")
	if status == "" {
		return
	}
	payload := map[string]any{
		"schema":    hdpSchema,
		"type":      "command_result",
		"origin":    "device-hub",
		"device_id": strings.TrimSpace(deviceID),
		"corr":      strings.TrimSpace(corr),
		"status":    status,
		"success":   !isFailureLifecycleStatus(status),
		"terminal":  isTerminalLifecycleStatus(status),
		"ts":        time.Now().UnixMilli(),
	}
	if strings.TrimSpace(errMsg) != "" {
		payload["error"] = strings.TrimSpace(errMsg)
	}
	for key, value := range extra {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		payload[key] = value
	}
	if data, err := json.Marshal(payload); err == nil {
		if err := s.mqtt.Publish(hdpCommandResultPrefix+strings.TrimSpace(deviceID), data); err != nil {
			slog.Warn("hdp command lifecycle publish failed", "device_id", deviceID, "corr", corr, "status", status, "error", err)
		}
	}
}

func normalizeLifecycleStatus(raw string, success bool) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case commandStatusAccepted:
		return commandStatusAccepted
	case commandStatusQueued, "scheduled", "refreshing":
		return commandStatusQueued
	case commandStatusInProgress, "processing", "applying", "running":
		return commandStatusInProgress
	case commandStatusApplied, "completed", "complete", "done", "removed":
		return commandStatusApplied
	case commandStatusRejected, "invalid", "unsupported":
		return commandStatusRejected
	case commandStatusFailed, "error", "publish_failed", "not_found":
		return commandStatusFailed
	case commandStatusTimeout:
		return commandStatusTimeout
	case "":
		if success {
			return commandStatusQueued
		}
		return commandStatusFailed
	default:
		if success {
			return commandStatusInProgress
		}
		return commandStatusFailed
	}
}

func isTerminalLifecycleStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case commandStatusApplied, commandStatusRejected, commandStatusFailed, commandStatusTimeout:
		return true
	default:
		return false
	}
}

func isFailureLifecycleStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case commandStatusRejected, commandStatusFailed, commandStatusTimeout:
		return true
	default:
		return false
	}
}

func (s *Server) handleHDPCommandResultEvent(_ paho.Client, msg mqttinfra.Message) {
	if len(msg.Payload()) == 0 {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		slog.Debug("hdp command result decode failed", "topic", msg.Topic(), "error", err)
		return
	}
	if strings.EqualFold(asString(payload["origin"]), "device-hub") {
		return
	}
	deviceID := strings.TrimSpace(asString(payload["device_id"]))
	if deviceID == "" {
		deviceID = strings.TrimPrefix(msg.Topic(), hdpCommandResultPrefix)
	}
	corr := strings.TrimSpace(firstNonEmpty(asString(payload["corr"]), asString(payload["correlation_id"])))
	if deviceID == "" || corr == "" {
		return
	}
	status := normalizeLifecycleStatus(asString(payload["status"]), commandBoolish(payload["success"]))
	errMsg := strings.TrimSpace(asString(payload["error"]))
	s.processAdapterCommandResult(deviceID, corr, status, errMsg)
}

func (s *Server) processAdapterCommandResult(deviceID, corr, status, errMsg string) {
	deviceID = strings.TrimSpace(deviceID)
	corr = strings.TrimSpace(corr)
	status = normalizeLifecycleStatus(status, errMsg == "")
	if deviceID == "" || corr == "" || status == "" {
		return
	}
	entry := s.getPendingCommand(deviceID, corr)
	if entry == nil && !isTerminalLifecycleStatus(status) {
		entry = s.ensurePassivePendingCommand(deviceID, corr)
	}

	switch status {
	case commandStatusAccepted, commandStatusQueued, commandStatusInProgress:
		if entry != nil && entry.LastStatus == status {
			return
		}
		if entry != nil {
			entry.LastStatus = status
		}
		s.publishCommandLifecycle(deviceID, corr, status, errMsg, map[string]any{"source_status": status})
	case commandStatusApplied:
		if entry == nil {
			s.publishCommandLifecycle(deviceID, corr, commandStatusApplied, errMsg, map[string]any{"source_status": status})
			return
		}
		if len(entry.Expected) == 0 && len(entry.Baseline) == 0 {
			s.removePendingCommand(deviceID, corr)
			s.publishCommandLifecycle(deviceID, corr, commandStatusApplied, errMsg, map[string]any{"source_status": status})
			return
		}
		if entry.LastStatus != commandStatusInProgress {
			entry.LastStatus = commandStatusInProgress
			s.publishCommandLifecycle(deviceID, corr, commandStatusInProgress, "", map[string]any{"source_status": status})
		}
	case commandStatusRejected, commandStatusFailed, commandStatusTimeout:
		s.removePendingCommand(deviceID, corr)
		s.publishCommandLifecycle(deviceID, corr, status, errMsg, map[string]any{"source_status": status})
	}
}

func (s *Server) processCommandStateLifecycle(deviceID string, state map[string]any, corr string, stateTs int64) {
	if s == nil || strings.TrimSpace(deviceID) == "" || len(state) == 0 {
		return
	}
	entry := s.getPendingCommand(deviceID, corr)
	if entry == nil {
		return
	}
	if !pendingCommandSatisfied(entry, state, corr, stateTs) {
		return
	}
	s.removePendingCommand(entry.DeviceID, entry.Corr)
	s.publishCommandLifecycle(entry.DeviceID, entry.Corr, commandStatusApplied, "", nil)
}

func pendingCommandSatisfied(entry *pendingCommand, state map[string]any, corr string, stateTs int64) bool {
	if entry == nil || len(state) == 0 {
		return false
	}
	corrMatches := strings.TrimSpace(corr) != "" && strings.EqualFold(strings.TrimSpace(entry.Corr), strings.TrimSpace(corr))
	stateAfterCommand := stateTs > 0 && entry.StartedAt > 0 && stateTs >= entry.StartedAt
	stateChangedFromBaseline := len(entry.Baseline) > 0 && stateMapsDiffer(state, entry.Baseline)
	if len(entry.Expected) > 0 {
		if expectedStateSatisfied(state, entry.Expected) {
			return true
		}
		if stateChangedFromBaseline && (corrMatches || stateAfterCommand) {
			return true
		}
		return false
	}
	if len(entry.Baseline) > 0 {
		return stateChangedFromBaseline && (corrMatches || stateAfterCommand)
	}
	return corrMatches
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	buf, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil
	}
	return out
}

func expectedStateSatisfied(state, expected map[string]any) bool {
	if len(expected) == 0 {
		return true
	}
	if len(state) == 0 {
		return false
	}
	for key, want := range expected {
		got, ok := state[key]
		if !ok {
			return false
		}
		if !stateValueMatches(got, want) {
			return false
		}
	}
	return true
}

func stateMapsDiffer(state, baseline map[string]any) bool {
	if state == nil {
		state = map[string]any{}
	}
	if baseline == nil {
		baseline = map[string]any{}
	}
	keys := make(map[string]struct{}, len(state)+len(baseline))
	for key := range state {
		keys[key] = struct{}{}
	}
	for key := range baseline {
		keys[key] = struct{}{}
	}
	for key := range keys {
		got, gok := state[key]
		want, wok := baseline[key]
		if gok != wok {
			return true
		}
		if !gok {
			continue
		}
		if !stateValueMatches(got, want) {
			return true
		}
	}
	return false
}

func stateValueMatches(got, want any) bool {
	if gb, ok := normalizeBoolLike(got); ok {
		if wb, ok := normalizeBoolLike(want); ok {
			return gb == wb
		}
	}
	if gn, ok := asNumeric(got); ok {
		if wn, ok := asNumeric(want); ok {
			return gn == wn
		}
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", got)), strings.TrimSpace(fmt.Sprintf("%v", want)))
}

func normalizeBoolLike(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "on", "enabled":
			return true, true
		case "false", "0", "no", "off", "disabled", "standby":
			return false, true
		}
	case float64:
		return v != 0, true
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	}
	return false, false
}

func asNumeric(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return n, true
	case string:
		var n json.Number = json.Number(strings.TrimSpace(v))
		parsed, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func commandBoolish(value any) bool {
	v, ok := normalizeBoolLike(value)
	return ok && v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
