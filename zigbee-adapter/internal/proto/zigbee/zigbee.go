package zigbee

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/hdp"
	paho "github.com/eclipse/paho.mqtt.golang"

	model "github.com/PetoAdam/homenavi/zigbee-adapter/internal/devices"
	dbinfra "github.com/PetoAdam/homenavi/zigbee-adapter/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/zigbee-adapter/internal/infra/mqtt"
	redisinfra "github.com/PetoAdam/homenavi/zigbee-adapter/internal/infra/redis"
)

type ZigbeeAdapter struct {
	client    *mqttinfra.Client
	repo      *dbinfra.Repository
	cache     *redisinfra.StateCache
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
	pendingMu      sync.Mutex
	pendingState   map[string]pendingZigbeeState
	devicesReqMu   sync.Mutex
	lastDevicesReq time.Time
	correlationMu  sync.Mutex
	correlationMap map[string]string
}

type pendingZigbeeState struct {
	payload    []byte
	receivedAt time.Time
}

const (
	hdpSchema               = hdp.SchemaV1
	hdpMetadataPrefix       = hdp.MetadataPrefix
	hdpStatePrefix          = hdp.StatePrefix
	hdpEventPrefix          = hdp.EventPrefix
	hdpCommandPrefix        = hdp.CommandPrefix
	hdpCommandResultPrefix  = hdp.CommandResultPrefix
	hdpPairingCommandTopic  = hdp.PairingCommandPrefix + "zigbee"
	hdpPairingProgressTopic = hdp.PairingProgressPrefix + "zigbee"
	hdpAdapterHelloTopic    = hdp.AdapterHelloTopic
	hdpAdapterStatusPrefix  = hdp.AdapterStatusPrefix
)

var deviceStateTopic = regexp.MustCompile(`^zigbee2mqtt/([^/]+)$`)

var zigbeeIEEEExternal = regexp.MustCompile(`^0x[0-9a-f]{16}$`)

func New(client *mqttinfra.Client, repo *dbinfra.Repository, cache *redisinfra.StateCache) *ZigbeeAdapter {
	refresh := envx.Bool("ZIGBEE_ADAPTER_REFRESH_STATES", envx.Bool("DEVICE_HUB_REFRESH_STATES", true))
	adapterID := envx.String("ZIGBEE_ADAPTER_ID", envx.String("DEVICE_HUB_ZIGBEE_ADAPTER_ID", "zigbee"))
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
		pendingState:   map[string]pendingZigbeeState{},
		correlationMap: map[string]string{},
	}
}

func (z *ZigbeeAdapter) requestBridgeDeviceByFriendly(friendly, reason string) {
	friendly = strings.TrimSpace(friendly)
	if friendly == "" {
		z.requestBridgeDevicesThrottled(reason)
		return
	}
	payload, _ := json.Marshal(map[string]string{"id": friendly})
	_ = z.client.Publish("zigbee2mqtt/bridge/request/device", payload)
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
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-z.ctx.Done():
				return
			case <-ticker.C:
				_ = z.publishStatus("online", "heartbeat")
			}
		}
	}()
	slog.Info("zigbee adapter started", "adapter_id", z.adapterID)
	return nil
}

func (z *ZigbeeAdapter) requestBridgeDevicesThrottled(reason string) {
	if z == nil || z.client == nil {
		return
	}
	z.devicesReqMu.Lock()
	defer z.devicesReqMu.Unlock()
	if !z.lastDevicesReq.IsZero() && time.Since(z.lastDevicesReq) < 10*time.Second {
		return
	}
	z.lastDevicesReq = time.Now().UTC()
	if err := z.client.Publish("zigbee2mqtt/bridge/request/devices", []byte(`{}`)); err != nil {
		slog.Debug("zigbee bridge devices request publish failed", "reason", reason, "err", err)
	}
}

func isCanonicalZigbeeExternal(external string) bool {
	ext := strings.ToLower(strings.TrimSpace(external))
	return zigbeeIEEEExternal.MatchString(ext)
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

func (z *ZigbeeAdapter) subscribe(topic string, handler mqttinfra.Handler) error {
	if err := z.client.Subscribe(topic, handler); err != nil {
		return err
	}
	z.subscriptions = append(z.subscriptions, topic)
	return nil
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
		// State-first discovery: state topics are commonly keyed by friendly name.
		// Cache the payload and request bridge metadata so we can resolve the IEEE.
		slog.Debug("zigbee state missing canonical identity", "friendly", friendly)
		z.stashPendingState(friendly, m.Payload())
		z.requestBridgeDeviceByFriendly(friendly, "state-missing-identity")
		return
	}
	if !isCanonicalZigbeeExternal(canonical) {
		// Guardrail: never ingest/publish Zigbee devices using non-canonical IDs.
		// This prevents protocol collisions like zigbee/<hash> when some upstream
		// system mislabels an ID or bridge mappings are incomplete.
		slog.Warn("zigbee state has non-canonical external id", "friendly", friendly, "external", canonical)
		z.requestBridgeDevicesThrottled("state-non-canonical-identity")
		return
	}
	if friendly != "" && canonical != "" {
		z.setFriendlyMapping(friendly, canonical)
		z.reconcileFriendlyDevice(context.Background(), friendly, canonical)
	}

	ctx := context.Background()
	z.ingestState(ctx, friendly, canonical, raw, false)
}

func adapterVersion() string {
	return envx.String("ZIGBEE_ADAPTER_VERSION", envx.String("DEVICE_HUB_ZIGBEE_VERSION", "dev"))
}
