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

	"github.com/PetoAdam/homenavi/device-hub/internal/model"
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
		if err := json.NewDecoder(io.LimitReader(r.Body, 8*1024)).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		protocol := normalizeProtocol(req.Protocol)
		if protocol == "" {
			http.Error(w, "protocol is required", http.StatusBadRequest)
			return
		}
		session, err := s.startPairing(protocol, req.Timeout, req.Metadata)
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
				_ = s.publishPairingCommand(item.Protocol, "stop", 0)
				s.emitPairingEvent(item)
			}
		}(expired)
	}
	return result
}

func (s *Server) startPairing(protocol string, timeout int, metadata pairingMetadata) (*pairingSession, error) {
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
	session := &pairingSession{
		ID:           uuid.NewString(),
		Protocol:     protocol,
		Status:       "starting",
		Active:       true,
		StartedAt:    now,
		ExpiresAt:    now.Add(time.Duration(timeout) * time.Second),
		Metadata:     meta,
		knownDevices: s.snapshotKnownDevices(),
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
			_ = s.publishPairingCommand(protocol, "stop", 0)
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
	if err := s.publishPairingCommand(protocol, "start", timeout); err != nil {
		s.pairingMu.Lock()
		delete(s.pairings, protocol)
		s.pairingMu.Unlock()
		return nil, fmt.Errorf("failed to start pairing: %w", err)
	}
	session.Status = "active"
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
	_ = s.publishPairingCommand(protocol, "stop", 0)
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
	_ = s.publishPairingCommand(protocol, "stop", 0)
	s.emitPairingEvent(clone)
}

func (s *Server) publishPairingCommand(protocol, action string, timeout int) error {
	hdpPayload := map[string]any{
		"schema":   hdpSchema,
		"type":     "pairing_command",
		"protocol": protocol,
		"action":   action,
	}
	if timeout > 0 {
		hdpPayload["timeout_sec"] = timeout
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
		"schema":   hdpSchema,
		"type":     "pairing_progress",
		"protocol": session.Protocol,
		"origin":   "device-hub",
		"stage":    session.Status,
		"status":   session.Status,
		"ts":       time.Now().UnixMilli(),
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
	if b, err := json.Marshal(envelope); err == nil {
		if err := s.mqtt.Publish(hdpPairingProgressPrefix+session.Protocol, b); err != nil {
			slog.Warn("hdp pairing progress publish failed", "protocol", session.Protocol, "error", err)
		}
	}
}

func (s *Server) handlePairingProgressEvent(_ paho.Client, msg mqttinfra.Message) {
	if len(msg.Payload()) == 0 {
		return
	}
	var evt struct {
		Protocol   string `json:"protocol"`
		Stage      string `json:"stage"`
		Status     string `json:"status"`
		ExternalID string `json:"external_id"`
	}
	if err := json.Unmarshal(msg.Payload(), &evt); err != nil {
		slog.Debug("pairing progress decode failed", "error", err)
		return
	}
	s.processPairingProgress(evt.Protocol, evt.Stage, evt.Status, evt.ExternalID)
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
	external := asString(evt["external_id"])
	if external == "" {
		external = asString(evt["device_id"])
	}
	s.processPairingProgress(protocol, stage, status, external)
}

func (s *Server) processPairingProgress(protocol, stage, status, externalID string) {
	proto := normalizeProtocol(protocol)
	if proto == "" {
		return
	}
	stage = strings.TrimSpace(strings.ToLower(stage))
	status = strings.TrimSpace(strings.ToLower(status))
	if stage == "timeout" || stage == "stopped" || stage == "failed" || stage == "error" || stage == "completed" ||
		status == "timeout" || status == "stopped" || status == "failed" || status == "error" || status == "completed" {
		s.pairingMu.Lock()
		session, ok := s.pairings[proto]
		if !ok || !session.Active {
			s.pairingMu.Unlock()
			return
		}
		if stage != "" {
			session.Status = stage
		} else if status != "" {
			session.Status = status
		}
		session.Active = false
		session.ExpiresAt = time.Now().UTC()
		if session.cancel != nil {
			session.cancel()
			session.cancel = nil
		}
		snapshot := session.clone()
		s.pairingMu.Unlock()
		s.emitPairingEvent(snapshot)
		return
	}
	if !s.supportsInterviewTracking(proto) {
		return
	}
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
	updated := false
	finalStatus := ""
	switch stage {
	case "device_joined", "device_announced":
		session.Status = "device_joined"
		session.awaitingInterview = true
		updated = true
	case "interview_started", "interviewing":
		session.Status = "interviewing"
		session.awaitingInterview = true
		updated = true
	case "interview_succeeded", "interview_complete", "completed":
		session.Status = "interview_complete"
		session.awaitingInterview = false
		updated = true
		finalStatus = "completed"
	case "interview_failed", "failed":
		session.Status = "failed"
		session.Active = false
		session.awaitingInterview = false
		updated = true
		finalStatus = "failed"
	default:
		s.pairingMu.Unlock()
		return
	}
	snapshot := session.clone()
	s.pairingMu.Unlock()
	if !updated {
		return
	}
	s.emitPairingEvent(snapshot)
	if finalStatus != "" {
		go func(proto, status string) {
			if _, err := s.stopPairing(proto, status); err != nil && !errors.Is(err, errPairingNotFound) {
				slog.Warn("pairing finalize failed", "protocol", proto, "status", status, "error", err)
			}
		}(proto, finalStatus)
	}
}

func (s *Server) shouldAcceptPairingCandidate(session *pairingSession, dev *model.Device) bool {
	if session == nil || dev == nil {
		return false
	}
	if session.DeviceID != "" {
		return false
	}
	if len(session.knownDevices) > 0 {
		if _, exists := session.knownDevices[dev.ID.String()]; exists {
			return false
		}
	}
	if session.candidateExternalID != "" && dev.ExternalID != "" {
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
	session.DeviceID = deviceID
	session.Status = "device_detected"
	if supportsInterview {
		session.awaitingInterview = true
	} else {
		session.Active = false
	}
	snapshot := session.clone()
	s.pairingMu.Unlock()
	s.emitPairingEvent(snapshot)
	go s.applyPairingMetadata(deviceID, meta)
	go func(proto string, deferCompletion bool) {
		if err := s.publishPairingCommand(proto, "stop", 0); err != nil {
			slog.Warn("pairing permit stop failed", "protocol", proto, "error", err)
		}
		if !deferCompletion {
			if _, err := s.stopPairing(proto, "completed"); err != nil && !errors.Is(err, errPairingNotFound) {
				slog.Warn("pairing stop failed", "protocol", proto, "error", err)
			}
		}
	}(protocol, supportsInterview)
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
