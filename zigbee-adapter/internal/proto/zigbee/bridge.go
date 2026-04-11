package zigbee

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	model "github.com/PetoAdam/homenavi/zigbee-adapter/internal/devices"
	"github.com/PetoAdam/homenavi/zigbee-adapter/internal/proto/adapterutil"
)

func (z *ZigbeeAdapter) reconcileFriendlyDevice(ctx context.Context, friendly, external string) {
	if friendly == "" || external == "" || strings.EqualFold(friendly, external) {
		return
	}
	if !isCanonicalZigbeeExternal(external) {
		return
	}
	canonicalDev, _ := z.repo.GetByExternal(ctx, "zigbee", external)
	friendlyDev, _ := z.repo.GetByExternal(ctx, "zigbee", friendly)
	if canonicalDev == nil && friendlyDev != nil {
		friendlyDev.ExternalID = external
		if strings.TrimSpace(friendlyDev.Name) == "" || strings.EqualFold(friendlyDev.Name, friendlyDev.ExternalID) {
			friendlyDev.Name = friendly
		}
		if err := z.repo.UpsertDevice(ctx, friendlyDev); err == nil {
			return
		}
	}
	if canonicalDev != nil && friendlyDev != nil && canonicalDev.ID != friendlyDev.ID {
		if strings.TrimSpace(canonicalDev.Name) == "" || strings.EqualFold(canonicalDev.Name, canonicalDev.ExternalID) {
			if nm := strings.TrimSpace(friendlyDev.Name); nm != "" {
				canonicalDev.Name = nm
			} else {
				canonicalDev.Name = friendly
			}
			_ = z.repo.UpsertDevice(ctx, canonicalDev)
		}
		if err := z.repo.DeleteDeviceAndState(ctx, friendlyDev.ID.String()); err == nil {
			z.cleanupRemovedDevices(ctx, []model.Device{*friendlyDev}, "dedupe-friendly")
		}
	}
}

func (z *ZigbeeAdapter) handleBridgeEvent(_ paho.Client, m paho.Message) {
	var evt struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(m.Payload(), &evt); err != nil {
		return
	}
	ctx := context.Background()
	switch evt.Type {
	case "device_joined", "device_announce":
		friendly, _ := evt.Data["friendly_name"].(string)
		if friendly == "" {
			return
		}
		external := canonicalExternalID(evt.Data)
		if external == "" {
			external = z.resolveExternalID(friendly)
		}
		if external == "" {
			z.requestBridgeDevicesThrottled("bridge-event-join")
			return
		}
		z.setFriendlyMapping(friendly, external)
		z.reconcileFriendlyDevice(ctx, friendly, external)
		if dev := z.ensureBridgeDevice(ctx, friendly, external, evt.Data); dev != nil {
			z.publishPairingProgress(bridgeLifecycleStage(evt.Type), "", external, friendly)
		}
	case "device_interview":
		friendly, _ := evt.Data["friendly_name"].(string)
		if friendly == "" {
			return
		}
		external := canonicalExternalID(evt.Data)
		if external == "" {
			external = z.resolveExternalID(friendly)
		}
		if external == "" {
			z.requestBridgeDevicesThrottled("bridge-event-interview")
			return
		}
		z.setFriendlyMapping(friendly, external)
		z.reconcileFriendlyDevice(ctx, friendly, external)
		status := adapterutil.StringField(evt.Data, "status")
		if dev := z.ensureBridgeDevice(ctx, friendly, external, evt.Data); dev != nil {
			stage := interviewStageFromStatus(status)
			z.publishPairingProgress(stage, status, external, friendly)
			if stage == "interview_complete" {
				// If pairing was started via HDP permit_join, stop the timer now.
				// Otherwise a later permit_join timeout would overwrite a successful pairing.
				z.pairingMu.Lock()
				active := z.pairingActive
				z.pairingMu.Unlock()
				if active {
					z.stopPairing()
				}
				// Frontend advances the pairing flow only once it sees a terminal "completed" stage.
				z.publishPairingProgress("completed", status, external, friendly)
			}
		}
	case "device_removed", "device_leave", "device_left":
		friendly, _ := evt.Data["friendly_name"].(string)
		external := z.resolveExternalID(friendly)
		if external == "" {
			external = canonicalExternalID(evt.Data)
		}
		var dev *model.Device
		if external != "" {
			dev, _ = z.repo.GetByExternal(ctx, "zigbee", external)
		}
		if dev == nil && friendly != "" {
			dev, _ = z.repo.GetByExternal(ctx, "zigbee", friendly)
		}
		if dev != nil {
			z.removeDevice(ctx, dev, "zigbee-event")
		} else {
			slog.Warn("zigbee device_removed missing record", "friendly", friendly, "external", external)
		}
	case "device_renamed":
		from := adapterutil.StringField(evt.Data, "from")
		to := adapterutil.StringField(evt.Data, "to")
		if from == "" || to == "" || strings.EqualFold(from, to) {
			return
		}
		external := z.resolveExternalID(from)
		if external == "" {
			external = z.resolveExternalID(to)
		}
		if external == "" {
			external = canonicalExternalID(evt.Data)
		}
		dev, _ := z.repo.GetByExternal(ctx, "zigbee", external)
		if dev == nil {
			if external != "" {
				dev, _ = z.repo.GetByExternal(ctx, "zigbee", to)
			}
			if dev == nil {
				dev, _ = z.repo.GetByExternal(ctx, "zigbee", from)
			}
			if dev == nil {
				return
			}
		}
		if strings.TrimSpace(dev.Name) == "" || strings.EqualFold(dev.Name, from) || strings.EqualFold(dev.Name, dev.ExternalID) {
			dev.Name = to
		}
		dev.LastSeen = time.Now().UTC()
		if err := z.repo.UpsertDevice(ctx, dev); err != nil {
			slog.Error("zigbee rename upsert failed", "from", from, "to", to, "error", err)
			return
		}
		z.setFriendlyMapping(from, "")
		z.setFriendlyMapping(to, dev.ExternalID)
		z.metaMu.Lock()
		if entry, ok := z.capIndex[from]; ok {
			z.capIndex[to] = entry
			delete(z.capIndex, from)
		}
		if props, ok := z.refreshProps[from]; ok {
			z.refreshProps[to] = props
			delete(z.refreshProps, from)
		}
		z.metaMu.Unlock()
		if b, err := json.Marshal(map[string]string{"id": to}); err == nil {
			_ = z.client.Publish("zigbee2mqtt/bridge/request/device", b)
		}
		slog.Info("zigbee device renamed", "from", from, "to", to)
	}
}

func (z *ZigbeeAdapter) ensureBridgeDevice(ctx context.Context, friendly, external string, data map[string]any) *model.Device {
	if external == "" {
		return nil
	}
	dev, _ := z.repo.GetByExternal(ctx, "zigbee", external)
	if dev == nil && friendly != "" && !strings.EqualFold(external, friendly) {
		if legacy, _ := z.repo.GetByExternal(ctx, "zigbee", friendly); legacy != nil {
			legacy.ExternalID = external
			dev = legacy
		}
	}
	if dev == nil {
		name := friendly
		if name == "" {
			name = external
		}
		dev = &model.Device{ID: uuid.New(), Protocol: "zigbee", ExternalID: external, Name: name}
	} else if external != "" && !strings.EqualFold(dev.ExternalID, external) {
		dev.ExternalID = external
	}
	dev.Online = true
	dev.LastSeen = time.Now().UTC()
	if mf, ok := data["manufacturer"].(string); ok {
		dev.Manufacturer = mf
	}
	if mo, ok := data["model"].(string); ok {
		dev.Model = mo
	}
	if friendly != "" && (strings.TrimSpace(dev.Name) == "" || strings.EqualFold(dev.Name, dev.ExternalID)) {
		dev.Name = friendly
	}
	if err := z.repo.UpsertDevice(ctx, dev); err != nil {
		slog.Warn("zigbee bridge device upsert failed", "external", external, "error", err)
		return nil
	}
	return dev
}

func (z *ZigbeeAdapter) handleBridgeDevices(m paho.Message) {
	var list []map[string]any
	if err := json.Unmarshal(m.Payload(), &list); err != nil {
		slog.Warn("bridge devices unmarshal", "err", err)
		return
	}
	ctx := context.Background()
	discovered := 0
	metadataUpdates := 0
	seen := make(map[string]string, len(list))
	for _, d := range list {
		if strings.EqualFold(adapterutil.StringField(d, "type"), "coordinator") {
			continue
		}
		if supported, ok := d["supported"].(bool); ok && !supported {
			continue
		}
		friendly := adapterutil.StringField(d, "friendly_name")
		external := canonicalExternalID(d)
		if external == "" {
			continue
		}
		if friendly != "" {
			z.setFriendlyMapping(friendly, external)
		}
		discovered++
		dev, exposures := z.upsertBridgeDevice(ctx, d, "devices")
		if dev != nil {
			seen[dev.ExternalID] = dev.ID.String()
			if friendly != "" {
				z.replayPendingState(ctx, friendly, dev.ExternalID)
			}
		}
		if exposures {
			metadataUpdates++
		}
	}
	slog.Info("zigbee bridge devices", "count", discovered, "metadata_updates", metadataUpdates, "refresh", z.refreshOnStart)
	if discovered > 0 {
		existing, err := z.repo.List(ctx)
		if err != nil {
			slog.Warn("zigbee bridge prune list failed", "error", err)
		} else {
			nonZigbeeIDs := make([]string, 0)
			for idx := range existing {
				dev := &existing[idx]
				if dev.Protocol != "zigbee" {
					if dev.ID != uuid.Nil {
						nonZigbeeIDs = append(nonZigbeeIDs, dev.ID.String())
					}
					continue
				}
				if dev.ExternalID == "" {
					continue
				}
				if _, ok := seen[dev.ExternalID]; ok {
					continue
				}
				// Backward-compat: legacy rows stored ExternalID as "zigbee/<ieee>".
				// If the canonical IEEE exists in this refresh, delete the legacy row without
				// clearing retained HDP topics (which would otherwise wipe the canonical metadata).
				lower := strings.ToLower(dev.ExternalID)
				if strings.HasPrefix(lower, "zigbee/") {
					legacy := strings.TrimSpace(dev.ExternalID[len("zigbee/"):])
					if legacy != "" {
						if _, ok := seen[legacy]; ok {
							if err := z.repo.DeleteDeviceAndState(ctx, dev.ID.String()); err != nil {
								slog.Warn("zigbee legacy row delete failed", "device", dev.ExternalID, "id", dev.ID, "error", err)
							} else {
								slog.Info("zigbee legacy row pruned", "device", dev.ExternalID, "reason", "bridge-refresh-legacy")
							}
							continue
						}
					}
				}
				z.removeDevice(ctx, dev, "bridge-refresh-prune")
			}
			keepIDs := make([]string, 0, len(seen)+len(nonZigbeeIDs))
			for _, id := range seen {
				if id != "" {
					keepIDs = append(keepIDs, id)
				}
			}
			keepIDs = append(keepIDs, nonZigbeeIDs...)
			z.pruneOrphanStates(ctx, keepIDs)
			if removedStates, err := z.repo.DeleteDeviceStatesNotIn(ctx, keepIDs); err != nil {
				slog.Warn("zigbee state db prune failed", "error", err)
			} else if len(removedStates) > 0 {
				slog.Info("zigbee state db pruned", "removed", len(removedStates))
			}
		}
	}
	shouldRefresh := false
	z.metaMu.Lock()
	if z.refreshOnStart && discovered > 0 {
		z.refreshOnStart = false
		shouldRefresh = true
	}
	z.metaMu.Unlock()
	if shouldRefresh {
		go z.requestInitialStates()
	}
}

func (z *ZigbeeAdapter) handleBridgeDeviceResponse(m paho.Message) {
	var resp struct {
		Data   map[string]any `json:"data"`
		Status string         `json:"status"`
		Error  any            `json:"error"`
		Msg    string         `json:"message"`
	}
	if err := json.Unmarshal(m.Payload(), &resp); err != nil {
		slog.Warn("bridge device response unmarshal", "err", err)
		return
	}
	if resp.Status != "" && !strings.EqualFold(resp.Status, "ok") && !strings.EqualFold(resp.Status, "success") {
		slog.Warn("zigbee bridge device response status", "status", resp.Status, "message", resp.Msg, "error", resp.Error)
	}
	if resp.Data == nil {
		return
	}
	ctx := context.Background()
	dev, _ := z.upsertBridgeDevice(ctx, resp.Data, "device-response")
	if dev != nil {
		friendly := strings.TrimSpace(adapterutil.StringField(resp.Data, "friendly_name"))
		if friendly != "" {
			z.replayPendingState(ctx, friendly, dev.ExternalID)
		}
	}
}

func (z *ZigbeeAdapter) upsertBridgeDevice(ctx context.Context, raw map[string]any, source string) (*model.Device, bool) {
	origFriendly := strings.TrimSpace(adapterutil.StringField(raw, "friendly_name"))
	friendly := origFriendly
	if friendly == "" {
		friendly = adapterutil.StringField(raw, "id")
	}
	external := canonicalExternalID(raw)
	if external == "" {
		slog.Debug("zigbee bridge device missing ieee address", "source", source, "friendly", friendly)
		return nil, false
	}
	if friendly != "" {
		z.setFriendlyMapping(friendly, external)
		z.reconcileFriendlyDevice(ctx, friendly, external)
	}
	dev, _ := z.repo.GetByExternal(ctx, "zigbee", external)
	if dev == nil && friendly != "" && !strings.EqualFold(external, friendly) {
		if legacy, _ := z.repo.GetByExternal(ctx, "zigbee", friendly); legacy != nil {
			legacy.ExternalID = external
			dev = legacy
		}
	}
	isNew := dev == nil
	if dev == nil {
		name := friendly
		if name == "" {
			name = external
		}
		dev = &model.Device{ID: uuid.New(), Protocol: "zigbee", ExternalID: external, Name: name}
	} else {
		if !strings.EqualFold(dev.ExternalID, external) {
			dev.ExternalID = external
		}
		if friendly != "" && (strings.TrimSpace(dev.Name) == "" || strings.EqualFold(dev.Name, dev.ExternalID)) {
			dev.Name = friendly
		}
	}
	if typ := adapterutil.StringField(raw, "type"); typ != "" {
		dev.Type = typ
	}
	if mf := adapterutil.StringField(raw, "manufacturer"); mf != "" {
		dev.Manufacturer = mf
	}
	if mo := adapterutil.StringField(raw, "model"); mo != "" {
		dev.Model = mo
	}
	if fw := adapterutil.StringField(raw, "software_build_id"); fw != "" {
		dev.Firmware = fw
	} else if fw := adapterutil.StringField(raw, "date_code"); fw != "" {
		dev.Firmware = fw
	}
	if desc := adapterutil.StringField(raw, "description"); desc != "" {
		dev.Description = desc
	}

	if def, ok := raw["definition"].(map[string]any); ok {
		if dev.Type == "" {
			if defType := adapterutil.StringField(def, "type"); defType != "" {
				dev.Type = defType
			}
		}
		if dev.Manufacturer == "" {
			if vendor := adapterutil.StringField(def, "vendor"); vendor != "" {
				dev.Manufacturer = vendor
			}
		}
		if dev.Model == "" {
			if model := adapterutil.StringField(def, "model"); model != "" {
				dev.Model = model
			}
		}
		if dev.Description == "" {
			if desc := adapterutil.StringField(def, "description"); desc != "" {
				dev.Description = desc
			}
		}
	}

	adapterutil.SanitizeDeviceStrings(dev)

	exposes, exposuresFound := extractExposes(raw)
	if isNew && !exposuresFound {
		// Zigbee2MQTT sometimes emits partial device payloads (events/startup) without
		// exposes/definition yet. Create a minimal device so it appears in inventory,
		// then request full metadata to enrich capabilities.
		dev.Online = true
		dev.LastSeen = time.Now().UTC()
		if err := z.repo.UpsertDevice(ctx, dev); err != nil {
			slog.Error("zigbee device upsert failed", "device", friendly, "err", err)
		}
		z.publishHDPMeta(dev)
		z.requestBridgeDeviceByFriendly(origFriendly, "bridge-device-missing-exposes")
		if strings.TrimSpace(origFriendly) == "" {
			z.requestBridgeDeviceByFriendly(friendly, "bridge-device-missing-exposes")
		}
		return dev, false
	}
	var (
		capabilities []model.Capability
		inputs       []model.DeviceInput
		refreshProps []string
		capMap       map[string]model.Capability
	)
	if exposuresFound {
		capabilities, inputs, refreshProps, capMap = buildCapabilitiesFromExposes(exposes)
		if len(capabilities) == 0 {
			// Treat empty capability sets as "metadata not ready" (often happens when Z2M is restarting).
			// For existing devices: keep the last known capabilities/inputs instead of wiping them.
			// For new devices: persist/publish a minimal device and request a full device payload.
			if isNew {
				dev.Online = true
				dev.LastSeen = time.Now().UTC()
				if err := z.repo.UpsertDevice(ctx, dev); err != nil {
					slog.Error("zigbee device upsert failed", "device", friendly, "err", err)
				}
				z.publishHDPMeta(dev)
				z.requestBridgeDeviceByFriendly(origFriendly, "bridge-device-empty-capabilities")
				if strings.TrimSpace(origFriendly) == "" {
					z.requestBridgeDeviceByFriendly(friendly, "bridge-device-empty-capabilities")
				}
				return dev, false
			}
		} else {
			if b, err := json.Marshal(capabilities); err == nil {
				dev.Capabilities = datatypes.JSON(b)
			} else {
				slog.Warn("zigbee capabilities marshal", "device", friendly, "err", err)
			}
			if len(inputs) > 0 {
				if b, err := json.Marshal(inputs); err == nil {
					dev.Inputs = datatypes.JSON(b)
				} else {
					slog.Warn("zigbee inputs marshal", "device", friendly, "err", err)
				}
			} else {
				dev.Inputs = nil
			}
		}
	}

	dev.Online = true
	dev.LastSeen = time.Now().UTC()
	if err := z.repo.UpsertDevice(ctx, dev); err != nil {
		slog.Error("zigbee device upsert failed", "device", friendly, "err", err)
	}
	if removed, err := z.repo.DeleteDuplicatesByExternal(ctx, "zigbee", dev.ExternalID, dev.ID.String()); err != nil {
		slog.Warn("zigbee duplicate prune failed", "device", friendly, "error", err)
	} else {
		z.cleanupRemovedDevices(ctx, removed, "duplicate-external-id")
	}

	if exposuresFound {
		z.metaMu.Lock()
		if len(capMap) > 0 {
			if friendly != "" {
				z.capIndex[friendly] = capMap
			}
			z.capIndex[dev.ExternalID] = capMap
		} else {
			if friendly != "" {
				delete(z.capIndex, friendly)
			}
			delete(z.capIndex, dev.ExternalID)
		}
		if len(refreshProps) > 0 {
			if friendly != "" {
				z.refreshProps[friendly] = refreshProps
			}
			z.refreshProps[dev.ExternalID] = refreshProps
		} else {
			if friendly != "" {
				delete(z.refreshProps, friendly)
			}
			delete(z.refreshProps, dev.ExternalID)
		}
		z.metaMu.Unlock()
		slog.Info("zigbee device metadata synced", "device", friendly, "caps", len(capabilities), "inputs", len(inputs), "source", source)
	}
	z.publishHDPMeta(dev)
	return dev, exposuresFound
}
