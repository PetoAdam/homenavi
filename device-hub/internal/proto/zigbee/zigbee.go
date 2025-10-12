package zigbee

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"device-hub/internal/model"
	"device-hub/internal/mqtt"
	"device-hub/internal/store"
)

type ZigbeeAdapter struct {
	client *mqtt.Client
	repo   *store.Repository
	cache  *store.StateCache
	events chan<- any

	pairingMu     sync.Mutex
	pairingActive bool
	pairingCancel context.CancelFunc

	refreshOnStart bool
	metaMu         sync.RWMutex
	refreshProps   map[string][]string
	capIndex       map[string]map[string]model.Capability
	friendlyIndex  map[string]string
	friendlyTopic  map[string]string
}

const (
	stateTopicPrefix   = "homenavi/devicehub/devices/"
	deviceRemovedTopic = "homenavi/devicehub/events/device.removed"
)

var deviceStateTopic = regexp.MustCompile(`^zigbee2mqtt/([^/]+)$`)

func New(client *mqtt.Client, repo *store.Repository, cache *store.StateCache, events chan<- any) *ZigbeeAdapter {
	refresh := true
	if v := strings.ToLower(os.Getenv("DEVICE_HUB_REFRESH_STATES")); v == "0" || v == "false" || v == "no" {
		refresh = false
	}
	return &ZigbeeAdapter{
		client:         client,
		repo:           repo,
		cache:          cache,
		events:         events,
		refreshOnStart: refresh,
		refreshProps:   map[string][]string{},
		capIndex:       map[string]map[string]model.Capability{},
		friendlyIndex:  map[string]string{},
		friendlyTopic:  map[string]string{},
	}
}

func (z *ZigbeeAdapter) Name() string { return "zigbee" }

func (z *ZigbeeAdapter) Start(ctx context.Context) error {
	if err := z.client.Subscribe("zigbee2mqtt/#", z.handle); err != nil {
		return err
	}
	if err := z.client.Subscribe("homenavi/devicehub/commands/pairing", z.handlePairingCommand); err != nil {
		return err
	}
	if err := z.client.Subscribe("homenavi/devicehub/commands/device.set", z.handleDeviceSetCommand); err != nil {
		return err
	}
	if err := z.client.Subscribe("homenavi/devicehub/commands/device.rename", z.handleDeviceRenameCommand); err != nil {
		return err
	}
	if err := z.client.Subscribe("homenavi/devicehub/test/inject/zigbee/#", z.handleTestInject); err != nil {
		return err
	}
	slog.Info("zigbee adapter subscribed", "patterns", []string{"zigbee2mqtt/#", "pairing"})

	go z.primeFromDB(context.Background())
	_ = z.client.Publish("zigbee2mqtt/bridge/request/devices", []byte(`{}`))
	return nil
}

func (z *ZigbeeAdapter) PublishCommand(ctx context.Context, device *model.Device, cmd map[string]any) error {
	payload := map[string]any{}
	for k, v := range cmd {
		payload[k] = v
	}
	b, _ := json.Marshal(payload)
	deviceTopic := strings.TrimSpace(device.ExternalID)
	z.metaMu.RLock()
	if norm := normalizeExternalKey(deviceTopic); norm != "" {
		if friendly, ok := z.friendlyTopic[norm]; ok && friendly != "" {
			deviceTopic = friendly
		}
	}
	z.metaMu.RUnlock()
	if deviceTopic == "" {
		if device.Name != "" {
			deviceTopic = strings.TrimSpace(device.Name)
		}
	}
	if deviceTopic == "" {
		deviceTopic = device.ID.String()
	}
	topic := "zigbee2mqtt/" + deviceTopic + "/set"
	return z.client.Publish(topic, b)
}

func (z *ZigbeeAdapter) handle(_ paho.Client, m paho.Message) {
	topic := m.Topic()
	if topic == "zigbee2mqtt/bridge/devices" {
		z.handleBridgeDevices(m)
		return
	}
	if topic == "zigbee2mqtt/bridge/response/device" {
		z.handleBridgeDeviceResponse(m)
		return
	}
	if strings.HasPrefix(topic, "zigbee2mqtt/bridge/event") {
		z.handleBridgeEvent(nil, m)
		return
	}

	matches := deviceStateTopic.FindStringSubmatch(topic)
	if len(matches) != 2 {
		return
	}
	friendly := matches[1]

	var raw map[string]any
	if err := json.Unmarshal(m.Payload(), &raw); err != nil {
		slog.Warn("zigbee payload unmarshal failed", "topic", topic, "error", err)
		return
	}

	canonical := canonicalExternalID(raw)
	if canonical == "" {
		canonical = z.resolveExternalID(friendly)
	}
	if canonical == "" {
		canonical = friendly
	}
	if friendly != "" && canonical != "" {
		z.setFriendlyMapping(friendly, canonical)
	}

	ctx := context.Background()
	dev, _ := z.repo.GetByExternal(ctx, "zigbee", canonical)
	if dev == nil && !strings.EqualFold(canonical, friendly) {
		dev, _ = z.repo.GetByExternal(ctx, "zigbee", friendly)
	}
	if dev == nil {
		name := friendly
		if name == "" {
			name = canonical
		}
		dev = &model.Device{ID: uuid.New(), Protocol: "zigbee", ExternalID: canonical, Name: name, Online: true, LastSeen: time.Now().UTC()}
	} else {
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
	_ = z.client.PublishWith(stateTopicPrefix+dev.ID.String(), sb, true)
	devJSON, _ := json.Marshal(dev)
	_ = z.client.Publish("homenavi/devicehub/events/device.upsert", devJSON)

	if len(state) == 0 {
		slog.Warn("zigbee state empty", "device", dev.ExternalID)
	} else {
		slog.Info("zigbee state", "device", dev.ExternalID, "keys", len(state), "changes", len(changes))
		for k, diff := range changes {
			slog.Debug("zigbee state change", "device", dev.ExternalID, "key", k, "old", diff[0], "new", diff[1])
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
	case "device_joined", "device_announce", "device_interview":
		friendly, _ := evt.Data["friendly_name"].(string)
		if friendly == "" {
			return
		}
		external := canonicalExternalID(evt.Data)
		if external == "" {
			external = z.resolveExternalID(friendly)
		}
		if external == "" {
			external = friendly
		}
		z.setFriendlyMapping(friendly, external)
		dev, _ := z.repo.GetByExternal(ctx, "zigbee", external)
		if dev == nil && !strings.EqualFold(external, friendly) {
			dev, _ = z.repo.GetByExternal(ctx, "zigbee", friendly)
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
		_ = z.repo.UpsertDevice(ctx, dev)
		b, _ := json.Marshal(dev)
		_ = z.client.Publish("homenavi/devicehub/events/device.upsert", b)
	case "device_removed":
		friendly, _ := evt.Data["friendly_name"].(string)
		external := z.resolveExternalID(friendly)
		if external == "" {
			external = friendly
		}
		dev, _ := z.repo.GetByExternal(ctx, "zigbee", external)
		if dev != nil {
			z.removeDevice(ctx, dev, "zigbee-event")
		} else {
			slog.Warn("zigbee device_removed missing record", "friendly", friendly, "external", external)
		}
	case "device_renamed":
		from := stringField(evt.Data, "from")
		to := stringField(evt.Data, "to")
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
		devJSON, _ := json.Marshal(dev)
		_ = z.client.Publish("homenavi/devicehub/events/device.upsert", devJSON)
		if b, err := json.Marshal(map[string]string{"id": to}); err == nil {
			_ = z.client.Publish("zigbee2mqtt/bridge/request/device", b)
		}
		slog.Info("zigbee device renamed", "from", from, "to", to)
	}
}

func (z *ZigbeeAdapter) handlePairingCommand(_ paho.Client, m paho.Message) {
	var cmd struct {
		Action  string `json:"action"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(m.Payload(), &cmd); err != nil {
		return
	}
	switch cmd.Action {
	case "start":
		z.pairingMu.Lock()
		if z.pairingActive {
			z.pairingMu.Unlock()
			return
		}
		if cmd.Timeout <= 0 {
			cmd.Timeout = 60
		}
		ctx2, cancel := context.WithCancel(context.Background())
		z.pairingActive = true
		z.pairingCancel = cancel
		z.pairingMu.Unlock()
		_ = z.client.Publish("zigbee2mqtt/bridge/request/permit_join", []byte(fmt.Sprintf(`{"value":true,"time":%d}`, cmd.Timeout)))
		go func(timeout int, c context.Context) {
			select {
			case <-time.After(time.Duration(timeout) * time.Second):
				z.pairingMu.Lock()
				active := z.pairingActive
				z.pairingMu.Unlock()
				if active {
					z.stopPairing()
					_ = z.client.Publish("homenavi/devicehub/events/pairing.timeout", []byte(`{}`))
				}
			case <-c.Done():
			}
		}(cmd.Timeout, ctx2)
	case "stop":
		z.pairingMu.Lock()
		active := z.pairingActive
		z.pairingMu.Unlock()
		if !active {
			return
		}
		z.stopPairing()
	}
}

func (z *ZigbeeAdapter) stopPairing() {
	z.pairingMu.Lock()
	cancel := z.pairingCancel
	z.pairingCancel = nil
	z.pairingActive = false
	z.pairingMu.Unlock()
	if cancel != nil {
		cancel()
	}
	_ = z.client.Publish("zigbee2mqtt/bridge/request/permit_join", []byte(`{"value":false}`))
}

func (z *ZigbeeAdapter) handleTestInject(_ paho.Client, m paho.Message) {
	parts := strings.Split(m.Topic(), "/")
	if len(parts) < 6 {
		return
	}
	friendly := parts[len(parts)-1]
	_ = z.client.Publish("zigbee2mqtt/"+friendly, m.Payload())
}

func (z *ZigbeeAdapter) handleDeviceSetCommand(_ paho.Client, m paho.Message) {
	var req struct {
		DeviceID string         `json:"device_id"`
		State    map[string]any `json:"state"`
	}
	if err := json.Unmarshal(m.Payload(), &req); err != nil {
		return
	}
	if req.DeviceID == "" || len(req.State) == 0 {
		return
	}
	ctx := context.Background()
	dev, _ := z.repo.GetByID(ctx, req.DeviceID)
	if dev == nil {
		return
	}
	payload := map[string]any{}
	for k, v := range req.State {
		switch k {
		case "on":
			if vb, ok := v.(bool); ok {
				if vb {
					payload["state"] = "ON"
				} else {
					payload["state"] = "OFF"
				}
			}
		default:
			payload[k] = v
		}
	}
	b, _ := json.Marshal(payload)
	_ = z.client.Publish("zigbee2mqtt/"+dev.ExternalID+"/set", b)
}

func (z *ZigbeeAdapter) handleDeviceRenameCommand(_ paho.Client, m paho.Message) {
	var req struct {
		DeviceID string `json:"device_id"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(m.Payload(), &req); err != nil {
		return
	}
	name := strings.TrimSpace(req.Name)
	if req.DeviceID == "" || name == "" {
		return
	}
	ctx := context.Background()
	dev, _ := z.repo.GetByID(ctx, req.DeviceID)
	if dev == nil {
		return
	}
	dev.Name = name
	if err := z.repo.UpsertDevice(ctx, dev); err != nil {
		slog.Error("zigbee rename update failed", "device_id", req.DeviceID, "error", err)
		return
	}
	devJSON, err := json.Marshal(dev)
	if err != nil {
		slog.Error("zigbee rename encode failed", "device_id", req.DeviceID, "error", err)
		return
	}
	if err := z.client.Publish("homenavi/devicehub/events/device.upsert", devJSON); err != nil {
		slog.Warn("zigbee rename event publish failed", "device_id", dev.ID.String(), "error", err)
	}
	slog.Info("device name updated", "device", dev.ExternalID, "name", name)
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
		if strings.EqualFold(stringField(d, "type"), "coordinator") {
			continue
		}
		if supported, ok := d["supported"].(bool); ok && !supported {
			continue
		}
		friendly := stringField(d, "friendly_name")
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
	if z.refreshOnStart && discovered > 0 {
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
	_, _ = z.upsertBridgeDevice(ctx, resp.Data, "device-response")
}

func (z *ZigbeeAdapter) upsertBridgeDevice(ctx context.Context, raw map[string]any, source string) (*model.Device, bool) {
	origFriendly := strings.TrimSpace(stringField(raw, "friendly_name"))
	friendly := origFriendly
	if friendly == "" {
		friendly = stringField(raw, "id")
	}
	external := canonicalExternalID(raw)
	if external == "" {
		if friendly == "" {
			slog.Debug("zigbee bridge device missing canonical identifier", "source", source)
			return nil, false
		}
		external = strings.ToLower(strings.TrimSpace(friendly))
	}
	if friendly != "" {
		z.setFriendlyMapping(friendly, external)
	}
	dev, _ := z.repo.GetByExternal(ctx, "zigbee", external)
	if dev == nil && friendly != "" && !strings.EqualFold(external, friendly) {
		dev, _ = z.repo.GetByExternal(ctx, "zigbee", friendly)
	}
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
	if typ := stringField(raw, "type"); typ != "" {
		dev.Type = typ
	}
	if mf := stringField(raw, "manufacturer"); mf != "" {
		dev.Manufacturer = mf
	}
	if mo := stringField(raw, "model"); mo != "" {
		dev.Model = mo
	}
	if fw := stringField(raw, "software_build_id"); fw != "" {
		dev.Firmware = fw
	} else if fw := stringField(raw, "date_code"); fw != "" {
		dev.Firmware = fw
	}
	if desc := stringField(raw, "description"); desc != "" {
		dev.Description = desc
	}

	if def, ok := raw["definition"].(map[string]any); ok {
		if dev.Type == "" {
			if defType := stringField(def, "type"); defType != "" {
				dev.Type = defType
			}
		}
		if dev.Manufacturer == "" {
			if vendor := stringField(def, "vendor"); vendor != "" {
				dev.Manufacturer = vendor
			}
		}
		if dev.Model == "" {
			if model := stringField(def, "model"); model != "" {
				dev.Model = model
			}
		}
		if dev.Description == "" {
			if desc := stringField(def, "description"); desc != "" {
				dev.Description = desc
			}
		}
	}

	exposes, exposuresFound := extractExposes(raw)
	var (
		capabilities []model.Capability
		inputs       []model.DeviceInput
		refreshProps []string
		capMap       map[string]model.Capability
	)
	if exposuresFound {
		capabilities, inputs, refreshProps, capMap = buildCapabilitiesFromExposes(exposes)
		if len(capabilities) > 0 {
			if b, err := json.Marshal(capabilities); err == nil {
				dev.Capabilities = datatypes.JSON(b)
			} else {
				slog.Warn("zigbee capabilities marshal", "device", friendly, "err", err)
			}
		} else {
			dev.Capabilities = nil
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

	dev.Online = true
	dev.LastSeen = time.Now().UTC()
	if err := z.repo.UpsertDevice(ctx, dev); err != nil {
		slog.Error("zigbee device upsert failed", "device", friendly, "err", err)
	}
	devJSON, _ := json.Marshal(dev)
	_ = z.client.Publish("homenavi/devicehub/events/device.upsert", devJSON)
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

	return dev, exposuresFound
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
		if err := z.client.PublishWith(stateTopicPrefix+dev.ID.String(), []byte{}, true); err != nil {
			slog.Warn("zigbee state cleanup publish failed", "device", dev.ExternalID, "error", err)
		}
		payload := map[string]any{
			"id":          dev.ID.String(),
			"device_id":   dev.ID.String(),
			"external_id": dev.ExternalID,
			"protocol":    dev.Protocol,
			"reason":      reason,
		}
		if data, err := json.Marshal(payload); err == nil {
			if err := z.client.Publish(deviceRemovedTopic, data); err != nil {
				slog.Warn("zigbee removal event publish failed", "device", dev.ExternalID, "error", err)
			}
		} else {
			slog.Warn("zigbee removal payload encode failed", "device", dev.ExternalID, "error", err)
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

func extractExposes(raw map[string]any) ([]any, bool) {
	if raw == nil {
		return nil, false
	}
	if def, ok := raw["definition"].(map[string]any); ok {
		if exposes, ok := def["exposes"].([]any); ok {
			return exposes, true
		}
		if exposes, ok := def["exposes"].([]interface{}); ok {
			return exposes, true
		}
	}
	if exposes, ok := raw["exposes"].([]any); ok {
		return exposes, true
	}
	if exposes, ok := raw["exposes"].([]interface{}); ok {
		return exposes, true
	}
	return nil, false
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
	for friendly, props := range copyMap {
		for _, p := range props {
			if p == "" {
				continue
			}
			payload := map[string]any{p: ""}
			b, _ := json.Marshal(payload)
			topic := "zigbee2mqtt/" + friendly + "/get"
			if err := z.client.Publish(topic, b); err != nil {
				slog.Debug("zigbee get publish failed", "device", friendly, "prop", p, "err", err)
			} else {
				slog.Debug("zigbee get published", "device", friendly, "prop", p)
			}
			time.Sleep(150 * time.Millisecond)
		}
	}
}

func (z *ZigbeeAdapter) primeFromDB(ctx context.Context) {
	devices, err := z.repo.List(ctx)
	if err != nil {
		return
	}
	missingCaps := []string{}
	for _, d := range devices {
		stateJSON, err := z.repo.GetDeviceState(ctx, d.ID.String())
		if err != nil {
			continue
		}
		if len(stateJSON) == 0 {
			stateJSON = []byte(`{}`)
		}
		_ = z.cache.Set(ctx, d.ID.String(), stateJSON)
		_ = z.client.PublishWith(stateTopicPrefix+d.ID.String(), stateJSON, true)
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
						z.refreshProps[d.ExternalID] = uniqueStrings(refresh)
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
	z.clearLegacyByExternal(ctx)
	if len(missingCaps) > 0 {
		unique := uniqueStrings(missingCaps)
		slog.Info("zigbee requesting capability backfill", "devices", unique)
		for _, friendly := range unique {
			payload, _ := json.Marshal(map[string]string{"id": friendly})
			if err := z.client.Publish("zigbee2mqtt/bridge/request/device", payload); err != nil {
				slog.Debug("zigbee capability backfill publish failed", "device", friendly, "err", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (z *ZigbeeAdapter) clearLegacyByExternal(ctx context.Context) {
	const topic = "homenavi/devicehub/devices/by-external/#"
	const prefix = "homenavi/devicehub/devices/by-external/"
	legacy := map[string]struct{}{}
	var mu sync.Mutex
	h := func(_ paho.Client, msg mqtt.Message) {
		if !msg.Retained() {
			return
		}
		if len(msg.Payload()) == 0 {
			return
		}
		topicName := msg.Topic()
		if !strings.HasPrefix(topicName, prefix) {
			return
		}
		mu.Lock()
		legacy[topicName] = struct{}{}
		mu.Unlock()
	}
	if err := z.client.Subscribe(topic, h); err != nil {
		slog.Error("legacy metadata cleanup subscribe failed", "error", err)
		return
	}
	waitCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	<-waitCtx.Done()
	if err := z.client.Unsubscribe(topic); err != nil {
		slog.Warn("legacy metadata cleanup unsubscribe failed", "topic", topic, "error", err)
	}
	if len(legacy) == 0 {
		return
	}
	slog.Info("clearing legacy device metadata topics", "count", len(legacy))
	for orphan := range legacy {
		if err := z.client.PublishWith(orphan, []byte{}, true); err != nil {
			slog.Warn("legacy metadata cleanup publish failed", "topic", orphan, "error", err)
		}
	}
}

func canonicalExternalID(raw map[string]any) string {
	ieee := strings.TrimSpace(stringField(raw, "ieee_address"))
	if ieee == "" {
		return ""
	}
	ieee = strings.ToLower(ieee)
	if strings.HasPrefix(ieee, "0x") {
		return ieee
	}
	return "0x" + strings.TrimPrefix(ieee, "0x")
}

func (z *ZigbeeAdapter) resolveExternalID(friendly string) string {
	if friendly == "" {
		return ""
	}
	z.metaMu.RLock()
	defer z.metaMu.RUnlock()
	if external, ok := z.friendlyIndex[friendly]; ok {
		return external
	}
	return ""
}

func normalizeExternalKey(external string) string {
	return strings.ToLower(strings.TrimSpace(external))
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
		if norm := normalizeExternalKey(current); norm != "" {
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
	if norm := normalizeExternalKey(external); norm != "" {
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
	if norm := normalizeExternalKey(external); norm != "" {
		delete(z.friendlyTopic, norm)
	}
	z.metaMu.Unlock()
	return removed
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case string:
			return val
		case fmt.Stringer:
			return val.String()
		default:
			return fmt.Sprint(val)
		}
	}
	return ""
}

func normalizeValueForCapability(cap model.Capability, v any) any {
	switch cap.ValueType {
	case "boolean":
		return coerceBool(v, cap.TrueValue, cap.FalseValue)
	case "number":
		if f, ok := numericValue(v); ok {
			if cap.Range != nil && cap.Range.Step > 0 {
				step := cap.Range.Step
				f = math.Round(f/step) * step
			}
			return f
		}
	case "enum", "string":
		return fmt.Sprint(v)
	default:
		return v
	}
	return v
}

func normalizeLooseValue(prop string, v any) any {
	switch strings.ToLower(prop) {
	case "state", "on":
		return coerceBool(v, "", "")
	}
	if f, ok := numericValue(v); ok {
		return f
	}
	return v
}

func coerceBool(v any, trueVal, falseVal string) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		s := strings.TrimSpace(strings.ToLower(val))
		if trueVal != "" && strings.EqualFold(val, trueVal) {
			return true
		}
		if falseVal != "" && strings.EqualFold(val, falseVal) {
			return false
		}
		if s == "on" || s == "true" || s == "1" || s == "yes" {
			return true
		}
		if s == "off" || s == "false" || s == "0" || s == "no" {
			return false
		}
	case float64:
		return val != 0
	case float32:
		return val != 0
	case int:
		return val != 0
	case int64:
		return val != 0
	case uint64:
		return val != 0
	}
	return false
}

func numericValue(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func buildCapabilitiesFromExposes(exposes []any) ([]model.Capability, []model.DeviceInput, []string, map[string]model.Capability) {
	caps := []model.Capability{}
	inputs := []model.DeviceInput{}
	refresh := []string{}
	capMap := map[string]model.Capability{}
	for _, raw := range exposes {
		extractCapability(raw, "", &caps, &inputs, &refresh, capMap)
	}
	if len(refresh) > 0 {
		refresh = uniqueStrings(refresh)
	}
	return caps, inputs, refresh, capMap
}

func extractCapability(raw any, parentKind string, caps *[]model.Capability, inputs *[]model.DeviceInput, refresh *[]string, capMap map[string]model.Capability) {
	m, ok := raw.(map[string]any)
	if !ok {
		return
	}

	kind := parentKind
	if t := stringField(m, "type"); t != "" {
		kind = strings.ToLower(t)
	}
	property := strings.ToLower(stringField(m, "property"))
	name := stringField(m, "name")
	description := stringField(m, "description")
	unit := stringField(m, "unit")
	access := parseAccess(m["access"])
	enumValues := stringSliceFromAny(m["values"])
	rng := parseRange(m)
	trueVal := stringField(m, "value_on")
	falseVal := stringField(m, "value_off")

	features, hasChildren := m["features"].([]any)

	includeSelf := !hasChildren || property != ""
	var capID string
	if includeSelf {
		capID = makeCapabilityID(property, name, len(*caps))
		cap := model.Capability{
			ID:          capID,
			Name:        humanizeName(name, property, kind, capID),
			Kind:        kind,
			Property:    property,
			ValueType:   inferValueType(kind, enumValues, rng, property, m),
			Unit:        unit,
			Access:      access,
			Description: description,
		}
		if parentKind != "" && parentKind != kind {
			cap.SubType = parentKind
		}
		if rng != nil {
			cap.Range = rng
		}
		if len(enumValues) > 0 {
			cap.Enum = enumValues
		}
		if trueVal != "" {
			cap.TrueValue = trueVal
		}
		if falseVal != "" {
			cap.FalseValue = falseVal
		}
		if cap.ValueType == "boolean" {
			if cap.TrueValue == "" {
				cap.TrueValue = "ON"
			}
			if cap.FalseValue == "" {
				cap.FalseValue = "OFF"
			}
		}
		*caps = append(*caps, cap)
		if property != "" {
			capMap[property] = cap
			if access.Read {
				*refresh = append(*refresh, property)
			}
		}
		if access.Write {
			input := buildInputForCapability(cap, enumValues, trueVal, falseVal, m)
			*inputs = append(*inputs, input)
		}
	}

	if hasChildren {
		for _, child := range features {
			extractCapability(child, kind, caps, inputs, refresh, capMap)
		}
	}
}

func parseAccess(v any) model.CapabilityAccess {
	access := 1
	switch val := v.(type) {
	case float64:
		access = int(val)
	case int:
		access = val
	case int64:
		access = int(val)
	case json.Number:
		if i, err := val.Int64(); err == nil {
			access = int(i)
		}
	}
	return model.CapabilityAccess{
		Read:  access&1 != 0,
		Write: access&2 != 0,
		Event: access&4 != 0,
	}
}

func parseRange(m map[string]any) *model.CapabilityRange {
	min, minOK := floatFromAny(m["value_min"])
	max, maxOK := floatFromAny(m["value_max"])
	if !minOK && !maxOK {
		return nil
	}
	rng := &model.CapabilityRange{}
	if minOK {
		rng.Min = min
	}
	if maxOK {
		rng.Max = max
	}
	if step, ok := floatFromAny(m["value_step"]); ok {
		rng.Step = step
	}
	return rng
}

func inferValueType(kind string, enumValues []string, rng *model.CapabilityRange, property string, raw map[string]any) string {
	lowerKind := strings.ToLower(kind)
	switch lowerKind {
	case "binary", "switch":
		if property == "state" || property == "contact" || property == "occupancy" {
			return "boolean"
		}
	case "light":
		if property == "state" {
			return "boolean"
		}
		if property == "brightness" || property == "color_temp" {
			return "number"
		}
	case "numeric":
		return "number"
	case "enum":
		return "enum"
	case "composite":
		if property == "color" {
			return "object"
		}
	}
	if len(enumValues) > 0 {
		return "enum"
	}
	if rng != nil {
		return "number"
	}
	if property == "linkquality" || strings.Contains(property, "battery") {
		return "number"
	}
	if _, ok := raw["features"]; ok {
		return "object"
	}
	return "string"
}

func makeCapabilityID(property, name string, idx int) string {
	if property != "" {
		return property
	}
	if name != "" {
		return slugify(name)
	}
	return fmt.Sprintf("cap_%d", idx)
}

func slugify(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '/':
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return v
	}
	return out
}

func humanizeName(name, property, kind, fallback string) string {
	if name != "" {
		return name
	}
	if property != "" {
		return titleCase(strings.ReplaceAll(property, "_", " "))
	}
	if kind != "" {
		return titleCase(kind)
	}
	return fallback
}

func buildInputForCapability(cap model.Capability, enumValues []string, trueVal, falseVal string, raw map[string]any) model.DeviceInput {
	input := model.DeviceInput{
		ID:           cap.ID,
		Label:        cap.Name,
		Type:         determineInputType(cap, enumValues),
		CapabilityID: cap.ID,
		Property:     cap.Property,
	}
	if cap.Range != nil {
		input.Range = cap.Range
	}
	switch input.Type {
	case "toggle":
		if input.Metadata == nil {
			input.Metadata = map[string]any{}
		}
		if trueVal == "" {
			trueVal = "ON"
		}
		if falseVal == "" {
			falseVal = "OFF"
		}
		input.Metadata["true_value"] = trueVal
		input.Metadata["false_value"] = falseVal
		input.Options = []model.InputOption{{Value: falseVal, Label: "Off"}, {Value: trueVal, Label: "On"}}
	case "select":
		opts := make([]model.InputOption, 0, len(enumValues))
		for _, v := range enumValues {
			opts = append(opts, model.InputOption{Value: v, Label: titleCase(v)})
		}
		input.Options = opts
	case "color":
		if input.Metadata == nil {
			input.Metadata = map[string]any{}
		}
		input.Metadata["mode"] = cap.Kind
	}
	return input
}

func determineInputType(cap model.Capability, enumValues []string) string {
	prop := cap.Property
	switch cap.ValueType {
	case "boolean":
		return "toggle"
	case "number":
		if prop == "color_temp" || cap.Range != nil {
			return "slider"
		}
		return "number"
	case "enum":
		return "select"
	default:
		if strings.Contains(prop, "color") {
			return "color"
		}
		return "custom"
	}
}

func stringSliceFromAny(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func floatFromAny(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	}
	return 0, false
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func titleCase(v string) string {
	if v == "" {
		return ""
	}
	splitFn := func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	}
	parts := strings.FieldsFunc(strings.ToLower(v), splitFn)
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
