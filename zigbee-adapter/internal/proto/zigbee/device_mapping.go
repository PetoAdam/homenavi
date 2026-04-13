package zigbee

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	model "github.com/PetoAdam/homenavi/zigbee-adapter/internal/devices"
	"github.com/PetoAdam/homenavi/zigbee-adapter/internal/proto/adapterutil"
)

func (z *ZigbeeAdapter) resolveExternalID(friendly string) string {
	if friendly == "" {
		return ""
	}
	friendly = strings.TrimSpace(friendly)
	z.metaMu.RLock()
	defer z.metaMu.RUnlock()
	if external, ok := z.friendlyIndex[friendly]; ok {
		return external
	}
	return ""
}

func (z *ZigbeeAdapter) resolveFriendlyName(external string) string {
	if external == "" {
		return ""
	}
	norm := adapterutil.NormalizeExternalKey(external)
	if norm == "" {
		return ""
	}
	z.metaMu.RLock()
	defer z.metaMu.RUnlock()
	return z.friendlyTopic[norm]
}

func (z *ZigbeeAdapter) setFriendlyMapping(friendly, external string) {
	friendly = strings.TrimSpace(friendly)
	if friendly == "" {
		return
	}
	external = strings.TrimSpace(external)
	z.metaMu.Lock()
	defer z.metaMu.Unlock()
	if current, ok := z.friendlyIndex[friendly]; ok {
		if norm := adapterutil.NormalizeExternalKey(current); norm != "" {
			if stored, ok2 := z.friendlyTopic[norm]; ok2 && stored == friendly {
				delete(z.friendlyTopic, norm)
			}
		}
	}
	if external == "" {
		delete(z.friendlyIndex, friendly)
		return
	}
	z.friendlyIndex[friendly] = external
	if norm := adapterutil.NormalizeExternalKey(external); norm != "" {
		z.friendlyTopic[norm] = friendly
	}
}

func (z *ZigbeeAdapter) dropFriendlyMappingsByExternal(external string) []string {
	if external == "" {
		return nil
	}
	z.metaMu.Lock()
	removed := make([]string, 0)
	for friendly, mapped := range z.friendlyIndex {
		if strings.EqualFold(mapped, external) {
			removed = append(removed, friendly)
			delete(z.friendlyIndex, friendly)
		}
	}
	if norm := adapterutil.NormalizeExternalKey(external); norm != "" {
		delete(z.friendlyTopic, norm)
	}
	z.metaMu.Unlock()
	return removed
}

func (z *ZigbeeAdapter) primeFromDB(ctx context.Context) {
	devices, err := z.repo.List(ctx)
	if err != nil {
		return
	}
	missingCaps := []string{}
	for _, d := range devices {
		if d.Protocol == "zigbee" && isCanonicalZigbeeExternal(d.ExternalID) {
			if nm := strings.TrimSpace(d.Name); nm != "" && !strings.EqualFold(nm, d.ExternalID) {
				z.setFriendlyMapping(nm, d.ExternalID)
			}
		}
		stateJSON, err := z.repo.GetDeviceState(ctx, d.ID.String())
		if err != nil {
			continue
		}
		if len(stateJSON) == 0 {
			stateJSON = []byte(`{}`)
		}
		_ = z.cache.Set(ctx, d.ID.String(), stateJSON)
		var state map[string]any
		if err := json.Unmarshal(stateJSON, &state); err == nil && len(state) > 0 {
			if d.Protocol != "zigbee" || isCanonicalZigbeeExternal(d.ExternalID) {
				z.publishHDPState(&d, state, "")
			}
		}
		if d.Protocol == "zigbee" && d.ExternalID != "" {
			if len(d.Capabilities) > 0 {
				var caps []model.Capability
				if err := json.Unmarshal(d.Capabilities, &caps); err != nil {
					slog.Warn("zigbee stored capabilities unmarshal failed", "device", d.ExternalID, "err", err)
				} else {
					capMap := map[string]model.Capability{}
					refresh := []string{}
					for _, cap := range caps {
						prop := strings.ToLower(cap.Property)
						if prop == "" {
							continue
						}
						capMap[prop] = cap
						if cap.Access.Read {
							refresh = append(refresh, prop)
						}
					}
					z.metaMu.Lock()
					if len(capMap) > 0 {
						z.capIndex[d.ExternalID] = capMap
					} else {
						delete(z.capIndex, d.ExternalID)
					}
					if len(refresh) > 0 {
						z.refreshProps[d.ExternalID] = adapterutil.UniqueStrings(refresh)
					} else {
						delete(z.refreshProps, d.ExternalID)
					}
					z.metaMu.Unlock()
				}
			} else {
				missingCaps = append(missingCaps, d.ExternalID)
			}
		}
	}
	if len(missingCaps) > 0 {
		unique := adapterutil.UniqueStrings(missingCaps)
		slog.Info("zigbee requesting capability backfill", "devices", unique)
		for _, key := range unique {
			target := strings.TrimSpace(key)
			if isCanonicalZigbeeExternal(key) {
				if friendly := z.resolveFriendlyName(key); friendly != "" {
					target = friendly
				}
			}
			if target == "" || isCanonicalZigbeeExternal(target) {
				z.requestBridgeDevicesThrottled("capability-backfill")
				continue
			}
			payload, _ := json.Marshal(map[string]string{"id": target})
			if err := z.client.Publish("zigbee2mqtt/bridge/request/device", payload); err != nil {
				slog.Debug("zigbee capability backfill publish failed", "device", target, "err", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// requestInitialStates attempts to actively trigger devices to publish their current state.
// Not all sensors respond to get requests; we issue best-effort per exposure property.
func (z *ZigbeeAdapter) requestInitialStates() {
	z.metaMu.RLock()
	copyMap := make(map[string][]string, len(z.refreshProps))
	for friendly, props := range z.refreshProps {
		if len(props) == 0 {
			continue
		}
		dup := make([]string, len(props))
		copy(dup, props)
		copyMap[friendly] = dup
	}
	z.metaMu.RUnlock()
	for key, props := range copyMap {
		target := key
		if isCanonicalZigbeeExternal(key) {
			if friendly := z.resolveFriendlyName(key); friendly != "" {
				target = friendly
			} else {
				continue
			}
		}
		z.requestStateSnapshotForDevice(target, props)
	}
}

func (z *ZigbeeAdapter) requestStateSnapshotForDevice(target string, props []string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	unique := adapterutil.UniqueStrings(props)
	if len(unique) == 0 {
		unique = []string{"state"}
	}

	allowedGetProps := make([]string, 0, len(unique))
	for _, p := range unique {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "state":
			allowedGetProps = append(allowedGetProps, "state")
		}
	}
	if len(allowedGetProps) == 0 {
		return
	}
	for _, p := range adapterutil.UniqueStrings(allowedGetProps) {
		if p == "" {
			continue
		}
		payload := map[string]any{p: ""}
		b, _ := json.Marshal(payload)
		topic := "zigbee2mqtt/" + target + "/get"
		if err := z.client.Publish(topic, b); err != nil {
			slog.Debug("zigbee get publish failed", "device", target, "prop", p, "err", err)
		} else {
			slog.Debug("zigbee get published", "device", target, "prop", p)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (z *ZigbeeAdapter) removeDevice(ctx context.Context, dev *model.Device, reason string) {
	if dev == nil {
		return
	}
	if err := z.repo.DeleteDeviceAndState(ctx, dev.ID.String()); err != nil {
		slog.Error("zigbee device delete failed", "device", dev.ExternalID, "id", dev.ID, "reason", reason, "error", err)
		return
	}
	z.cleanupRemovedDevices(ctx, []model.Device{*dev}, reason)
}

func (z *ZigbeeAdapter) cleanupRemovedDevices(ctx context.Context, removed []model.Device, reason string) {
	if len(removed) == 0 {
		return
	}
	for _, dev := range removed {
		if dev.ID == uuid.Nil {
			continue
		}
		if err := z.cache.Delete(ctx, dev.ID.String()); err != nil {
			slog.Debug("zigbee cache delete failed", "device", dev.ExternalID, "error", err)
		}
		if hdpID := z.hdpDeviceID(dev.ExternalID); hdpID != "" {
			_ = z.client.PublishWith(hdpStatePrefix+hdpID, []byte{}, true)
			_ = z.client.PublishWith(hdpMetadataPrefix+hdpID, []byte{}, true)
			z.publishHDPEvent(dev.ExternalID, "device_removed", map[string]any{"reason": reason})
		}
		removedFriendly := z.dropFriendlyMappingsByExternal(dev.ExternalID)
		z.metaMu.Lock()
		delete(z.capIndex, dev.ExternalID)
		delete(z.refreshProps, dev.ExternalID)
		for _, friendly := range removedFriendly {
			delete(z.capIndex, friendly)
			delete(z.refreshProps, friendly)
		}
		z.metaMu.Unlock()
		slog.Info("zigbee device pruned", "device", dev.ExternalID, "reason", reason)
	}
}

func (z *ZigbeeAdapter) pruneOrphanStates(ctx context.Context, keepIDs []string) {
	removed, err := z.cache.RemoveAllExcept(ctx, keepIDs)
	if err != nil {
		slog.Warn("zigbee state cache prune failed", "error", err)
		return
	}
	if len(removed) == 0 {
		return
	}
	phantoms := make([]model.Device, 0, len(removed))
	for _, id := range removed {
		u, err := uuid.Parse(id)
		if err != nil {
			slog.Debug("zigbee orphan state invalid id", "id", id, "error", err)
			continue
		}
		phantoms = append(phantoms, model.Device{ID: u, Protocol: "zigbee"})
	}
	if len(phantoms) > 0 {
		z.cleanupRemovedDevices(ctx, phantoms, "orphan-state-cache")
	}
}

func canonicalExternalID(raw map[string]any) string {
	ieee := strings.TrimSpace(adapterutil.StringField(raw, "ieee_address"))
	if ieee == "" {
		return ""
	}
	ieee = strings.ToLower(ieee)
	if strings.HasPrefix(ieee, "0x") {
		return ieee
	}
	return "0x" + strings.TrimPrefix(ieee, "0x")
}

func (z *ZigbeeAdapter) hdpDeviceID(external string) string {
	ext := strings.Trim(strings.TrimSpace(external), "/")
	if ext == "" {
		return ""
	}
	// Only allow canonical Zigbee external IDs (IEEE addresses) to become HDP IDs.
	// This prevents collisions when a non-Zigbee ID (e.g. "lgthinq/<hash>") is
	// accidentally passed in; previously we would truncate to the last segment.
	if strings.HasPrefix(strings.ToLower(ext), "zigbee/") {
		suffix := strings.TrimPrefix(ext, "zigbee/")
		if !isCanonicalZigbeeExternal(suffix) {
			return ""
		}
		return "zigbee/" + suffix
	}
	if !isCanonicalZigbeeExternal(ext) {
		return ""
	}
	return "zigbee/" + ext
}

func (z *ZigbeeAdapter) externalFromHDP(deviceID string) (protocol, external string) {
	id := strings.Trim(strings.TrimSpace(deviceID), "/")
	if id == "" {
		return "", ""
	}
	parts := strings.Split(id, "/")
	// Strict: HDP device IDs must be protocol-qualified (e.g. "zigbee/0x...").
	if len(parts) < 2 {
		return "", ""
	}
	protocol = strings.ToLower(parts[0])
	if len(parts) >= 3 {
		external = strings.Join(parts[2:], "/")
	} else {
		external = strings.Join(parts[1:], "/")
	}
	return protocol, external
}
