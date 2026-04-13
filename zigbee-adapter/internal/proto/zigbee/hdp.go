package zigbee

import (
	"encoding/json"
	"strings"
	"time"

	model "github.com/PetoAdam/homenavi/zigbee-adapter/internal/devices"
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
			"label":               "Zigbee (Zigbee2MQTT)",
			"supported":           true,
			"supports_interview":  true,
			"default_timeout_sec": 60,
			"instructions": []string{
				"Reset or power-cycle the device to enter pairing mode.",
				"Keep it close to the coordinator while pairing runs.",
				"We will auto-register it as soon as it is detected.",
			},
			"cta_label": "Start Zigbee pairing",
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
			"label":               "Zigbee (Zigbee2MQTT)",
			"supported":           true,
			"supports_interview":  true,
			"default_timeout_sec": 60,
			"cta_label":           "Start Zigbee pairing",
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
	if b, err := json.Marshal(envelope); err == nil {
		_ = z.client.Publish(hdpCommandResultPrefix+deviceID, b)
	}
}
