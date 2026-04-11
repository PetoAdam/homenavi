package zigbee

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/PetoAdam/homenavi/zigbee-adapter/internal/proto/adapterutil"
)

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
		_ = z.client.Publish("zigbee2mqtt/bridge/request/permit_join", []byte(fmt.Sprintf(`{"time":%d}`, cmd.Timeout)))
		z.publishPairingProgress("active", "active", "", "")
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
		z.publishPairingProgress("stopped", "stopped", "", "")
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
	_ = z.client.Publish("zigbee2mqtt/bridge/request/permit_join", []byte(`{"time":0}`))
}

func (z *ZigbeeAdapter) publishZigbee2MQTTRemove(target string, force bool) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	payload := map[string]any{
		"id":    target,
		"block": false,
		"force": force,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return z.client.Publish("zigbee2mqtt/bridge/request/device/remove", data)
}

func (z *ZigbeeAdapter) publishPairingProgress(stage, status, external string, _ string) {
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
		if deviceID := z.hdpDeviceID(external); deviceID != "" {
			hdp["device_id"] = deviceID
		}
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
		// Zigbee2MQTT may emit announce shortly after join; treat it as "detected".
		return "device_detected"
	default:
		return ""
	}
}

func interviewStageFromStatus(status string) string {
	switch strings.ToLower(status) {
	case "started", "interview_started":
		return "interviewing"
	case "successful", "success", "completed", "complete":
		return "interview_complete"
	case "failed", "failure", "error":
		return "failed"
	default:
		return "interviewing"
	}
}
