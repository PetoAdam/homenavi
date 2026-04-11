package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/device-hub/internal/model"
	"github.com/google/uuid"
)

func normalizeColorValue(input *model.DeviceInput, value any) any {
	if value == nil {
		return value
	}
	if m, ok := value.(map[string]any); ok {
		if hexRaw, found := m["hex"]; found {
			if hex, ok := hexRaw.(string); ok {
				return map[string]any{"hex": strings.TrimSpace(hex)}
			}
		}
		return value
	}
	if s, ok := value.(string); ok {
		return map[string]any{"hex": strings.TrimSpace(s)}
	}
	return value
}

func (s *Server) publishDeviceMetadata(dev *model.Device) {
	if dev == nil {
		return
	}
	s.handlePairingCandidate(dev)
	s.publishHDPMeta(dev)
}

func (s *Server) publishDeviceRemoval(dev *model.Device, reason string) {
	if dev == nil {
		return
	}
	hdpID := canonicalHDPDeviceID(dev.Protocol, dev.ExternalID)
	if hdpID != "" {
		_ = s.mqtt.PublishWith(hdpMetadataPrefix+hdpID, []byte{}, true)
		_ = s.mqtt.PublishWith(hdpStatePrefix+hdpID, []byte{}, true)
		_ = s.mqtt.PublishWith(hdpCommandResultPrefix+hdpID, []byte{}, true)
	}
	payload := map[string]any{
		"id":          dev.ID.String(),
		"device_id":   hdpID,
		"external_id": dev.ExternalID,
		"protocol":    dev.Protocol,
		"reason":      reason,
	}
	if data, err := json.Marshal(payload); err == nil {
		if err := s.mqtt.Publish(hdpEventPrefix+hdpID, data); err != nil {
			slog.Warn("device removal publish failed", "device_id", dev.ID, "error", err)
		}
	}
	slog.Info("device removal broadcast", "device_id", dev.ID.String(), "protocol", dev.Protocol, "reason", reason)
	s.publishHDPEvent(hdpID, "device_removed", map[string]any{"reason": reason})
}

func (s *Server) publishHDPMeta(dev *model.Device) {
	if dev == nil {
		return
	}
	hdpID := canonicalHDPDeviceID(dev.Protocol, dev.ExternalID)
	envelope := map[string]any{
		"schema":       hdpSchema,
		"type":         "metadata",
		"device_id":    hdpID,
		"protocol":     dev.Protocol,
		"manufacturer": dev.Manufacturer,
		"model":        dev.Model,
		"description":  dev.Description,
		"icon":         dev.Icon,
		"ts":           time.Now().UnixMilli(),
	}
	if len(dev.Capabilities) > 0 {
		var caps any
		if err := json.Unmarshal(dev.Capabilities, &caps); err == nil {
			envelope["capabilities"] = caps
		}
	}
	if len(dev.Inputs) > 0 {
		var inputs any
		if err := json.Unmarshal(dev.Inputs, &inputs); err == nil {
			envelope["inputs"] = inputs
		}
	}
	if data, err := json.Marshal(envelope); err == nil {
		if err := s.mqtt.PublishWith(hdpMetadataPrefix+hdpID, data, true); err != nil {
			slog.Warn("hdp metadata publish failed", "device_id", hdpID, "error", err)
		}
	}
}

func (s *Server) publishHDPEvent(deviceID, event string, data map[string]any) {
	if strings.TrimSpace(deviceID) == "" || strings.TrimSpace(event) == "" {
		return
	}
	envelope := map[string]any{
		"schema":    hdpSchema,
		"type":      "event",
		"device_id": deviceID,
		"event":     event,
		"ts":        time.Now().UnixMilli(),
	}
	if len(data) > 0 {
		envelope["data"] = data
	}
	if b, err := json.Marshal(envelope); err == nil {
		if err := s.mqtt.Publish(hdpEventPrefix+deviceID, b); err != nil {
			slog.Warn("hdp event publish failed", "device_id", deviceID, "event", event, "error", err)
		}
	}
}

func (s *Server) publishHDPCommand(deviceID string, cmd map[string]any) error {
	if strings.TrimSpace(deviceID) == "" || cmd == nil {
		return errors.New("device_id and command are required")
	}
	if _, ok := cmd["device_id"]; !ok {
		cmd["device_id"] = deviceID
	}
	if _, ok := cmd["schema"]; !ok {
		cmd["schema"] = hdpSchema
	}
	if _, ok := cmd["type"]; !ok {
		cmd["type"] = "command"
	}
	if _, ok := cmd["ts"]; !ok {
		cmd["ts"] = time.Now().UnixMilli()
	}
	if b, err := json.Marshal(cmd); err == nil {
		if err := s.mqtt.Publish(hdpCommandPrefix+deviceID, b); err != nil {
			slog.Warn("hdp command publish failed", "device_id", deviceID, "error", err)
			return err
		}
		return nil
	}
	return errors.New("failed to encode command")
}

func (s *Server) writeEmptyArray(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("[]"))
}

func (s *Server) buildDeviceItem(ctx context.Context, d *model.Device) (deviceListItem, error) {
	stateJSON, err := s.repo.GetDeviceState(ctx, d.ID.String())
	if err != nil {
		slog.Warn("device state lookup failed", "device_id", d.ID.String(), "error", err)
		stateJSON = nil
	}
	if len(stateJSON) == 0 {
		stateJSON = []byte(`{}`)
	}
	item := deviceListItem{
		ID:           d.ID,
		DeviceID:     canonicalHDPDeviceID(d.Protocol, d.ExternalID),
		Protocol:     d.Protocol,
		ExternalID:   d.ExternalID,
		Type:         d.Type,
		Manufacturer: d.Manufacturer,
		Model:        d.Model,
		Description:  d.Description,
		Firmware:     d.Firmware,
		Icon:         d.Icon,
		Online:       d.Online,
		LastSeen:     d.LastSeen,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
		State:        json.RawMessage(append([]byte(nil), stateJSON...)),
	}
	if len(d.Capabilities) > 0 {
		item.Capabilities = json.RawMessage(append([]byte(nil), d.Capabilities...))
	}
	if len(d.Inputs) > 0 {
		item.Inputs = json.RawMessage(append([]byte(nil), d.Inputs...))
	}
	return item, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

type unsupportedProtocolError struct {
	protocol string
}

func (e unsupportedProtocolError) Error() string {
	return "protocol " + e.protocol + " does not support this operation"
}

func (e unsupportedProtocolError) Is(target error) bool {
	return target == errPairingUnsupported
}

func (s *Server) requestProtocolRemoval(dev *model.Device) error {
	if dev == nil {
		return errors.New("device is required")
	}
	if s == nil || s.mqtt == nil {
		return errors.New("mqtt client unavailable")
	}
	hdpID := canonicalHDPDeviceID(dev.Protocol, dev.ExternalID)
	if strings.TrimSpace(hdpID) == "" {
		return errors.New("device_id missing")
	}
	payload := map[string]any{
		"schema":      hdpSchema,
		"type":        "command",
		"device_id":   hdpID,
		"protocol":    dev.Protocol,
		"command":     "remove_device",
		"ts":          time.Now().UnixMilli(),
		"corr":        uuid.NewString(),
		"external_id": dev.ExternalID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.mqtt.Publish(hdpCommandPrefix+hdpID, b)
}
