package zigbee

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/observability"
	model "github.com/PetoAdam/homenavi/zigbee-adapter/internal/devices"
	"go.opentelemetry.io/otel/codes"
)

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
			"supports_pairing":     true,
			"supports_interview":   true,
		},
		"pairing": map[string]any{
			"schema_version":      "1.0",
			"label":               "Zigbee (Zigbee2MQTT)",
			"supported":           true,
			"supports_interview":  true,
			"default_timeout_sec": 60,
			"instructions": []string{
				"Reset or power-cycle the device to enter pairing mode.",
				"Keep it close to the coordinator while pairing runs.",
				"Devices default to their Zigbee identifier so you can rename them later.",
			},
			"cta_label": "Start Zigbee pairing",
			"flow": map[string]any{
				"id":          "zigbee-default-flow",
				"entry_modes": []string{"default"},
				"forms": []map[string]any{
					{
						"mode":  "default",
						"label": "Default",
						"fields": []map[string]any{
							{"id": "timeout", "component": "number", "label": "Permit join timeout (seconds)", "bind": "timeout", "min": 30, "max": 300, "default": 60},
							{"id": "allow_multiple_devices", "component": "checkbox", "label": "Keep pairing open for multiple devices", "description": "Optional. Use this for lamps or devices you want to add together before stopping pairing.", "default": false},
							{"id": "network_key_permit", "component": "checkbox", "label": "Allow network key exchange", "default": true},
							{"id": "join_strategy", "component": "select", "label": "Join strategy", "options": []map[string]any{{"value": "auto", "label": "Auto"}, {"value": "router-first", "label": "Router first"}}, "default": "auto"},
						},
					},
				},
				"steps": []map[string]any{
					{"id": "active", "label": "Pairing Active", "stage": "active"},
					{"id": "detected", "label": "Device Detected", "stage": "device_detected"},
					{"id": "interview", "label": "Interview", "stage": "interviewing"},
					{"id": "completed", "label": "Completed", "stage": "completed"},
				},
			},
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
		"protocol":   "zigbee",
		"status":     status,
		"reason":     reason,
		"version":    adapterVersion(),
		"features": map[string]any{
			"supports_pairing":   true,
			"supports_interview": true,
		},
		"pairing": map[string]any{
			"schema_version":      "1.0",
			"label":               "Zigbee (Zigbee2MQTT)",
			"supported":           true,
			"supports_interview":  true,
			"default_timeout_sec": 60,
			"cta_label":           "Start Zigbee pairing",
			"flow": map[string]any{
				"id":          "zigbee-default-flow",
				"entry_modes": []string{"default"},
				"forms": []map[string]any{
					{
						"mode":  "default",
						"label": "Default",
						"fields": []map[string]any{
							{"id": "timeout", "component": "number", "label": "Permit join timeout (seconds)", "bind": "timeout", "min": 30, "max": 300, "default": 60},
							{"id": "allow_multiple_devices", "component": "checkbox", "label": "Keep pairing open for multiple devices", "description": "Optional. Use this for lamps or devices you want to add together before stopping pairing.", "default": false},
						},
					},
				},
			},
		},
		"ts": time.Now().UnixMilli(),
	}
	if hb, err := json.Marshal(hdp); err == nil {
		_ = z.client.PublishWith(hdpAdapterStatusPrefix+z.adapterID, hb, true)
	}
	return nil
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
	z.publishHDPTelemetry(hdpStatePrefix+deviceID, envelope, true)
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
		"name":         dev.Name,
		"protocol":     "zigbee",
		"manufacturer": dev.Manufacturer,
		"model":        dev.Model,
		"description":  dev.Description,
		"icon":         dev.Icon,
		"online":       dev.Online,
		"last_seen":    dev.LastSeen,
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
	z.publishHDPTelemetry(hdpMetadataPrefix+deviceID, envelope, true)
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
	z.publishHDPTelemetry(hdpEventPrefix+id, envelope, false)
}

func (z *ZigbeeAdapter) publishHDPTelemetry(topic string, envelope map[string]any, retained bool) {
	failed := func(string) {}
	if observability.MessagePropagationEnabled() {
		ctx, span := observability.StartMessageProducer(context.Background())
		defer span.End()
		observability.InjectMessageContext(ctx, envelope)
		failed = func(message string) { span.SetStatus(codes.Error, message) }
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		failed("MQTT message encoding failed")
		return
	}
	if retained {
		err = z.client.PublishWith(topic, payload, true)
	} else {
		err = z.client.Publish(topic, payload)
	}
	if err != nil {
		failed("MQTT publish failed")
	}
}

func (z *ZigbeeAdapter) publishHDPCommandResult(dev *model.Device, corr string, success bool, status, errMsg string) {
	z.publishHDPCommandResultWithContext(context.Background(), dev, corr, success, status, errMsg)
}

func (z *ZigbeeAdapter) publishHDPCommandResultWithContext(ctx context.Context, dev *model.Device, corr string, success bool, status, errMsg string) {
	if dev == nil || corr == "" {
		return
	}
	deviceID := z.hdpDeviceID(dev.ExternalID)
	if deviceID == "" {
		return
	}
	ctx, span := observability.StartMessageProducer(ctx)
	defer span.End()
	envelope := map[string]any{
		"schema":    hdpSchema,
		"type":      "command_result",
		"origin":    "adapter",
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
	if observability.MessagePropagationEnabled() {
		observability.InjectMessageContext(ctx, envelope)
	}
	if b, err := json.Marshal(envelope); err == nil {
		if err := z.client.Publish(hdpCommandResultPrefix+deviceID, b); err != nil {
			span.SetStatus(codes.Error, "MQTT publish failed")
		}
	} else {
		span.SetStatus(codes.Error, "MQTT message encoding failed")
	}
}
