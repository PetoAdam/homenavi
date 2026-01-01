package autoimport

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"entity-registry-service/internal/mqtt"
	"entity-registry-service/internal/realtime"
	"entity-registry-service/internal/store"

	paho "github.com/eclipse/paho.mqtt.golang"
)

const (
	hdpRoot         = "homenavi/hdp/"
	hdpMetadataPref = hdpRoot + "device/metadata/"
	hdpStatePref    = hdpRoot + "device/state/"
	hdpEventPref    = hdpRoot + "device/event/"
)

type Runner struct {
	repo *store.Repo
	cli  *mqtt.Client
	hub  *realtime.Hub

	seenMu sync.Mutex
	seen   map[string]time.Time
}

type hdpEnvelope struct {
	DeviceID     string `json:"device_id"`
	Description  string `json:"description"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Protocol     string `json:"protocol"`
}

type hdpEventEnvelope struct {
	Type     string         `json:"type"`
	Event    string         `json:"event"`
	DeviceID string         `json:"device_id"`
	Data     map[string]any `json:"data"`
}

func (r *Runner) handleMessage(ctx context.Context, topic string, payload []byte) {
	if strings.HasPrefix(topic, hdpEventPref) {
		if len(payload) == 0 {
			return
		}
		hdpID := extractHDPIDFromTopic(topic)
		var env hdpEventEnvelope
		if err := json.Unmarshal(payload, &env); err == nil {
			if hdpID == "" {
				hdpID = strings.TrimSpace(env.DeviceID)
			}
			ev := strings.TrimSpace(env.Event)
			if ev == "" {
				ev = strings.TrimSpace(env.Type)
			}
			if ev != "device_removed" {
				return
			}
		} else {
			// If we can't parse the payload, don't risk deleting anything.
			return
		}

		if hdpID == "" {
			return
		}

		msgCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		deviceID, ok, err := r.repo.FindDeviceIDByHDPExternalID(msgCtx, hdpID)
		if err != nil {
			slog.Warn("ers auto-delete lookup failed", "device_id", hdpID, "error", err)
			return
		}
		if !ok {
			return
		}

		view, err := r.repo.GetDeviceView(msgCtx, deviceID)
		if err != nil {
			slog.Warn("ers auto-delete read failed", "device_id", hdpID, "error", err)
			return
		}
		remaining := make([]string, 0, len(view.HDPExternalIDs))
		for _, id := range view.HDPExternalIDs {
			x := strings.TrimSpace(id)
			if x == "" || x == hdpID {
				continue
			}
			remaining = append(remaining, x)
		}

		if len(remaining) == 0 {
			if err := r.repo.DeleteDevice(msgCtx, deviceID); err != nil {
				slog.Warn("ers auto-delete failed", "device_id", hdpID, "error", err)
				return
			}
			r.seenMu.Lock()
			if r.seen != nil {
				delete(r.seen, hdpID)
			}
			r.seenMu.Unlock()
			if r.hub != nil {
				r.hub.Broadcast(realtime.Event{Type: "ers.device.deleted", Entity: "device", ID: deviceID.String()})
			}
			slog.Info("ers auto-deleted device", "device_id", hdpID)
			return
		}

		if err := r.repo.SetDeviceHDPBindings(msgCtx, deviceID, remaining); err != nil {
			slog.Warn("ers auto-unbind failed", "device_id", hdpID, "error", err)
			return
		}
		r.seenMu.Lock()
		if r.seen != nil {
			delete(r.seen, hdpID)
		}
		r.seenMu.Unlock()
		if r.hub != nil {
			r.hub.Broadcast(realtime.Event{Type: "ers.device.bindings_updated", Entity: "device", ID: deviceID.String()})
		}
		slog.Info("ers auto-unbound hdp id", "device_id", hdpID)
		return
	}

	// Metadata/state import path.
	if len(payload) == 0 {
		// Device hub clears retained metadata/state by publishing empty payloads.
		// ERS should tolerate churn; keep existing bindings.
		return
	}

	hdpID := extractHDPIDFromTopic(topic)
	if hdpID == "" {
		// Fall back to payload parsing.
		var env hdpEnvelope
		if err := json.Unmarshal(payload, &env); err == nil {
			hdpID = strings.TrimSpace(env.DeviceID)
		}
	}
	if hdpID == "" {
		return
	}

	// Avoid hammering Postgres on frequent state updates.
	// We only need to ensure existence once per device over a short window.
	{
		now := time.Now()
		r.seenMu.Lock()
		if r.seen == nil {
			r.seen = map[string]time.Time{}
		}
		if last, ok := r.seen[hdpID]; ok {
			if now.Sub(last) < 2*time.Minute {
				r.seenMu.Unlock()
				return
			}
		}
		// Mark as seen pre-emptively (best-effort); if DB call fails we'll overwrite next time.
		r.seen[hdpID] = now
		r.seenMu.Unlock()
	}

	name, desc := "", ""
	var env hdpEnvelope
	if err := json.Unmarshal(payload, &env); err == nil {
		desc = strings.TrimSpace(env.Description)
		fallback := strings.TrimSpace(strings.Join([]string{env.Manufacturer, env.Model}, " "))
		if fallback != "" {
			name = fallback
		}
	}

	// Short timeout per message to avoid stuck goroutines.
	msgCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	deviceID, created, err := r.repo.EnsureDeviceForHDP(msgCtx, hdpID, name, desc)
	if err != nil {
		// Allow retry soon if DB was temporarily unavailable.
		r.seenMu.Lock()
		if r.seen != nil {
			delete(r.seen, hdpID)
		}
		r.seenMu.Unlock()
		slog.Warn("ers auto-import failed", "device_id", hdpID, "error", err)
		return
	}
	// Keep it marked as seen if DB succeeded.
	_ = deviceID
	if created {
		if r.hub != nil {
			r.hub.Broadcast(realtime.Event{Type: "ers.device.created", Entity: "device", ID: deviceID.String()})
		}
		slog.Info("ers auto-imported device", "device_id", hdpID)
	}
}

func Start(ctx context.Context, repo *store.Repo, brokerURL string, hub *realtime.Hub) *Runner {
	if strings.TrimSpace(brokerURL) == "" {
		brokerURL = "tcp://mosquitto:1883"
	}
	r := &Runner{repo: repo, hub: hub, seen: map[string]time.Time{}}
	r.cli = mqtt.New(brokerURL, "entity-registry-autoimport")

	h := func(_ paho.Client, msg mqtt.Message) {
		r.handleMessage(ctx, msg.Topic(), msg.Payload())
	}

	// Subscribe to both metadata and state retained streams.
	_ = r.cli.Subscribe(hdpMetadataPref+"#", h)
	_ = r.cli.Subscribe(hdpStatePref+"#", h)
	_ = r.cli.Subscribe(hdpEventPref+"#", h)

	go func() {
		<-ctx.Done()
		r.cli.Disconnect(250)
	}()

	return r
}

func extractHDPIDFromTopic(topic string) string {
	if strings.HasPrefix(topic, hdpMetadataPref) {
		return strings.TrimPrefix(topic, hdpMetadataPref)
	}
	if strings.HasPrefix(topic, hdpStatePref) {
		return strings.TrimPrefix(topic, hdpStatePref)
	}
	if strings.HasPrefix(topic, hdpEventPref) {
		return strings.TrimPrefix(topic, hdpEventPref)
	}
	return ""
}
