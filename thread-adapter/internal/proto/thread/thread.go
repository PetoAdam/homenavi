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
	a.publishHello()
	a.publishStatus("online", "placeholder")
	if err := a.client.Subscribe(hdpPairingCommandTopic, a.handlePairingCommand); err != nil {
		slog.Warn("thread adapter pairing subscribe failed", "error", err)
	}
	if err := a.client.Subscribe(hdpCommandPrefix+"thread/#", a.handleDeviceCommand); err != nil {
		slog.Warn("thread adapter command subscribe failed", "error", err)
	}
	slog.Info("thread adapter placeholder running", "status", "planned")
	return nil
}

func (a *Adapter) Stop() {
	slog.Info("thread adapter stopping")
	a.publishStatus("offline", "shutdown")
	// Placeholder: nothing to tear down yet beyond MQTT handled by main.
}

func (a *Adapter) publishHello() {
	if a.adapterID == "" {
		return
	}
	payload := map[string]any{
		"type":        "hello",
		"adapter_id":  a.adapterID,
		"protocols":   []string{"thread"},
		"version":     a.version,
		"hdp_version": "1.0",
		"features": map[string]any{
			"supports_ack":         false,
			"supports_correlation": false,
			"supports_batch_state": false,
		},
		"timestamp": time.Now().Unix(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = a.client.Publish("homenavi/adapter/"+a.adapterID+"/hello", b)
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
	payload := map[string]any{
		"type":       "adapter_status",
		"adapter_id": a.adapterID,
		"status":     status,
		"reason":     reason,
		"version":    a.version,
		"timestamp":  time.Now().Unix(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_ = a.client.PublishWith("homenavi/adapter/"+a.adapterID+"/status", b, true)
	}
	hdp := map[string]any{
		"schema":     hdpSchema,
		"type":       "status",
		"adapter_id": a.adapterID,
		"status":     status,
		"reason":     reason,
		"version":    a.version,
		"ts":         time.Now().UnixMilli(),
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
	if strings.Contains(id, "/") {
		return "thread/" + id
	}
	if strings.TrimSpace(a.adapterID) != "" {
		return "thread/" + a.adapterID + "/" + id
	}
	return "thread/" + id
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
