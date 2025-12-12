package zigbee

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"zigbee-adapter/internal/model"
	"zigbee-adapter/internal/mqtt"
	"zigbee-adapter/internal/proto/adapterutil"
	"zigbee-adapter/internal/store"
)

type ZigbeeAdapter struct {
	client    *mqtt.Client
	repo      *store.Repository
	cache     *store.StateCache
	adapterID string

	ctx    context.Context
	cancel context.CancelFunc

	subscriptions []string

	pairingMu     sync.Mutex
	pairingActive bool
	pairingCancel context.CancelFunc

	refreshOnStart bool
	metaMu         sync.RWMutex
	refreshProps   map[string][]string
	capIndex       map[string]map[string]model.Capability
	friendlyIndex  map[string]string
	friendlyTopic  map[string]string
	correlationMu  sync.Mutex
	correlationMap map[string]string
}

const (
	hdpSchema               = "hdp.v1"
	hdpMetadataPrefix       = "homenavi/hdp/device/metadata/"
	hdpStatePrefix          = "homenavi/hdp/device/state/"
	hdpEventPrefix          = "homenavi/hdp/device/event/"
	hdpCommandPrefix        = "homenavi/hdp/device/command/"
	hdpCommandResultPrefix  = "homenavi/hdp/device/command_result/"
	hdpPairingCommandTopic  = "homenavi/hdp/pairing/command/zigbee"
	hdpPairingProgressTopic = "homenavi/hdp/pairing/progress/zigbee"
	hdpAdapterHelloTopic    = "homenavi/hdp/adapter/hello"
	hdpAdapterStatusPrefix  = "homenavi/hdp/adapter/status/"
)

var deviceStateTopic = regexp.MustCompile(`^zigbee2mqtt/([^/]+)$`)

func New(client *mqtt.Client, repo *store.Repository, cache *store.StateCache) *ZigbeeAdapter {
	refresh := true
	if v := strings.ToLower(os.Getenv("ZIGBEE_ADAPTER_REFRESH_STATES")); v == "0" || v == "false" || v == "no" {
		refresh = false
	} else if v := strings.ToLower(os.Getenv("DEVICE_HUB_REFRESH_STATES")); v == "0" || v == "false" || v == "no" {
		refresh = false
	}
	adapterID := os.Getenv("ZIGBEE_ADAPTER_ID")
	if strings.TrimSpace(adapterID) == "" {
		adapterID = os.Getenv("DEVICE_HUB_ZIGBEE_ADAPTER_ID")
	}
	if strings.TrimSpace(adapterID) == "" {
		adapterID = "zigbee"
	}
	return &ZigbeeAdapter{
		client:         client,
		repo:           repo,
		cache:          cache,
		adapterID:      adapterID,
		refreshOnStart: refresh,
		refreshProps:   map[string][]string{},
		capIndex:       map[string]map[string]model.Capability{},
		friendlyIndex:  map[string]string{},
		friendlyTopic:  map[string]string{},
		correlationMap: map[string]string{},
	}
}

func (z *ZigbeeAdapter) Name() string { return "zigbee" }

func (z *ZigbeeAdapter) Start(ctx context.Context) error {
	z.ctx, z.cancel = context.WithCancel(ctx)
	slog.Info("zigbee adapter starting", "adapter_id", z.adapterID)
	// Announce adapter presence to hub. Non-retained per spec.
	_ = z.publishHello()
	_ = z.publishStatus("starting", "initializing")

	if err := z.subscribe("zigbee2mqtt/#", z.handle); err != nil {
		return err
	}
	if err := z.subscribe(hdpPairingCommandTopic, z.handlePairingCommand); err != nil {
		return err
	}
	if err := z.subscribe(hdpCommandPrefix+"zigbee/#", z.handleHDPDeviceCommand); err != nil {
		return err
	}
	slog.Info("zigbee adapter subscribed", "patterns", []string{"zigbee2mqtt/#", "hdp commands", "hdp pairing"})

	go z.primeFromDB(context.Background())
	_ = z.client.Publish("zigbee2mqtt/bridge/request/devices", []byte(`{}`))
	_ = z.publishStatus("online", "healthy")
	slog.Info("zigbee adapter started", "adapter_id", z.adapterID)
	return nil
}

func (z *ZigbeeAdapter) Stop() {
	if z.cancel != nil {
		z.cancel()
	}
	// Best-effort unsubscribe to avoid lingering retained subs.
	for _, topic := range z.subscriptions {
		if err := z.client.Unsubscribe(topic); err != nil {
			slog.Debug("unsubscribe failed", "topic", topic, "error", err)
		}
	}
	_ = z.publishStatus("offline", "shutdown")
}

func (z *ZigbeeAdapter) subscribe(topic string, handler mqtt.Handler) error {
	if err := z.client.Subscribe(topic, handler); err != nil {
		return err
	}
	z.subscriptions = append(z.subscriptions, topic)
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
	if norm := adapterutil.NormalizeExternalKey(deviceTopic); norm != "" {
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
			external = friendly
		}
		z.setFriendlyMapping(friendly, external)
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
			external = friendly
		}
		z.setFriendlyMapping(friendly, external)
		status := adapterutil.StringField(evt.Data, "status")
		if dev := z.ensureBridgeDevice(ctx, friendly, external, evt.Data); dev != nil {
			z.publishPairingProgress(interviewStageFromStatus(status), status, external, friendly)
		}
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

func (z *ZigbeeAdapter) handlePairingCommand(_ paho.Client, m paho.Message) {
	var cmd struct {
		Action   string `json:"action"`
		Timeout  int    `json:"timeout"`
		Protocol string `json:"protocol"`
	}
	if err := json.Unmarshal(m.Payload(), &cmd); err != nil {
		slog.Debug("adapter cmd decode failed", "error", err)
		return
	}
	if cmd.Action == "" {
		var env map[string]any
		if err := json.Unmarshal(m.Payload(), &env); err == nil {
			cmd.Action = strings.TrimSpace(adapterutil.StringField(env, "action"))
			cmd.Protocol = adapterutil.StringField(env, "protocol")
			if cmd.Timeout == 0 {
				switch v := env["timeout_sec"].(type) {
				case float64:
					cmd.Timeout = int(v)
				case int:
					cmd.Timeout = v
				}
			}
		}
	}
	if cmd.Protocol == "" && strings.HasPrefix(m.Topic(), hdpPairingCommandTopic) {
		cmd.Protocol = "zigbee"
	}
	if cmd.Protocol != "" && !strings.EqualFold(cmd.Protocol, "zigbee") {
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
					z.publishPairingProgress("timeout", "timeout", "", "")
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

func (z *ZigbeeAdapter) handleDeviceRemoveCommand(_ paho.Client, m paho.Message) {
	var req struct {
		Protocol   string `json:"protocol"`
		DeviceID   string `json:"device_id"`
		ExternalID string `json:"external_id"`
		Friendly   string `json:"friendly_name"`
	}
	if err := json.Unmarshal(m.Payload(), &req); err != nil {
		slog.Debug("device.set decode failed", "error", err)
		return
	}
	if req.Protocol != "" && !strings.EqualFold(req.Protocol, "zigbee") {
		return
	}
	target := strings.TrimSpace(req.Friendly)
	ctx := context.Background()
	var dev *model.Device
	if req.DeviceID != "" {
		var err error
		dev, err = z.repo.GetByID(ctx, req.DeviceID)
		if err != nil {
			slog.Warn("zigbee remove lookup failed", "device_id", req.DeviceID, "error", err)
		}
	}
	if dev == nil && req.ExternalID != "" {
		var err error
		dev, err = z.repo.GetByExternal(ctx, "zigbee", req.ExternalID)
		if err != nil {
			slog.Warn("zigbee remove external lookup failed", "external", req.ExternalID, "error", err)
		}
	}
	if dev != nil {
		if target == "" {
			if friendly := z.resolveFriendlyName(dev.ExternalID); friendly != "" {
				target = friendly
			} else {
				target = dev.ExternalID
			}
		}
	} else if target == "" {
		target = strings.TrimSpace(req.ExternalID)
	}
	if target == "" {
		slog.Warn("zigbee remove command missing target", "device_id", req.DeviceID)
		return
	}
	payload := map[string]any{
		"id":    target,
		"block": false,
		"force": true,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("zigbee remove payload encode failed", "target", target, "error", err)
		return
	}
	if err := z.client.Publish("zigbee2mqtt/bridge/request/device/remove", data); err != nil {
		slog.Warn("zigbee remove publish failed", "target", target, "error", err)
	}
}

func (z *ZigbeeAdapter) ensureBridgeDevice(ctx context.Context, friendly, external string, data map[string]any) *model.Device {
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

func (z *ZigbeeAdapter) forwardStateCommand(dev *model.Device, state map[string]any, correlationID string) {
	if dev == nil || len(state) == 0 {
		return
	}
	if correlationID != "" {
		z.setCorrelation(dev.ID.String(), correlationID)
	}
	payload := map[string]any{}
	for k, v := range state {
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
	if correlationID != "" {
		payload["correlation_id"] = correlationID
	}
	b, _ := json.Marshal(payload)
	_ = z.client.Publish("zigbee2mqtt/"+dev.ExternalID+"/set", b)
}

func (z *ZigbeeAdapter) handleAdapterCommand(_ paho.Client, m paho.Message) {
	var cmd struct {
		Type   string `json:"type"`
		Device struct {
			ID string `json:"id"`
		} `json:"device"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(m.Payload(), &cmd); err != nil {
		return
	}
	if !strings.EqualFold(cmd.Type, "command") {
		return
	}
	deviceRef := strings.TrimSpace(cmd.Device.ID)
	if deviceRef == "" {
		return
	}
	ctx := context.Background()
	var dev *model.Device
	dev, _ = z.repo.GetByID(ctx, deviceRef)
	if dev == nil {
		parts := strings.Split(deviceRef, ":")
		if len(parts) == 2 {
			dev, _ = z.repo.GetByExternal(ctx, parts[0], parts[1])
		}
	}
	if dev == nil {
		return
	}
	state := map[string]any{}
	correlationID := adapterutil.StringField(cmd.Payload, "correlation_id")
	if params, ok := cmd.Payload["params"].(map[string]any); ok {
		state = params
	}
	if len(state) == 0 {
		if st, ok := cmd.Payload["state"].(map[string]any); ok {
			state = st
		}
	}
	z.forwardStateCommand(dev, state, correlationID)
	slog.Info("adapter command forwarded", "device_id", dev.ID.String(), "correlation_id", correlationID, "keys", len(state))
}

func (z *ZigbeeAdapter) publishHello() error {
	hdp := map[string]any{
		"schema":      hdpSchema,
		"type":        "hello",
		"adapter_id":  z.adapterID,
		"protocol":    "zigbee",
		"version":     adapterVersion(),
		"hdp_version": "1.0",
		"features": map[string]any{
			"supports_ack":         true,
			"supports_correlation": true,
			"supports_batch_state": true,
		},
		"ts": time.Now().UnixMilli(),
	}
	if hb, err := json.Marshal(hdp); err == nil {
		_ = z.client.Publish(hdpAdapterHelloTopic, hb)
	}
	return nil
}

func (z *ZigbeeAdapter) publishStatus(status, reason string) error {
	hdp := map[string]any{
		"schema":     hdpSchema,
		"type":       "status",
		"adapter_id": z.adapterID,
		"status":     status,
		"reason":     reason,
		"version":    adapterVersion(),
		"ts":         time.Now().UnixMilli(),
	}
	if hb, err := json.Marshal(hdp); err == nil {
		_ = z.client.PublishWith(hdpAdapterStatusPrefix+z.adapterID, hb, true)
	}
	return nil
}

func adapterVersion() string {
	if v := strings.TrimSpace(os.Getenv("ZIGBEE_ADAPTER_VERSION")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("DEVICE_HUB_ZIGBEE_VERSION")); v != "" {
		return v
	}
	return "dev"
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

func (z *ZigbeeAdapter) publishHDPState(dev *model.Device, state map[string]any, corr string) {
	if dev == nil || len(state) == 0 {
		return
	}
	deviceID := z.hdpDeviceID(dev.ExternalID)
	if deviceID == "" {
		return
	}
	envelope := map[string]any{
		"schema":    hdpSchema,
		"type":      "state",
		"device_id": deviceID,
		"ts":        time.Now().UnixMilli(),
		"state":     state,
	}
	if corr != "" {
		envelope["corr"] = corr
	}
	if b, err := json.Marshal(envelope); err == nil {
		_ = z.client.PublishWith(hdpStatePrefix+deviceID, b, true)
	}
}

func (z *ZigbeeAdapter) publishHDPMeta(dev *model.Device) {
	if dev == nil {
		return
	}
	deviceID := z.hdpDeviceID(dev.ExternalID)
	if deviceID == "" {
		return
	}
	envelope := map[string]any{
		"schema":       hdpSchema,
		"type":         "metadata",
		"device_id":    deviceID,
		"protocol":     "zigbee",
		"name":         dev.Name,
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
	if b, err := json.Marshal(envelope); err == nil {
		_ = z.client.PublishWith(hdpMetadataPrefix+deviceID, b, true)
	}
}

func (z *ZigbeeAdapter) publishHDPEvent(deviceID, event string, data map[string]any) {
	id := z.hdpDeviceID(deviceID)
	if id == "" || strings.TrimSpace(event) == "" {
		return
	}
	envelope := map[string]any{
		"schema":    hdpSchema,
		"type":      "event",
		"device_id": id,
		"event":     event,
		"ts":        time.Now().UnixMilli(),
	}
	if len(data) > 0 {
		envelope["data"] = data
	}
	if b, err := json.Marshal(envelope); err == nil {
		_ = z.client.Publish(hdpEventPrefix+id, b)
	}
}

func (z *ZigbeeAdapter) publishHDPCommandResult(dev *model.Device, corr string, success bool, status, errMsg string) {
	if dev == nil || corr == "" {
		return
	}
	deviceID := z.hdpDeviceID(dev.ExternalID)
	if deviceID == "" {
		return
	}
	envelope := map[string]any{
		"schema":    hdpSchema,
		"type":      "command_result",
		"device_id": deviceID,
		"corr":      corr,
		"success":   success,
		"ts":        time.Now().UnixMilli(),
	}
	if status != "" {
		envelope["status"] = status
	}
	if errMsg != "" {
		envelope["error"] = errMsg
	}
	if b, err := json.Marshal(envelope); err == nil {
		_ = z.client.Publish(hdpCommandResultPrefix+deviceID, b)
	}
}

func (z *ZigbeeAdapter) publishPairingProgress(stage, status, external, friendly string) {
	if stage == "" {
		return
	}
	hdp := map[string]any{
		"schema":   hdpSchema,
		"type":     "pairing_progress",
		"protocol": "zigbee",
		"stage":    stage,
		"status":   status,
		"ts":       time.Now().UnixMilli(),
	}
	if external != "" {
		hdp["external_id"] = external
	}
	if friendly != "" {
		hdp["friendly_name"] = friendly
	}
	if b, err := json.Marshal(hdp); err == nil {
		_ = z.client.Publish(hdpPairingProgressTopic, b)
	}
}

func bridgeLifecycleStage(eventType string) string {
	switch eventType {
	case "device_joined":
		return "device_joined"
	case "device_announce":
		return "device_announced"
	default:
		return ""
	}
}

func interviewStageFromStatus(status string) string {
	switch strings.ToLower(status) {
	case "started", "interview_started":
		return "interview_started"
	case "successful", "success", "completed", "complete":
		return "interview_succeeded"
	case "failed", "failure", "error":
		return "interview_failed"
	default:
		return "interview_started"
	}
}

func (z *ZigbeeAdapter) handleHDPDeviceCommand(_ paho.Client, m paho.Message) {
	var envelope map[string]any
	if err := json.Unmarshal(m.Payload(), &envelope); err != nil {
		slog.Debug("hdp command decode failed", "topic", m.Topic(), "error", err)
		return
	}
	deviceID := adapterutil.StringField(envelope, "device_id")
	if deviceID == "" {
		deviceID = strings.TrimPrefix(m.Topic(), hdpCommandPrefix)
	}
	proto, external := z.externalFromHDP(deviceID)
	if proto != "" && proto != "zigbee" {
		return
	}
	if external == "" {
		external = deviceID
	}
	corr := adapterutil.StringField(envelope, "corr")
	command := strings.ToLower(adapterutil.StringField(envelope, "command"))
	if command == "" {
		command = "set_state"
	}
	args := map[string]any{}
	if payload, ok := envelope["args"].(map[string]any); ok {
		args = payload
	} else if payload, ok := envelope["state"].(map[string]any); ok {
		args = payload
	}
	ctx := context.Background()
	var dev *model.Device
	dev, _ = z.repo.GetByExternal(ctx, "zigbee", external)
	if dev == nil {
		dev, _ = z.repo.GetByID(ctx, external)
	}
	if dev == nil {
		return
	}
refreshProps := func(target, external string, props []string) []string {
		out := adapterutil.UniqueStrings(props)
		if len(out) == 0 {
			z.metaMu.RLock()
			if refresh, ok := z.refreshProps[target]; ok {
				out = append(out, refresh...)
			} else if target != external && external != "" {
				if refresh, ok := z.refreshProps[external]; ok {
					out = append(out, refresh...)
				}
			}
			z.metaMu.RUnlock()
		}
		return adapterutil.UniqueStrings(out)
	}

	switch command {
	case "set_state":
		slog.Info("hdp command", "device_id", dev.ID.String(), "external", dev.ExternalID, "corr", corr, "keys", len(args))
		z.forwardStateCommand(dev, args, corr)
		if corr != "" {
			z.publishHDPCommandResult(dev, corr, true, "queued", "")
		}
	case "remove_device":
		slog.Info("hdp remove device", "device_id", dev.ID.String(), "external", dev.ExternalID, "corr", corr)
		z.removeDevice(ctx, dev, "hdp-command")
		if corr != "" {
			z.publishHDPCommandResult(dev, corr, true, "removed", "")
		}
	case "refresh":
		target := external
		if friendly := z.resolveFriendlyName(external); friendly != "" {
			target = friendly
		}
		refreshMetadata := true
		refreshState := true
		if metaFlag, ok := envelope["metadata"].(bool); ok {
			refreshMetadata = metaFlag
		}
		if stateFlag, ok := envelope["state"].(bool); ok {
			refreshState = stateFlag
		}
		props := refreshProps(target, external, adapterutil.StringSliceFromAny(envelope["properties"]))
		if refreshMetadata {
			data, _ := json.Marshal(map[string]string{"id": target})
			if err := z.client.Publish("zigbee2mqtt/bridge/request/device", data); err != nil {
				slog.Warn("zigbee metadata refresh publish failed", "device", target, "error", err)
			}
		}
		if refreshState {
			z.requestStateSnapshotForDevice(target, props)
		}
		if corr != "" {
			z.publishHDPCommandResult(dev, corr, true, "refreshing", "")
		}
	default:
		return
	}
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
	origFriendly := strings.TrimSpace(adapterutil.StringField(raw, "friendly_name"))
	friendly := origFriendly
	if friendly == "" {
		friendly = adapterutil.StringField(raw, "id")
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
		z.requestStateSnapshotForDevice(friendly, props)
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
	for _, p := range unique {
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
		var state map[string]any
		if err := json.Unmarshal(stateJSON, &state); err == nil && len(state) > 0 {
			z.publishHDPState(&d, state, "")
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
		for _, friendly := range unique {
			payload, _ := json.Marshal(map[string]string{"id": friendly})
			if err := z.client.Publish("zigbee2mqtt/bridge/request/device", payload); err != nil {
				slog.Debug("zigbee capability backfill publish failed", "device", friendly, "err", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
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
	friendly = adapterutil.SanitizeString(strings.TrimSpace(friendly))
	if friendly == "" {
		return
	}
	external = adapterutil.SanitizeString(strings.TrimSpace(external))
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

func (z *ZigbeeAdapter) hdpDeviceID(external string) string {
	ext := strings.Trim(strings.TrimSpace(external), "/")
	if ext == "" {
		return ""
	}
	if strings.HasPrefix(ext, "zigbee/") {
		return ext
	}
	if strings.HasPrefix(ext, z.adapterID+"/") {
		return "zigbee/" + ext
	}
	if strings.Contains(ext, "/") {
		return "zigbee/" + ext
	}
	if strings.TrimSpace(z.adapterID) != "" {
		return fmt.Sprintf("zigbee/%s/%s", z.adapterID, ext)
	}
	return "zigbee/" + ext
}

func (z *ZigbeeAdapter) externalFromHDP(deviceID string) (protocol, external string) {
	id := strings.Trim(strings.TrimSpace(deviceID), "/")
	if id == "" {
		return "", ""
	}
	parts := strings.Split(id, "/")
	if len(parts) == 1 {
		return "zigbee", parts[0]
	}
	protocol = strings.ToLower(parts[0])
	if len(parts) >= 3 {
		external = strings.Join(parts[2:], "/")
	} else {
		external = strings.Join(parts[1:], "/")
	}
	return protocol, external
}
