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
	dbinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/db"
	"github.com/google/uuid"

	mqttinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/mqtt"
)

type storedPairingSession struct {
	Session             pairingSession      `json:"session"`
	KnownDevices        map[string]struct{} `json:"known_devices,omitempty"`
	KnownExternalIDs    map[string]struct{} `json:"known_external_ids,omitempty"`
	CandidateExternalID string              `json:"candidate_external_id,omitempty"`
}

func marshalPairingSession(session *pairingSession) ([]byte, error) {
	if session == nil {
		return nil, fmt.Errorf("pairing session is required")
	}
	return json.Marshal(storedPairingSession{Session: session.clone(), KnownDevices: session.knownDevices, KnownExternalIDs: session.knownExternalIDs, CandidateExternalID: session.candidateExternalID})
}

func unmarshalPairingSession(payload []byte) (*pairingSession, error) {
	var stored storedPairingSession
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, err
	}
	session := stored.Session
	session.knownDevices = stored.KnownDevices
	session.knownExternalIDs = stored.KnownExternalIDs
	session.candidateExternalID = stored.CandidateExternalID
	return &session, nil
}

func (s *Server) mutatePersistentPairing(protocol string, mutate func(*pairingSession) (bool, error)) (pairingSession, bool, error) {
	lifecycle, changed, err := s.repo.MutateActivePairing(context.Background(), protocol, func(record *dbinfra.PairingLifecycle) (bool, error) {
		session, err := unmarshalPairingSession(record.Session)
		if err != nil {
			return false, err
		}
		updated, err := mutate(session)
		if err != nil || !updated {
			return updated, err
		}
		payload, err := marshalPairingSession(session)
		if err != nil {
			return false, err
		}
		record.Status = session.Status
		record.Active = session.Active
		record.Session = payload
		record.ExpiresAt = session.ExpiresAt
		return true, nil
	})
	if err != nil || !changed {
		return pairingSession{}, false, err
	}
	session, err := unmarshalPairingSession(lifecycle.Session)
	if err != nil {
		return pairingSession{}, false, err
	}
	return session.clone(), true, nil
}

func (s *Server) persistPairingLifecycle(session *pairingSession) {
	if s == nil || s.repo == nil || session == nil {
		return
	}
	payload, err := marshalPairingSession(session)
	if err != nil {
		return
	}
	if _, err := s.repo.CreatePairingLifecycle(context.Background(), dbinfra.PairingLifecycle{ID: session.ID, Protocol: session.Protocol, Status: session.Status, Session: payload, StartedAt: session.StartedAt, ExpiresAt: session.ExpiresAt}); err != nil {
		slog.Warn("pairing lifecycle persistence failed", "protocol", session.Protocol, "session_id", session.ID, "error", err)
	}
}

func (s *Server) completePairingLifecycle(session pairingSession) {
	if s == nil || s.repo == nil || session.ID == "" || session.Active {
		return
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return
	}
	if _, _, err := s.repo.CompletePairingLifecycle(context.Background(), session.ID, session.Status, payload); err != nil {
		slog.Warn("pairing lifecycle completion persistence failed", "protocol", session.Protocol, "session_id", session.ID, "error", err)
	}
}

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
	if s.repo != nil {
		lifecycles, err := s.repo.ListActivePairings(context.Background())
		if err != nil {
			slog.Warn("active pairing list failed", "error", err)
			return nil
		}
		items := make([]pairingSession, 0, len(lifecycles))
		for _, lifecycle := range lifecycles {
			session, err := unmarshalPairingSession(lifecycle.Session)
			if err != nil {
				slog.Warn("active pairing decode failed", "session_id", lifecycle.ID, "error", err)
				continue
			}
			if session.Active && !session.ExpiresAt.After(time.Now().UTC()) {
				s.handlePairingTimeout(session.Protocol, session.ID)
				continue
			}
			items = append(items, session.clone())
		}
		return items
	}
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
		Stage:                "active",
		Status:               "active",
		Active:               true,
		StartedAt:            now,
		ExpiresAt:            now.Add(time.Duration(timeout) * time.Second),
		AllowMultipleDevices: pairingAllowsMultipleDevices(normalizedInputs),
		Metadata:             meta,
		knownDevices:         s.snapshotKnownDevices(),
		knownExternalIDs:     s.snapshotKnownExternalIDs(protocol),
	}
	if s.repo != nil {
		payload, err := marshalPairingSession(session)
		if err != nil {
			return nil, fmt.Errorf("encode pairing session: %w", err)
		}
		created, err := s.repo.CreatePairingLifecycle(context.Background(), dbinfra.PairingLifecycle{ID: session.ID, Protocol: session.Protocol, Status: session.Status, Active: true, Session: payload, StartedAt: session.StartedAt, ExpiresAt: session.ExpiresAt})
		if err != nil {
			return nil, fmt.Errorf("create pairing session: %w", err)
		}
		if !created {
			lifecycle, found, err := s.repo.GetActivePairing(context.Background(), protocol)
			if err != nil {
				return nil, fmt.Errorf("load active pairing session: %w", err)
			}
			if !found {
				return nil, errPairingActive
			}
			existing, err := unmarshalPairingSession(lifecycle.Session)
			if err != nil {
				return nil, fmt.Errorf("decode active pairing session: %w", err)
			}
			clone := existing.clone()
			return &clone, nil
		}
		if err := s.publishPairingCommand(protocol, "start", timeout, mode, flowID, inputs); err != nil {
			_, _, _ = s.mutatePersistentPairing(protocol, func(current *pairingSession) (bool, error) {
				if current.ID != session.ID || !current.Active {
					return false, nil
				}
				current.Active = false
				current.Status = "failed"
				current.ExpiresAt = time.Now().UTC()
				return true, nil
			})
			return nil, fmt.Errorf("failed to start pairing: %w", err)
		}
		s.emitPairingEvent(session.clone())
		clone := session.clone()
		return &clone, nil
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
	s.persistPairingLifecycle(session)
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
	if s.repo != nil {
		snapshot, changed, err := s.mutatePersistentPairing(protocol, func(session *pairingSession) (bool, error) {
			if !session.Active {
				return false, nil
			}
			session.Active = false
			session.Status = status
			session.ExpiresAt = time.Now().UTC()
			return true, nil
		})
		if err != nil {
			return nil, err
		}
		if !changed {
			return nil, errPairingNotFound
		}
		_ = s.publishPairingCommand(protocol, "stop", 0, "", "", nil)
		s.emitPairingEvent(snapshot)
		return &snapshot, nil
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
	s.completePairingLifecycle(clone)
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

func (s *Server) startPairingExpiryReaper() {
	if s == nil || s.repo == nil {
		return
	}
	s.pairingExpiryOnce.Do(func() {
		s.reapExpiredPairings()
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for range ticker.C {
				s.reapExpiredPairings()
			}
		}()
	})
}

func (s *Server) reapExpiredPairings() {
	if s == nil || s.repo == nil {
		return
	}
	lifecycles, err := s.repo.ListActivePairings(context.Background())
	if err != nil {
		slog.Warn("pairing expiry list failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, lifecycle := range lifecycles {
		if !lifecycle.Active || lifecycle.ExpiresAt.After(now) {
			continue
		}
		s.handlePairingTimeout(lifecycle.Protocol, lifecycle.ID)
	}
}

func (s *Server) handlePairingTimeout(protocol, sessionID string) {
	if s.repo != nil {
		snapshot, changed, err := s.mutatePersistentPairing(protocol, func(session *pairingSession) (bool, error) {
			if session.ID != sessionID || !session.Active || session.ExpiresAt.After(time.Now().UTC()) {
				return false, nil
			}
			session.Active = false
			session.Status = "timeout"
			session.ExpiresAt = time.Now().UTC()
			return true, nil
		})
		if err != nil {
			slog.Warn("pairing timeout transition failed", "protocol", protocol, "session_id", sessionID, "error", err)
			return
		}
		if changed {
			_ = s.publishPairingCommand(protocol, "stop", 0, "", "", nil)
			s.emitPairingEvent(snapshot)
		}
		return
	}
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
	s.completePairingLifecycle(clone)
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

func (s *Server) handleHDPPairingProgressEvent(msg mqttinfra.Message) {
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

func normalizePairingProgressExternalID(protocol, externalID string) string {
	normalized := strings.TrimSpace(externalID)
	if normalized == "" {
		return ""
	}
	if eventProto, eventExt, ok := splitHDPDeviceID(normalized); ok && eventProto != "" {
		normalized = eventExt
	}
	if normExt, err := normalizeExternalID(protocol, normalized); err == nil {
		normalized = normExt
	}
	return strings.ToLower(strings.TrimSpace(normalized))
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
	if s.repo != nil {
		s.processPersistentPairingProgress(proto, stage, status, externalID, update)
		return
	}

	s.pairingMu.Lock()
	session, ok := s.pairings[proto]
	if !ok || !session.Active {
		s.pairingMu.Unlock()
		return
	}
	normalizedExternalID := normalizePairingProgressExternalID(proto, externalID)
	if !session.AllowMultipleDevices && session.candidateExternalID == "" && normalizedExternalID != "" {
		session.candidateExternalID = normalizedExternalID
	}
	if session.AllowMultipleDevices {
		if _, known := session.knownExternalIDs[normalizedExternalID]; normalizedExternalID != "" && !known {
			progressDevice := buildPairingProgressDevice(proto, normalizedExternalID, stage, status)
			session.AddedDevices = upsertPairingAddedDevice(session.AddedDevices, progressDevice)
		}
		if !shouldTerminatePairingSession(session, stage, status) {
			session.Stage = "active"
			session.Status = "active"
			if len(session.AddedDevices) > 0 {
				session.Message = multiDevicePairingNotice(len(session.AddedDevices))
			}
		}
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
	s.completePairingLifecycle(snapshot)
}

func (s *Server) processPersistentPairingProgress(protocol, stage, status, externalID string, update pairingProgressUpdate) {
	snapshot, changed, err := s.mutatePersistentPairing(protocol, func(session *pairingSession) (bool, error) {
		if !session.Active {
			return false, nil
		}
		normalizedExternalID := normalizePairingProgressExternalID(protocol, externalID)
		if !session.AllowMultipleDevices && session.candidateExternalID == "" && normalizedExternalID != "" {
			session.candidateExternalID = normalizedExternalID
		}
		if session.AllowMultipleDevices {
			if _, known := session.knownExternalIDs[normalizedExternalID]; normalizedExternalID != "" && !known {
				session.AddedDevices = upsertPairingAddedDevice(session.AddedDevices, buildPairingProgressDevice(protocol, normalizedExternalID, stage, status))
			}
			if !shouldTerminatePairingSession(session, stage, status) {
				session.Stage = "active"
				session.Status = "active"
				if len(session.AddedDevices) > 0 {
					session.Message = multiDevicePairingNotice(len(session.AddedDevices))
				}
			}
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
		}
		return true, nil
	})
	if err != nil {
		slog.Warn("pairing progress transition failed", "protocol", protocol, "error", err)
		return
	}
	if changed {
		s.emitPairingEvent(snapshot)
	}
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

func (s *Server) snapshotKnownExternalIDs(protocol string) map[string]struct{} {
	if s == nil || s.repo == nil {
		return nil
	}
	ctx := context.Background()
	devices, err := s.repo.List(ctx)
	if err != nil {
		slog.Debug("pairing external snapshot failed", "error", err)
		return nil
	}
	if len(devices) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(devices))
	for _, dev := range devices {
		if !strings.EqualFold(normalizeProtocol(dev.Protocol), protocol) {
			continue
		}
		normalized := normalizePairingProgressExternalID(protocol, dev.ExternalID)
		if normalized == "" {
			continue
		}
		known[normalized] = struct{}{}
	}
	if len(known) == 0 {
		return nil
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
	if s.repo != nil {
		s.handlePersistentPairingCandidate(protocol, dev)
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
		session.Stage = "active"
		session.Status = "active"
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

func (s *Server) handlePersistentPairingCandidate(protocol string, dev *model.Device) {
	var metadata pairingMetadata
	var supportsInterview bool
	snapshot, changed, err := s.mutatePersistentPairing(protocol, func(session *pairingSession) (bool, error) {
		if !session.Active || !s.shouldAcceptPairingCandidate(session, dev) {
			return false, nil
		}
		metadata = session.Metadata
		supportsInterview = s.supportsInterviewTracking(protocol)
		if session.DeviceID == "" {
			session.DeviceID = dev.ID.String()
		}
		state := "completed"
		if supportsInterview {
			state = "detected"
		}
		session.AddedDevices = upsertPairingAddedDevice(session.AddedDevices, buildPairingAddedDevice(dev, state))
		if session.knownDevices == nil {
			session.knownDevices = make(map[string]struct{})
		}
		session.knownDevices[dev.ID.String()] = struct{}{}
		if session.AllowMultipleDevices {
			session.Stage = "active"
			session.Status = "active"
			session.Message = multiDevicePairingNotice(len(session.AddedDevices))
		} else {
			session.Status = "device_detected"
		}
		if !supportsInterview && !session.AllowMultipleDevices {
			session.Active = false
		}
		return true, nil
	})
	if err != nil {
		slog.Warn("pairing candidate transition failed", "protocol", protocol, "device_id", dev.ID, "error", err)
		return
	}
	if !changed {
		return
	}
	s.emitPairingEvent(snapshot)
	go s.applyPairingMetadata(dev.ID.String(), metadata)
	if snapshot.AllowMultipleDevices {
		return
	}
	if err := s.publishPairingCommand(protocol, "stop", 0, "", "", nil); err != nil {
		slog.Warn("pairing permit stop failed", "protocol", protocol, "error", err)
	}
	if !supportsInterview {
		return
	}
}

func (s *Server) applyPairingMetadata(deviceID string, meta pairingMetadata) {
	trimmed := sanitizePairingMetadata(meta)
	if s == nil || s.repo == nil || deviceID == "" || trimmed == (pairingMetadata{}) {
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
