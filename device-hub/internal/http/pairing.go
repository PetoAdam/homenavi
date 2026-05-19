package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	model "github.com/PetoAdam/homenavi/device-hub/internal/devices"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"

	mqttinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/mqtt"
)

func (s *Server) handleIntegrations(w http.ResponseWriter, _ *http.Request) {
	if s == nil || s.adapters == nil {
		s.writeEmptyArray(w)
		return
	}
	items := s.adapters.integrationsSnapshot()
	if len(items) == 0 {
		s.writeEmptyArray(w)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handlePairingConfig(w http.ResponseWriter, _ *http.Request) {
	if s == nil || s.adapters == nil {
		s.writeEmptyArray(w)
		return
	}
	items := s.adapters.pairingConfigsSnapshot()
	if len(items) == 0 {
		s.writeEmptyArray(w)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handlePairings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessions := s.snapshotPairings()
		writeJSON(w, http.StatusOK, sessions)
		return
	case http.MethodPost:
		defer r.Body.Close()
		var req pairingStartRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		protocol := normalizeProtocol(req.Protocol)
		if protocol == "" {
			http.Error(w, "protocol is required", http.StatusBadRequest)
			return
		}
		session, err := s.startPairing(protocol, req.Timeout, req.Mode, req.FlowID, req.Inputs, req.Metadata)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errPairingActive) {
				status = http.StatusConflict
			} else if errors.Is(err, errPairingUnsupported) {
				status = http.StatusNotImplemented
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusAccepted, session)
		return
	case http.MethodDelete:
		protocol := normalizeProtocol(r.URL.Query().Get("protocol"))
		if protocol == "" {
			http.Error(w, "protocol query parameter required", http.StatusBadRequest)
			return
		}
		session, err := s.stopPairing(protocol, "stopped")
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errPairingNotFound) {
				status = http.StatusNotFound
			} else if errors.Is(err, errPairingUnsupported) {
				status = http.StatusNotImplemented
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, http.StatusOK, session)
		return
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) snapshotPairings() []pairingSession {
	s.pairingMu.Lock()
	defer s.pairingMu.Unlock()
	result := make([]pairingSession, 0, len(s.pairings))
	var expired []pairingSession
	now := time.Now().UTC()
	for _, session := range s.pairings {
		if session.Active && !session.ExpiresAt.IsZero() && !session.ExpiresAt.After(now) {
			session.Active = false
			session.Status = "timeout"
			session.ExpiresAt = now
			if session.cancel != nil {
				session.cancel()
				session.cancel = nil
			}
			expired = append(expired, session.clone())
		}
		clone := session.clone()
		result = append(result, clone)
	}
	if len(expired) > 0 {
		go func(items []pairingSession) {
			for _, item := range items {
				_ = s.publishPairingCommand(item.Protocol, "stop", 0, "", "", nil)
				s.emitPairingEvent(item)
			}
		}(expired)
	}
	return result
}

func (s *Server) startPairing(protocol string, timeout int, mode, flowID string, inputs map[string]any, metadata pairingMetadata) (*pairingSession, error) {
	if err := s.ensurePairingSupported(protocol); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 60
	} else if timeout > 300 {
		timeout = 300
	}
	now := time.Now().UTC()
	meta := sanitizePairingMetadata(metadata)
	normalizedInputs := sanitizePairingInputs(inputs)
	session := &pairingSession{
		ID:                   uuid.NewString(),
		Protocol:             protocol,
		Mode:                 strings.TrimSpace(strings.ToLower(mode)),
		FlowID:               strings.TrimSpace(flowID),
		Inputs:               normalizedInputs,
		Stage:                "starting",
		Status:               "starting",
		Active:               true,
		StartedAt:            now,
		ExpiresAt:            now.Add(time.Duration(timeout) * time.Second),
		AllowMultipleDevices: pairingAllowsMultipleDevices(normalizedInputs),
		Metadata:             meta,
		knownDevices:         s.snapshotKnownDevices(),
	}
	s.pairingMu.Lock()
	if existing, ok := s.pairings[protocol]; ok && existing.Active {
		if !existing.ExpiresAt.IsZero() && !existing.ExpiresAt.After(now) {
			existing.Active = false
			existing.Status = "timeout"
			existing.ExpiresAt = now
			if existing.cancel != nil {
				existing.cancel()
				existing.cancel = nil
			}
			expired := existing.clone()
			s.pairingMu.Unlock()
			_ = s.publishPairingCommand(protocol, "stop", 0, "", "", nil)
			s.emitPairingEvent(expired)
		} else {
			clone := existing.clone()
			s.pairingMu.Unlock()
			return &clone, nil
		}
	} else {
		s.pairingMu.Unlock()
	}
	s.pairingMu.Lock()
	s.pairings[protocol] = session
	s.pairingMu.Unlock()
	if err := s.publishPairingCommand(protocol, "start", timeout, mode, flowID, inputs); err != nil {
		s.pairingMu.Lock()
		delete(s.pairings, protocol)
		s.pairingMu.Unlock()
		return nil, fmt.Errorf("failed to start pairing: %w", err)
	}
	session.Status = "active"
	session.Stage = "active"
	s.emitPairingEvent(session.clone())
	s.startPairingTimeout(protocol, session.ID, session.ExpiresAt)
	clone := session.clone()
	return &clone, nil
}

func (s *Server) stopPairing(protocol, status string) (*pairingSession, error) {
	if err := s.ensurePairingSupported(protocol); err != nil {
		return nil, err
	}
	s.pairingMu.Lock()
	session, ok := s.pairings[protocol]
	if !ok {
		s.pairingMu.Unlock()
		return nil, errPairingNotFound
	}
	if session.cancel != nil {
		session.cancel()
		session.cancel = nil
	}
	session.Active = false
	session.Status = status
	session.ExpiresAt = time.Now().UTC()
	clone := session.clone()
	s.pairingMu.Unlock()
	_ = s.publishPairingCommand(protocol, "stop", 0, "", "", nil)
	s.emitPairingEvent(clone)
	return &clone, nil
}

func (s *Server) startPairingTimeout(protocol, sessionID string, expires time.Time) {
	duration := time.Until(expires)
	if duration <= 0 {
		duration = time.Second * 60
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.pairingMu.Lock()
	if session, ok := s.pairings[protocol]; ok && session.ID == sessionID {
		session.cancel = cancel
	} else {
		s.pairingMu.Unlock()
		cancel()
		return
	}
	s.pairingMu.Unlock()
	go func() {
		select {
		case <-time.After(duration):
			s.handlePairingTimeout(protocol, sessionID)
		case <-ctx.Done():
		}
	}()
}

func (s *Server) handlePairingTimeout(protocol, sessionID string) {
	s.pairingMu.Lock()
	session, ok := s.pairings[protocol]
	if !ok || session.ID != sessionID || !session.Active {
		s.pairingMu.Unlock()
		return
	}
	session.Active = false
	session.Status = "timeout"
	session.ExpiresAt = time.Now().UTC()
	if session.cancel != nil {
		session.cancel()
		session.cancel = nil
	}
	clone := session.clone()
	s.pairingMu.Unlock()
	_ = s.publishPairingCommand(protocol, "stop", 0, "", "", nil)
	s.emitPairingEvent(clone)
}

func (s *Server) publishPairingCommand(protocol, action string, timeout int, mode, flowID string, inputs map[string]any) error {
	hdpPayload := map[string]any{
		"schema":   hdpSchema,
		"type":     "pairing_command",
		"protocol": protocol,
		"action":   action,
	}
	if timeout > 0 {
		hdpPayload["timeout"] = timeout
		hdpPayload["timeout_sec"] = timeout
	}
	if m := strings.TrimSpace(mode); m != "" {
		hdpPayload["mode"] = m
	}
	if f := strings.TrimSpace(flowID); f != "" {
		hdpPayload["flow_id"] = f
	}
	if len(inputs) > 0 {
		hdpPayload["inputs"] = sanitizePairingInputs(inputs)
	}
	b, err := json.Marshal(hdpPayload)
	if err != nil {
		return err
	}
	if err := s.mqtt.Publish(hdpPairingCommandPrefix+protocol, b); err != nil {
		return err
	}
	return nil
}

func sanitizePairingInputs(inputs map[string]any) map[string]any {
	if len(inputs) == 0 {
		return nil
	}
	normalized := make(map[string]any, len(inputs))
	for rawKey, value := range inputs {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func (s *Server) emitPairingEvent(session pairingSession) {
	data, err := json.Marshal(session)
	if err != nil {
		slog.Warn("encode pairing event failed", "error", err)
		return
	}
	if err := s.mqtt.Publish(hdpPairingProgressPrefix+session.Protocol, data); err != nil {
		slog.Warn("pairing event publish failed", "error", err)
	}
	s.publishHDPPairingProgress(session)
}

func (s *Server) publishHDPPairingProgress(session pairingSession) {
	if session.Protocol == "" {
		return
	}
	envelope := map[string]any{
		"id":         session.ID,
		"schema":     hdpSchema,
		"type":       "pairing_progress",
		"protocol":   session.Protocol,
		"origin":     "device-hub",
		"stage":      session.Stage,
		"status":     session.Status,
		"active":     session.Active,
		"started_at": session.StartedAt,
		"expires_at": session.ExpiresAt,
		"ts":         time.Now().UnixMilli(),
	}
	if session.Stage == "" {
		envelope["stage"] = session.Status
	}
	if session.DeviceID != "" {
		envelope["device_id"] = session.DeviceID
	}
	if session.candidateExternalID != "" {
		envelope["external_id"] = session.candidateExternalID
	}
	if (session.Metadata != pairingMetadata{}) {
		envelope["metadata"] = session.Metadata
	}
	if session.Mode != "" {
		envelope["mode"] = session.Mode
	}
	if session.FlowID != "" {
		envelope["flow_id"] = session.FlowID
	}
	if session.AllowMultipleDevices {
		envelope["allow_multiple_devices"] = true
	}
	if len(session.Inputs) > 0 {
		envelope["inputs"] = session.Inputs
	}
	if len(session.AddedDevices) > 0 {
		envelope["added_devices"] = session.AddedDevices
	}
	if session.Message != "" {
		envelope["message"] = session.Message
	}
	if session.ErrorCode != "" {
		envelope["error_code"] = session.ErrorCode
	}
	if len(session.RequiredInputs) > 0 {
		envelope["required_inputs"] = session.RequiredInputs
	}
	if b, err := json.Marshal(envelope); err == nil {
		if err := s.mqtt.Publish(hdpPairingProgressPrefix+session.Protocol, b); err != nil {
			slog.Warn("hdp pairing progress publish failed", "protocol", session.Protocol, "error", err)
		}
	}
}

func (s *Server) handleHDPPairingProgressEvent(_ paho.Client, msg mqttinfra.Message) {
	protocol := strings.TrimPrefix(msg.Topic(), hdpPairingProgressPrefix)
	if protocol == msg.Topic() {
		protocol = ""
	}
	var evt map[string]any
	if err := json.Unmarshal(msg.Payload(), &evt); err != nil {
		slog.Debug("hdp pairing progress decode failed", "error", err)
		return
	}
	if strings.EqualFold(asString(evt["origin"]), "device-hub") {
		return
	}
	if protoVal := asString(evt["protocol"]); protoVal != "" {
		protocol = protoVal
	}
	stage := asString(evt["stage"])
	status := asString(evt["status"])
	message := strings.TrimSpace(asString(evt["message"]))
	errorCode := strings.TrimSpace(asString(evt["error_code"]))
	mode := strings.TrimSpace(strings.ToLower(asString(evt["mode"])))
	flowID := strings.TrimSpace(asString(evt["flow_id"]))
	inputs, _ := evt["inputs"].(map[string]any)
	requiredInputs := stringSlice(evt["required_inputs"])
	external := asString(evt["external_id"])
	if external == "" {
		external = asString(evt["device_id"])
	}
	s.processPairingProgress(protocol, stage, status, external, pairingProgressUpdate{
		Message:        message,
		ErrorCode:      errorCode,
		Mode:           mode,
		FlowID:         flowID,
		Inputs:         sanitizePairingInputs(inputs),
		RequiredInputs: requiredInputs,
	})
}

type pairingProgressUpdate struct {
	Message        string
	ErrorCode      string
	Mode           string
	FlowID         string
	Inputs         map[string]any
	RequiredInputs []string
}

func isTerminalPairingState(value string) bool {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "timeout", "stopped", "failed", "error", "completed":
		return true
	default:
		return false
	}
}

func pairingAllowsMultipleDevices(inputs map[string]any) bool {
	if len(inputs) == 0 {
		return false
	}
	value, ok := inputs["allow_multiple_devices"]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		normalized := strings.TrimSpace(strings.ToLower(typed))
		return normalized == "true" || normalized == "1" || normalized == "yes" || normalized == "on"
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return false
	}
}

func shouldTerminatePairingSession(session *pairingSession, stage, status string) bool {
	if session == nil {
		return isTerminalPairingState(stage) || isTerminalPairingState(status)
	}
	if !session.AllowMultipleDevices {
		return isTerminalPairingState(stage) || isTerminalPairingState(status) || isTerminalPairingState(session.Status)
	}
	for _, value := range []string{stage, status, session.Status} {
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "timeout", "stopped", "failed", "error":
			return true
		}
	}
	return false
}

func buildPairingAddedDevice(dev *model.Device, state string) pairingAddedDevice {
	now := time.Now().UTC()
	if dev == nil {
		return pairingAddedDevice{}
	}
	return pairingAddedDevice{
		DeviceID:     canonicalHDPDeviceID(dev.Protocol, dev.ExternalID),
		Protocol:     dev.Protocol,
		ExternalID:   dev.ExternalID,
		Name:         strings.TrimSpace(dev.Name),
		State:        strings.TrimSpace(state),
		Type:         dev.Type,
		Manufacturer: dev.Manufacturer,
		Model:        dev.Model,
		Description:  dev.Description,
		Icon:         dev.Icon,
		AddedAt:      now,
		UpdatedAt:    now,
	}
}

func multiDeviceItemState(stage, status string) string {
	for _, value := range []string{stage, status} {
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "failed", "error":
			return "failed"
		case "completed":
			return "completed"
		case "interviewing", "interview_complete":
			return "finalizing"
		case "device_joined", "device_detected", "device_added":
			return "detected"
		}
	}
	return ""
}

func multiDeviceItemStateRank(value string) int {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "detected":
		return 10
	case "finalizing":
		return 20
	case "completed":
		return 30
	case "failed":
		return 40
	default:
		return 0
	}
}

func buildPairingProgressDevice(protocol, externalID, stage, status string) pairingAddedDevice {
	state := multiDeviceItemState(stage, status)
	if strings.TrimSpace(externalID) == "" || state == "" {
		return pairingAddedDevice{}
	}
	now := time.Now().UTC()
	return pairingAddedDevice{
		Protocol:   protocol,
		ExternalID: externalID,
		State:      state,
		AddedAt:    now,
		UpdatedAt:  now,
	}
}

func mergePairingAddedDevice(existing, incoming pairingAddedDevice) pairingAddedDevice {
	merged := existing
	if strings.TrimSpace(incoming.DeviceID) != "" {
		merged.DeviceID = incoming.DeviceID
	}
	if strings.TrimSpace(incoming.Protocol) != "" {
		merged.Protocol = incoming.Protocol
	}
	if strings.TrimSpace(incoming.ExternalID) != "" {
		merged.ExternalID = incoming.ExternalID
	}
	if strings.TrimSpace(incoming.Name) != "" {
		merged.Name = incoming.Name
	}
	if strings.TrimSpace(incoming.State) != "" && multiDeviceItemStateRank(incoming.State) >= multiDeviceItemStateRank(merged.State) {
		merged.State = incoming.State
	}
	if strings.TrimSpace(incoming.Type) != "" {
		merged.Type = incoming.Type
	}
	if strings.TrimSpace(incoming.Manufacturer) != "" {
		merged.Manufacturer = incoming.Manufacturer
	}
	if strings.TrimSpace(incoming.Model) != "" {
		merged.Model = incoming.Model
	}
	if strings.TrimSpace(incoming.Description) != "" {
		merged.Description = incoming.Description
	}
	if strings.TrimSpace(incoming.Icon) != "" {
		merged.Icon = incoming.Icon
	}
	if merged.AddedAt.IsZero() && !incoming.AddedAt.IsZero() {
		merged.AddedAt = incoming.AddedAt
	}
	if !incoming.UpdatedAt.IsZero() {
		merged.UpdatedAt = incoming.UpdatedAt
	}
	return merged
}

func upsertPairingAddedDevice(items []pairingAddedDevice, item pairingAddedDevice) []pairingAddedDevice {
	if item.DeviceID == "" && item.ExternalID == "" {
		return items
	}
	for index := range items {
		if (item.DeviceID != "" && strings.EqualFold(items[index].DeviceID, item.DeviceID)) || (item.ExternalID != "" && strings.EqualFold(items[index].ExternalID, item.ExternalID)) {
			items[index] = mergePairingAddedDevice(items[index], item)
			return items
		}
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	if item.AddedAt.IsZero() {
		item.AddedAt = item.UpdatedAt
	}
	return append(items, item)
}

func multiDevicePairingNotice(count int) string {
	if count <= 0 {
		return "Pairing remains open for additional devices."
	}
	if count == 1 {
		return "1 device added. Keep pairing open for more devices or stop when you are done."
	}
	return fmt.Sprintf("%d devices added. Keep pairing open for more devices or stop when you are done.", count)
}

func (s *Server) processPairingProgress(protocol, stage, status, externalID string, update pairingProgressUpdate) {
	proto := normalizeProtocol(protocol)
	if proto == "" {
		return
	}
	stage = strings.TrimSpace(strings.ToLower(stage))
	status = strings.TrimSpace(strings.ToLower(status))

	s.pairingMu.Lock()
	session, ok := s.pairings[proto]
	if !ok || !session.Active {
		s.pairingMu.Unlock()
		return
	}
	if session.candidateExternalID == "" && externalID != "" {
		normalized := strings.TrimSpace(externalID)
		if eventProto, eventExt, ok := splitHDPDeviceID(normalized); ok && eventProto != "" {
			normalized = eventExt
		}
		if normExt, err := normalizeExternalID(proto, normalized); err == nil {
			normalized = normExt
		}
		session.candidateExternalID = strings.ToLower(normalized)
	}
	if session.AllowMultipleDevices {
		progressDevice := buildPairingProgressDevice(proto, session.candidateExternalID, stage, status)
		session.AddedDevices = upsertPairingAddedDevice(session.AddedDevices, progressDevice)
	}
	if session.AllowMultipleDevices && session.Active && (stage == "completed" || status == "completed") && len(session.AddedDevices) > 0 {
		session.Stage = "device_added"
		session.Status = "device_added"
		session.Message = multiDevicePairingNotice(len(session.AddedDevices))
	} else {
		if stage != "" {
			session.Stage = stage
		}
		if status != "" {
			session.Status = status
		} else if stage != "" {
			session.Status = stage
		}
		if isTerminalPairingState(stage) {
			session.Status = stage
		}
	}

	if update.Message != "" {
		session.Message = update.Message
	}
	if update.ErrorCode != "" {
		session.ErrorCode = update.ErrorCode
	}
	if update.Mode != "" {
		session.Mode = update.Mode
	}
	if update.FlowID != "" {
		session.FlowID = update.FlowID
	}
	if len(update.Inputs) > 0 {
		session.Inputs = update.Inputs
	}
	if update.RequiredInputs != nil {
		session.RequiredInputs = append([]string(nil), update.RequiredInputs...)
	}

	if shouldTerminatePairingSession(session, stage, status) {
		session.Active = false
		session.ExpiresAt = time.Now().UTC()
		if session.cancel != nil {
			session.cancel()
			session.cancel = nil
		}
	}
	snapshot := session.clone()
	s.pairingMu.Unlock()
	s.emitPairingEvent(snapshot)
}

func (s *Server) shouldAcceptPairingCandidate(session *pairingSession, dev *model.Device) bool {
	if session == nil || dev == nil {
		return false
	}
	if session.DeviceID != "" && !session.AllowMultipleDevices {
		return false
	}
	if len(session.knownDevices) > 0 {
		if _, exists := session.knownDevices[dev.ID.String()]; exists {
			return false
		}
	}
	if !session.AllowMultipleDevices && session.candidateExternalID != "" && dev.ExternalID != "" {
		if !strings.EqualFold(session.candidateExternalID, dev.ExternalID) {
			return false
		}
	}
	if !session.StartedAt.IsZero() && !dev.CreatedAt.IsZero() {
		if dev.CreatedAt.Before(session.StartedAt.Add(-5 * time.Second)) {
			return false
		}
	}
	return true
}

func (s *Server) snapshotKnownDevices() map[string]struct{} {
	if s == nil || s.repo == nil {
		return nil
	}
	ctx := context.Background()
	devices, err := s.repo.List(ctx)
	if err != nil {
		slog.Debug("pairing snapshot failed", "error", err)
		return nil
	}
	if len(devices) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(devices))
	for _, dev := range devices {
		if dev.ID == uuid.Nil {
			continue
		}
		known[dev.ID.String()] = struct{}{}
	}
	return known
}

func sanitizePairingMetadata(meta pairingMetadata) pairingMetadata {
	trim := func(v string) string {
		return strings.TrimSpace(v)
	}
	return pairingMetadata{
		Icon:         strings.ToLower(trim(meta.Icon)),
		Description:  trim(meta.Description),
		Type:         trim(meta.Type),
		Manufacturer: trim(meta.Manufacturer),
		Model:        trim(meta.Model),
	}
}

func (s *Server) ensurePairingSupported(protocol string) error {
	proto := normalizeProtocol(protocol)
	if proto == "" {
		return unsupportedProtocolError{protocol: protocol}
	}
	if s == nil || s.adapters == nil {
		return unsupportedProtocolError{protocol: proto}
	}
	if s.adapters.isPairingSupported(proto) {
		return nil
	}
	return unsupportedProtocolError{protocol: proto}
}

func (s *Server) supportsInterviewTracking(protocol string) bool {
	if s == nil || s.adapters == nil {
		return false
	}
	return s.adapters.supportsInterview(protocol)
}

func (s *Server) handlePairingCandidate(dev *model.Device) {
	if dev == nil || dev.ID == uuid.Nil {
		return
	}
	protocol := normalizeProtocol(dev.Protocol)
	if protocol == "" {
		return
	}
	deviceID := dev.ID.String()
	s.pairingMu.Lock()
	session, ok := s.pairings[protocol]
	if !ok || !session.Active {
		s.pairingMu.Unlock()
		return
	}
	if !s.shouldAcceptPairingCandidate(session, dev) {
		s.pairingMu.Unlock()
		return
	}
	meta := session.Metadata
	supportsInterview := s.supportsInterviewTracking(protocol)
	if session.DeviceID == "" {
		session.DeviceID = deviceID
	}
	addedState := "completed"
	if supportsInterview {
		addedState = "detected"
	}
	added := buildPairingAddedDevice(dev, addedState)
	session.AddedDevices = upsertPairingAddedDevice(session.AddedDevices, added)
	if session.knownDevices == nil {
		session.knownDevices = make(map[string]struct{})
	}
	session.knownDevices[dev.ID.String()] = struct{}{}
	if session.AllowMultipleDevices {
		session.Stage = "device_added"
		session.Status = "device_added"
		session.Message = multiDevicePairingNotice(len(session.AddedDevices))
	} else {
		session.Status = "device_detected"
	}
	if !supportsInterview && !session.AllowMultipleDevices {
		session.Active = false
	}
	snapshot := session.clone()
	s.pairingMu.Unlock()
	s.emitPairingEvent(snapshot)
	go s.applyPairingMetadata(deviceID, meta)
	go func(proto string, deferCompletion bool, keepOpen bool) {
		if keepOpen {
			return
		}
		if err := s.publishPairingCommand(proto, "stop", 0, "", "", nil); err != nil {
			slog.Warn("pairing permit stop failed", "protocol", proto, "error", err)
		}
		if !deferCompletion {
			if _, err := s.stopPairing(proto, "completed"); err != nil && !errors.Is(err, errPairingNotFound) {
				slog.Warn("pairing stop failed", "protocol", proto, "error", err)
			}
		}
	}(protocol, supportsInterview, session.AllowMultipleDevices)
}

func (s *Server) applyPairingMetadata(deviceID string, meta pairingMetadata) {
	trimmed := sanitizePairingMetadata(meta)
	if deviceID == "" || trimmed == (pairingMetadata{}) {
		return
	}
	ctx := context.Background()
	dev, err := s.repo.GetByID(ctx, deviceID)
	if err != nil || dev == nil {
		return
	}
	changed := false
	if trimmed.Description != "" && trimmed.Description != dev.Description {
		dev.Description = trimmed.Description
		changed = true
	}
	if trimmed.Type != "" && trimmed.Type != dev.Type {
		dev.Type = trimmed.Type
		changed = true
	}
	if trimmed.Manufacturer != "" && trimmed.Manufacturer != dev.Manufacturer {
		dev.Manufacturer = trimmed.Manufacturer
		changed = true
	}
	if trimmed.Model != "" && trimmed.Model != dev.Model {
		dev.Model = trimmed.Model
		changed = true
	}
	if trimmed.Icon != "" && trimmed.Icon != dev.Icon {
		dev.Icon = trimmed.Icon
		changed = true
	}
	if !changed {
		return
	}
	if err := s.repo.UpsertDevice(ctx, dev); err != nil {
		slog.Warn("pairing metadata update failed", "device_id", deviceID, "error", err)
		return
	}
	s.publishDeviceMetadata(dev)
}
