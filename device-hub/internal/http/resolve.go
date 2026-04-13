package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	model "github.com/PetoAdam/homenavi/device-hub/internal/devices"
)

func (s *Server) resolveDevice(ctx context.Context, deviceID string) (dev *model.Device, hdpID string, err error) {
	if s == nil || s.repo == nil {
		return nil, "", errors.New("repository unavailable")
	}
	trimmed := strings.TrimSpace(deviceID)
	if trimmed == "" {
		return nil, "", nil
	}
	if proto, ext, ok := splitHDPDeviceID(trimmed); ok {
		normExt, err := normalizeExternalID(proto, ext)
		if err != nil {
			return nil, "", err
		}
		found, err := s.repo.GetByExternal(ctx, proto, normExt)
		if err != nil {
			return nil, "", err
		}
		return found, canonicalHDPDeviceID(proto, normExt), nil
	}
	found, err := s.repo.GetByID(ctx, trimmed)
	if err != nil {
		return nil, "", err
	}
	if found == nil {
		return nil, "", nil
	}
	return found, canonicalHDPDeviceID(found.Protocol, found.ExternalID), nil
}

func splitHDPDeviceID(deviceID string) (protocol string, external string, ok bool) {
	trimmed := strings.Trim(strings.TrimSpace(deviceID), "/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", trimmed, false
	}
	proto := normalizeProtocol(parts[0])
	ext := strings.Trim(parts[1], "/")
	if proto == "" || ext == "" {
		return "", "", false
	}
	return proto, ext, true
}

func canonicalHDPDeviceID(protocol, external string) string {
	proto := normalizeProtocol(protocol)
	ext := strings.Trim(strings.TrimSpace(external), "/")
	if ext == "" {
		return ""
	}
	if proto != "" && strings.HasPrefix(ext, proto+"/") {
		return ext
	}
	if proto != "" {
		return fmt.Sprintf("%s/%s", proto, ext)
	}
	return ext
}

func normalizeExternalID(protocol, external string) (string, error) {
	proto := normalizeProtocol(protocol)
	ext := strings.TrimSpace(external)
	if proto == "" || ext == "" {
		return "", errors.New("protocol and external_id are required")
	}
	segments := strings.Split(ext, "/")
	filtered := make([]string, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			filtered = append(filtered, seg)
		}
	}
	if len(filtered) == 0 {
		return "", fmt.Errorf("external_id suffix is required")
	}
	if strings.EqualFold(filtered[0], proto) {
		filtered = filtered[1:]
	}
	if len(filtered) == 0 {
		return "", fmt.Errorf("external_id suffix is required")
	}
	normalized := strings.Join(filtered, "/")
	if proto == "zigbee" {
		normalized = strings.ToLower(normalized)
		if !zigbeeIEEEExternalIDRe.MatchString(normalized) {
			return "", fmt.Errorf("invalid zigbee external_id: %q", normalized)
		}
		return normalized, nil
	}
	return normalized, nil
}

var zigbeeIEEEExternalIDRe = regexp.MustCompile(`^0x[0-9a-f]{16}$`)

func normalizeProtocol(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func (s *Server) cleanupInvalidZigbeeDevices(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	devices, err := s.repo.List(ctx)
	if err != nil {
		slog.Warn("zigbee cleanup skipped; device list failed", "error", err)
		return
	}
	for _, dev := range devices {
		if normalizeProtocol(dev.Protocol) != "zigbee" {
			continue
		}
		norm, err := normalizeExternalID("zigbee", dev.ExternalID)
		if err == nil && norm != "" {
			continue
		}
		if err := s.repo.DeleteDeviceAndState(ctx, dev.ID.String()); err != nil {
			slog.Warn("invalid zigbee device delete failed", "device_id", dev.ID.String(), "external_id", dev.ExternalID, "error", err)
			continue
		}
		slog.Warn("invalid zigbee device removed", "device_id", dev.ID.String(), "external_id", dev.ExternalID)
	}
}

func parseTimestamp(raw any) int64 {
	switch v := raw.(type) {
	case float64:
		if v <= 0 {
			return time.Now().UnixMilli()
		}
		return int64(v)
	case int64:
		if v <= 0 {
			return time.Now().UnixMilli()
		}
		return v
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return time.Now().UnixMilli()
		}
		return int64(f)
	default:
		return time.Now().UnixMilli()
	}
}

func asString(raw any) string {
	if raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", raw)
	}
}
