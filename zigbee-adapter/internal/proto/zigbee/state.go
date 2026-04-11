package zigbee

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

func (z *ZigbeeAdapter) stashPendingState(friendly string, payload []byte) {
	friendly = strings.TrimSpace(friendly)
	if friendly == "" || len(payload) == 0 {
		return
	}
	z.pendingMu.Lock()
	defer z.pendingMu.Unlock()
	// Very small, best-effort cache: keep last retained state per friendly.
	// Prevent unbounded growth if topics are misconfigured.
	if len(z.pendingState) > 500 {
		now := time.Now().UTC()
		for k, v := range z.pendingState {
			if now.Sub(v.receivedAt) > 10*time.Minute {
				delete(z.pendingState, k)
			}
		}
		if len(z.pendingState) > 500 {
			// Still too big; drop the newest request rather than growing.
			return
		}
	}
	z.pendingState[friendly] = pendingZigbeeState{payload: append([]byte(nil), payload...), receivedAt: time.Now().UTC()}
}

func (z *ZigbeeAdapter) popPendingState(friendly string) (payload []byte, ok bool) {
	friendly = strings.TrimSpace(friendly)
	if friendly == "" {
		return nil, false
	}
	z.pendingMu.Lock()
	defer z.pendingMu.Unlock()
	if ps, exists := z.pendingState[friendly]; exists {
		delete(z.pendingState, friendly)
		return ps.payload, true
	}
	return nil, false
}

func (z *ZigbeeAdapter) replayPendingState(ctx context.Context, friendly, canonical string) {
	if strings.TrimSpace(friendly) == "" || strings.TrimSpace(canonical) == "" {
		return
	}
	if !isCanonicalZigbeeExternal(canonical) {
		return
	}
	payload, ok := z.popPendingState(friendly)
	if !ok || len(payload) == 0 {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return
	}
	z.ingestState(ctx, friendly, canonical, raw, true)
}

func (z *ZigbeeAdapter) ingestState(ctx context.Context, friendly, canonical string, raw map[string]any, isReplay bool) {
	if canonical == "" || !isCanonicalZigbeeExternal(canonical) {
		return
	}
	dev, _ := z.repo.GetByExternal(ctx, "zigbee", canonical)
	if dev == nil && friendly != "" && !strings.EqualFold(canonical, friendly) {
		if legacy, _ := z.repo.GetByExternal(ctx, "zigbee", friendly); legacy != nil {
			legacy.ExternalID = canonical
			dev = legacy
		}
	}
	if dev == nil {
		// State-first discovery: create a minimal device using the canonical IEEE.
		dev = z.ensureBridgeDevice(ctx, friendly, canonical, raw)
		if dev == nil {
			return
		}
		// Best-effort: ask for full definition/exposes to enrich capabilities.
		z.requestBridgeDeviceByFriendly(friendly, "state-first-discovery")
	}
	{
		dev.Online = true
		dev.LastSeen = time.Now().UTC()
		if canonical != "" && !strings.EqualFold(dev.ExternalID, canonical) {
			dev.ExternalID = canonical
		}
		if friendly != "" && (strings.TrimSpace(dev.Name) == "" || strings.EqualFold(dev.Name, dev.ExternalID)) {
			dev.Name = friendly
		}
	}
	if mf, ok := raw["manufacturer"].(string); ok {
		dev.Manufacturer = mf
	}
	if mo, ok := raw["model"].(string); ok {
		dev.Model = mo
	}
	_ = z.repo.UpsertDevice(ctx, dev)

	state := z.normalizeState(friendly, raw)
	corr := z.consumeCorrelation(dev.ID.String())
	if corr != "" {
		state["correlation_id"] = corr
	}
	var prev map[string]any
	if cached, err := z.cache.Get(ctx, dev.ID.String()); err == nil && len(cached) > 0 {
		_ = json.Unmarshal(cached, &prev)
	}
	changes := map[string][2]any{}
	for k, v := range state {
		ov, existed := prev[k]
		if !existed || fmt.Sprint(ov) != fmt.Sprint(v) {
			changes[k] = [2]any{ov, v}
		}
	}

	sb, _ := json.Marshal(state)
	_ = z.cache.Set(ctx, dev.ID.String(), sb)
	_ = z.repo.SaveDeviceState(ctx, dev.ID.String(), sb)
	z.publishHDPState(dev, state, corr)
	z.publishHDPMeta(dev)

	if len(state) == 0 {
		slog.Warn("zigbee state empty", "device", dev.ExternalID)
		return
	}
	if isReplay {
		slog.Info("zigbee state replayed", "device", dev.ExternalID, "keys", len(state), "changes", len(changes))
	} else {
		slog.Info("zigbee state", "device", dev.ExternalID, "keys", len(state), "changes", len(changes))
	}
	for k, diff := range changes {
		slog.Debug("zigbee state change", "device", dev.ExternalID, "key", k, "old", diff[0], "new", diff[1])
	}
}

// setCorrelation records the latest correlation_id for a device so it can be echoed with the next state event.
func (z *ZigbeeAdapter) setCorrelation(deviceID, cid string) {
	if deviceID == "" || cid == "" {
		return
	}
	z.correlationMu.Lock()
	z.correlationMap[deviceID] = cid
	z.correlationMu.Unlock()
}

// consumeCorrelation retrieves and clears the pending correlation_id for a device.
func (z *ZigbeeAdapter) consumeCorrelation(deviceID string) string {
	if deviceID == "" {
		return ""
	}
	z.correlationMu.Lock()
	cid := z.correlationMap[deviceID]
	if cid != "" {
		delete(z.correlationMap, deviceID)
	}
	z.correlationMu.Unlock()
	return cid
}

func (z *ZigbeeAdapter) normalizeState(friendly string, raw map[string]any) map[string]any {
	out := map[string]any{}
	z.metaMu.RLock()
	capabilityMap := z.capIndex[friendly]
	if capabilityMap == nil && friendly != "" {
		if external := z.friendlyIndex[friendly]; external != "" {
			capabilityMap = z.capIndex[external]
		}
	}
	z.metaMu.RUnlock()
	for k, v := range raw {
		prop := strings.ToLower(k)
		if capInfo, ok := capabilityMap[prop]; ok {
			out[capInfo.ID] = normalizeValueForCapability(capInfo, v)
			continue
		}
		out[prop] = normalizeLooseValue(prop, v)
	}
	return out
}
