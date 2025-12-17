package thread

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"thread-adapter/internal/mqtt"
)

// Adapter is a minimal Thread placeholder that keeps observability and health
// endpoints alive while the protocol implementation is built.
type Adapter struct {
	client    *mqtt.Client
	enabled   bool
	adapterID string
	version   string
	ctx       context.Context
	cancel    context.CancelFunc
}

type Config struct {
	Enabled   bool
	AdapterID string
	Version   string
}

const (
	hdpSchema               = "hdp.v1"
	hdpCommandPrefix        = "homenavi/hdp/device/command/"
	hdpCommandResultPrefix  = "homenavi/hdp/device/command_result/"
	hdpPairingCommandTopic  = "homenavi/hdp/pairing/command/thread"
	hdpPairingProgressTopic = "homenavi/hdp/pairing/progress/thread"
	hdpAdapterHelloTopic    = "homenavi/hdp/adapter/hello"
	hdpAdapterStatusPrefix  = "homenavi/hdp/adapter/status/"
)

func New(client *mqtt.Client, cfg Config) *Adapter {
	return &Adapter{client: client, enabled: cfg.Enabled, adapterID: cfg.AdapterID, version: cfg.Version}
}

func (a *Adapter) Name() string { return "thread" }

func (a *Adapter) Start(ctx context.Context) error {
	if !a.enabled {
		slog.Info("thread adapter disabled", "status", "placeholder")
		return nil
	}
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.publishHello()
	a.publishStatus("online", "placeholder")
	if err := a.client.Subscribe(hdpPairingCommandTopic, a.handlePairingCommand); err != nil {
		slog.Warn("thread adapter pairing subscribe failed", "error", err)
	}
	if err := a.client.Subscribe(hdpCommandPrefix+"thread/#", a.handleDeviceCommand); err != nil {
		slog.Warn("thread adapter command subscribe failed", "error", err)
	}
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				a.publishStatus("online", "heartbeat")
			}
		}
	}()
	slog.Info("thread adapter placeholder running", "status", "planned")
	return nil
}

func (a *Adapter) Stop() {
	slog.Info("thread adapter stopping")
	if a.cancel != nil {
		a.cancel()
	}
	a.publishStatus("offline", "shutdown")
	// Placeholder: nothing to tear down yet beyond MQTT handled by main.
}

func (a *Adapter) publishHello() {
	if a.adapterID == "" {
		return
	}
	hdp := map[string]any{
		"schema":      hdpSchema,
		"type":        "hello",
		"adapter_id":  a.adapterID,
		"protocol":    "thread",
		"version":     a.version,
		"hdp_version": "1.0",
		"features": map[string]any{
			"supports_ack":         true,
			"supports_correlation": true,
			"supports_batch_state": false,
			"supports_pairing":     true,
			"supports_interview":   false,
		},
		"pairing": map[string]any{
			"label":               "Thread",
			"supported":           true,
			"supports_interview":  false,
			"default_timeout_sec": 60,
			"instructions": []string{
				"Ensure the Thread border router is online.",
				"Put the Thread device into commissioning mode.",
				"We will attach it when the adapter reports the join.",
			},
			"cta_label": "Start Thread pairing",
			"notes":     "Placeholder implementation",
		},
		"ts": time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(hdp); err == nil {
		_ = a.client.Publish(hdpAdapterHelloTopic, b)
	}
}

func (a *Adapter) publishStatus(status, reason string) {
	if a.adapterID == "" {
		return
	}
	hdp := map[string]any{
		"schema":     hdpSchema,
		"type":       "status",
		"adapter_id": a.adapterID,
		"protocol":   "thread",
		"status":     status,
		"reason":     reason,
		"version":    a.version,
		"features": map[string]any{
			"supports_pairing":   true,
			"supports_interview": false,
		},
		"pairing": map[string]any{
			"label":               "Thread",
			"supported":           true,
			"supports_interview":  false,
			"default_timeout_sec": 60,
			"cta_label":           "Start Thread pairing",
			"notes":               "Placeholder implementation",
		},
		"ts": time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(hdp); err == nil {
		_ = a.client.PublishWith(hdpAdapterStatusPrefix+a.adapterID, b, true)
	}
}

func (a *Adapter) hdpDeviceID(deviceID string) string {
	id := strings.Trim(strings.TrimSpace(deviceID), "/")
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, "thread/") {
		return id
	}
	parts := strings.Split(id, "/")
	suffix := strings.TrimSpace(parts[len(parts)-1])
	if suffix == "" {
		return ""
	}
	return "thread/" + suffix
}

func (a *Adapter) externalFromHDP(deviceID string) (string, string) {
	id := strings.Trim(strings.TrimSpace(deviceID), "/")
	if id == "" {
		return "", ""
	}
	parts := strings.Split(id, "/")
	if len(parts) == 1 {
		return "thread", parts[0]
	}
	proto := strings.ToLower(parts[0])
	if len(parts) >= 3 {
		return proto, strings.Join(parts[2:], "/")
	}
	return proto, strings.Join(parts[1:], "/")
}

func (a *Adapter) publishCommandResult(deviceID, corr string, success bool, status, errMsg string) {
	id := a.hdpDeviceID(deviceID)
	if id == "" || corr == "" {
		return
	}
	env := map[string]any{
		"schema":    hdpSchema,
		"type":      "command_result",
		"device_id": id,
		"corr":      corr,
		"success":   success,
		"status":    status,
		"ts":        time.Now().UnixMilli(),
	}
	if errMsg != "" {
		env["error"] = errMsg
	}
	if b, err := json.Marshal(env); err == nil {
		_ = a.client.Publish(hdpCommandResultPrefix+id, b)
	}
}

func (a *Adapter) publishPairingProgress(stage, status, external string) {
	if stage == "" {
		return
	}
	env := map[string]any{
		"schema":   hdpSchema,
		"type":     "pairing_progress",
		"protocol": "thread",
		"stage":    stage,
		"status":   status,
		"ts":       time.Now().UnixMilli(),
	}
	if external != "" {
		env["external_id"] = external
	}
	if b, err := json.Marshal(env); err == nil {
		_ = a.client.Publish(hdpPairingProgressTopic, b)
	}
}

func (a *Adapter) handleDeviceCommand(_ paho.Client, m paho.Message) {
	var env map[string]any
	if err := json.Unmarshal(m.Payload(), &env); err != nil {
		slog.Debug("thread hdp command decode failed", "error", err)
		return
	}
	deviceID, _ := env["device_id"].(string)
	if deviceID == "" {
		deviceID = strings.TrimPrefix(m.Topic(), hdpCommandPrefix)
	}
	proto, external := a.externalFromHDP(deviceID)
	if proto != "" && proto != "thread" {
		return
	}
	corr, _ := env["corr"].(string)
	if corr == "" {
		if cid, ok := env["correlation_id"].(string); ok {
			corr = cid
		}
	}
	if corr == "" {
		corr = "ack-" + strings.ReplaceAll(external, "/", "-")
	}
	a.publishCommandResult(deviceID, corr, true, "accepted", "")
	slog.Info("thread adapter command ack", "device_id", deviceID, "corr", corr)
}

func (a *Adapter) handlePairingCommand(_ paho.Client, m paho.Message) {
	var env map[string]any
	if err := json.Unmarshal(m.Payload(), &env); err != nil {
		slog.Debug("thread pairing decode failed", "error", err)
		return
	}
	actionVal, _ := env["action"].(string)
	action := strings.ToLower(strings.TrimSpace(actionVal))
	if action == "" {
		if strings.Contains(strings.ToLower(m.Topic()), "start") {
			action = "start"
		}
	}
	switch action {
	case "start":
		a.publishPairingProgress("commissioning", "in_progress", "")
		a.publishPairingProgress("completed", "completed", "")
	case "stop":
		a.publishPairingProgress("stopped", "stopped", "")
	default:
		return
	}
}
