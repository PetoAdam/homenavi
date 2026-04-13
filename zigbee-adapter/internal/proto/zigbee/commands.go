package zigbee

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	model "github.com/PetoAdam/homenavi/zigbee-adapter/internal/devices"
	"github.com/PetoAdam/homenavi/zigbee-adapter/internal/proto/adapterutil"
)

func (z *ZigbeeAdapter) publishInvalidCommandResult(deviceID, corr, status, errMsg string) {
	deviceID = strings.Trim(strings.TrimSpace(deviceID), "/")
	if deviceID == "" || corr == "" {
		return
	}
	envelope := map[string]any{
		"schema":    hdpSchema,
		"type":      "command_result",
		"origin":    "adapter",
		"device_id": deviceID,
		"corr":      corr,
		"success":   false,
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
	if isCanonicalZigbeeExternal(deviceTopic) {
		if nm := strings.TrimSpace(device.Name); nm != "" && !strings.EqualFold(nm, deviceTopic) {
			deviceTopic = nm
		}
	}
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

func (z *ZigbeeAdapter) forwardStateCommand(dev *model.Device, state map[string]any, correlationID string) bool {
	if dev == nil || len(state) == 0 {
		return false
	}
	if correlationID != "" {
		z.setCorrelation(dev.ID.String(), correlationID)
	}
	var transitionMs float64
	var hasTransitionMs bool
	var hasTransition bool
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
		case "transition":
			hasTransition = true
			payload[k] = v
		case "transition_ms":
			hasTransitionMs = true
			switch n := v.(type) {
			case float64:
				transitionMs = n
			case float32:
				transitionMs = float64(n)
			case int:
				transitionMs = float64(n)
			case int64:
				transitionMs = float64(n)
			case json.Number:
				if f, err := n.Float64(); err == nil {
					transitionMs = f
				}
			default:
				// Ignore invalid values.
			}
		default:
			payload[k] = v
		}
	}
	if !hasTransition && hasTransitionMs && transitionMs > 0 {
		payload["transition"] = transitionMs / 1000.0
	}
	// Do not forward correlation_id to Zigbee2MQTT: it is not a standard writable property and
	// causes noisy "No converter available" errors. We still echo correlation_id back to the
	// UI by attaching it to the *next* HDP state publish (see setCorrelation/consumeCorrelation).
	b, _ := json.Marshal(payload)
	target := z.resolveFriendlyName(dev.ExternalID)
	if target == "" {
		if nm := strings.TrimSpace(dev.Name); nm != "" && !strings.EqualFold(nm, dev.ExternalID) {
			target = nm
		}
	}
	if target == "" {
		slog.Warn("zigbee command missing friendly mapping", "external", dev.ExternalID)
		z.requestBridgeDevicesThrottled("command-missing-mapping")
		return false
	}
	if err := z.client.Publish("zigbee2mqtt/"+target+"/set", b); err != nil {
		slog.Warn("zigbee command publish failed", "external", dev.ExternalID, "target", target, "err", err)
		return false
	}
	slog.Info("zigbee command published", "external", dev.ExternalID, "target", target, "bytes", len(b))
	return true
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
	corr := adapterutil.StringField(envelope, "corr")
	deviceID = strings.Trim(strings.TrimSpace(deviceID), "/")
	proto, external := z.externalFromHDP(deviceID)
	if proto != "zigbee" {
		// Strict: only accept explicit zigbee/<id> command targets.
		if corr != "" {
			z.publishInvalidCommandResult(deviceID, corr, "rejected", "invalid device_id protocol")
		}
		slog.Warn("hdp zigbee command rejected: non-zigbee protocol", "device_id", deviceID, "topic", m.Topic())
		return
	}
	if external == "" {
		if corr != "" {
			z.publishInvalidCommandResult(deviceID, corr, "rejected", "missing external id")
		}
		slog.Warn("hdp zigbee command rejected: missing external", "device_id", deviceID, "topic", m.Topic())
		return
	}
	external = strings.ToLower(strings.TrimSpace(external))
	if !isCanonicalZigbeeExternal(external) {
		// Strict: Zigbee devices must be addressed by canonical IEEE external IDs.
		if corr != "" {
			z.publishInvalidCommandResult(deviceID, corr, "rejected", "non-canonical zigbee external id (expected 0x[0-9a-f]{16})")
		}
		slog.Error("hdp zigbee command rejected: non-canonical external id", "device_id", deviceID, "external", external, "topic", m.Topic())
		return
	}
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
	dev, _ := z.repo.GetByExternal(ctx, "zigbee", external)
	if dev == nil {
		if corr != "" {
			// Keep the topic/ID stable for callers, but do not attempt any fallback routing.
			z.publishInvalidCommandResult("zigbee/"+external, corr, "failed", "zigbee device not found")
		}
		slog.Warn("hdp zigbee command rejected: unknown device", "external", external, "device_id", deviceID, "command", command)
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
		slog.Info("hdp command", "external", dev.ExternalID, "corr", corr, "keys", len(args))
		ok := z.forwardStateCommand(dev, args, corr)
		if corr != "" {
			if ok {
				z.publishHDPCommandResult(dev, corr, true, "queued", "")
			} else {
				z.publishHDPCommandResult(dev, corr, false, "failed", "could not route/publish zigbee command")
			}
		}
	case "remove_device":
		// Removing requires a persisted device row (UUID primary key).
		target := external
		if friendly := z.resolveFriendlyName(external); friendly != "" {
			target = friendly
		} else {
			if nm := strings.TrimSpace(dev.Name); nm != "" && !strings.EqualFold(nm, dev.ExternalID) {
				target = nm
			}
		}
		if err := z.publishZigbee2MQTTRemove(target, true); err != nil {
			slog.Warn("zigbee2mqtt remove request failed", "target", target, "external", external, "error", err)
		}
		slog.Info("hdp remove device", "device_id", dev.ID.String(), "external", dev.ExternalID, "corr", corr)
		z.removeDevice(ctx, dev, "hdp-command")
		if corr != "" {
			z.publishHDPCommandResult(dev, corr, true, "applied", "")
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
			z.publishHDPCommandResult(dev, corr, true, "queued", "")
		}
	default:
		return
	}
}
