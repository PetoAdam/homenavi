package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	mqttinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/mqtt"
	model "github.com/PetoAdam/homenavi/device-hub/internal/devices"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
)

func (s *Server) handleHDPAdapterHello(_ paho.Client, msg mqttinfra.Message) {
	if s == nil || s.adapters == nil {
		return
	}
	s.adapters.upsertFromHello(msg.Payload())
}

func (s *Server) handleHDPAdapterStatus(_ paho.Client, msg mqttinfra.Message) {
	if s == nil || s.adapters == nil {
		return
	}
	s.adapters.upsertFromStatusTopic(msg.Topic(), msg.Payload())
}

func (s *Server) handleHDPMetadataEvent(_ paho.Client, msg mqttinfra.Message) {
	deviceID := strings.TrimPrefix(msg.Topic(), hdpMetadataPrefix)
	if deviceID == msg.Topic() || deviceID == "" {
		return
	}
	slog.Info("hdp metadata received", "device_id", deviceID)
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		slog.Debug("hdp metadata decode failed", "topic", msg.Topic(), "error", err)
		return
	}
	s.upsertMetadataFromHDP(deviceID, payload)
}

func (s *Server) handleHDPEvent(_ paho.Client, msg mqttinfra.Message) {
	if len(msg.Payload()) == 0 {
		return
	}
	var evt map[string]any
	if err := json.Unmarshal(msg.Payload(), &evt); err != nil {
		slog.Debug("hdp event decode failed", "topic", msg.Topic(), "error", err)
		return
	}
	deviceID := asString(evt["device_id"])
	if deviceID == "" {
		deviceID = strings.TrimPrefix(msg.Topic(), hdpEventPrefix)
	}
	if deviceID == "" {
		return
	}
	if asString(evt["event"]) == "device_removed" {
		ctx := context.Background()
		dev, _, err := s.resolveDevice(ctx, deviceID)
		if err != nil {
			slog.Warn("hdp device remove lookup failed", "device_id", deviceID, "error", err)
			return
		}
		if dev != nil {
			_ = s.repo.DeleteDeviceAndState(ctx, dev.ID.String())
		}
	}
}

func (s *Server) upsertMetadataFromHDP(deviceID string, payload map[string]any) {
	if deviceID == "" {
		return
	}
	ctx := context.Background()
	proto := normalizeProtocol(asString(payload["protocol"]))
	external := strings.TrimSpace(deviceID)
	if parsedProto, parsedExternal, ok := splitHDPDeviceID(deviceID); ok {
		proto = parsedProto
		external = parsedExternal
	}
	if proto == "" {
		slog.Warn("hdp metadata missing protocol", "device_id", deviceID)
		return
	}
	normExt, err := normalizeExternalID(proto, external)
	if err != nil {
		slog.Warn("hdp metadata normalize external failed", "device_id", deviceID, "protocol", proto, "error", err)
		return
	}
	dev, err := s.repo.GetByExternal(ctx, proto, normExt)
	if err != nil {
		slog.Warn("hdp metadata lookup failed", "device_id", deviceID, "protocol", proto, "external", normExt, "error", err)
		return
	}
	if dev == nil {
		dev = &model.Device{ExternalID: normExt, Protocol: proto}
	}
	changed := false
	if m := strings.TrimSpace(asString(payload["manufacturer"])); m != "" && dev.Manufacturer != m {
		dev.Manufacturer = m
		changed = true
	}
	if m := strings.TrimSpace(asString(payload["model"])); m != "" && dev.Model != m {
		dev.Model = m
		changed = true
	}
	if d := strings.TrimSpace(asString(payload["description"])); d != "" && dev.Description != d {
		dev.Description = d
		changed = true
	}
	if ic := strings.TrimSpace(asString(payload["icon"])); dev.Icon != ic {
		dev.Icon = ic
		changed = true
	}
	if dev.Protocol != proto {
		dev.Protocol = proto
		changed = true
	}
	if caps, ok := payload["capabilities"]; ok {
		if b, err := json.Marshal(caps); err == nil {
			dev.Capabilities = b
			changed = true
		}
	}
	if inputs, ok := payload["inputs"]; ok {
		if b, err := json.Marshal(inputs); err == nil {
			dev.Inputs = b
			changed = true
		}
	}
	if !changed && dev.ID != uuid.Nil {
		return
	}
	if err := s.repo.UpsertDevice(ctx, dev); err != nil {
		slog.Warn("hdp metadata upsert failed", "device_id", deviceID, "error", err)
		return
	}
}

func (s *Server) handleHDPStateEvent(_ paho.Client, msg mqttinfra.Message) {
	topic := strings.TrimPrefix(msg.Topic(), hdpStatePrefix)
	if topic == msg.Topic() {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		slog.Debug("hdp state decode failed", "topic", msg.Topic(), "error", err)
		return
	}
	deviceID := strings.TrimSpace(asString(payload["device_id"]))
	if deviceID == "" {
		deviceID = strings.TrimSpace(topic)
	}
	state := map[string]any{}
	if st, ok := payload["state"].(map[string]any); ok && len(st) > 0 {
		state = st
	}
	ts := parseTimestamp(payload["ts"])
	corr := strings.TrimSpace(asString(payload["corr"]))
	if len(state) == 0 {
		return
	}
	slog.Info("hdp state received", "device_id", deviceID, "corr", corr, "ts", ts, "keys", len(state))
	s.consumeState(deviceID, state, corr, ts, false)
}

func (s *Server) consumeState(deviceTopicID string, state map[string]any, corr string, ts int64, publishHDP bool) {
	deviceKey := strings.TrimSpace(deviceTopicID)
	if deviceKey == "" || len(state) == 0 {
		return
	}
	ctx := context.Background()
	deviceUUID := deviceKey
	externalID := deviceKey
	dev, hdpID, err := s.resolveDevice(ctx, deviceKey)
	if err != nil {
		slog.Warn("state lookup failed", "device_id", deviceKey, "error", err)
	}
	if dev != nil {
		deviceUUID = dev.ID.String()
		if hdpID != "" {
			externalID = hdpID
		}
		_ = s.repo.TouchOnline(ctx, deviceUUID)
		if b, err := json.Marshal(state); err == nil {
			if err := s.repo.SaveDeviceState(ctx, deviceUUID, b); err != nil {
				slog.Warn("state persist failed", "device_id", deviceUUID, "error", err)
			}
		}
	}
	slog.Info("state persisted", "device_id", deviceUUID, "external_id", externalID, "corr", corr, "ts", ts, "keys", len(state))
	s.processCommandStateLifecycle(externalID, state, corr)
	if !publishHDP {
		return
	}
	envelope := map[string]any{
		"schema":    hdpSchema,
		"type":      "state",
		"device_id": externalID,
		"ts":        ts,
		"state":     state,
	}
	if corr != "" {
		envelope["corr"] = corr
	}
	if data, err := json.Marshal(envelope); err == nil {
		if err := s.mqtt.PublishWith(hdpStatePrefix+externalID, data, true); err != nil {
			slog.Warn("hdp state publish failed", "device_id", externalID, "error", err)
		}
	}
}
