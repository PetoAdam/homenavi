package adapter

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
)

const heartbeatInterval = 20 * time.Second
const demoStateTickInterval = 15 * time.Second

// Client is the subset of MQTT behavior the adapter needs.
type Client interface {
	Publish(topic string, payload []byte) error
	PublishWith(topic string, payload []byte, retain bool) error
	Subscribe(topic string, cb Handler) error
}

type Message interface {
	Topic() string
	Payload() []byte
	Qos() byte
	Retained() bool
	Duplicate() bool
}

type Handler func(Message)

// Service is a minimal Thread placeholder that keeps observability and health
// endpoints alive while the protocol implementation is built.
type Service struct {
	client    Client
	enabled   bool
	adapterID string
	version   string
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	demoTick  int
	devices   map[string]*demoDevice
}

func New(client Client, cfg Config) *Service {
	return &Service{client: client, enabled: cfg.Enabled, adapterID: cfg.AdapterID, version: cfg.Version}
}

func (s *Service) Start(ctx context.Context) error {
	if !s.enabled {
		slog.Info("mock adapter disabled", "status", "placeholder")
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.initDemoCatalog()
	s.publishHello()
	s.publishStatus("online", "ready")
	s.publishDemoCatalog()
	if err := s.client.Subscribe(hdp.PairingCommandPrefix+"mock", s.handlePairingCommand); err != nil {
		slog.Warn("mock adapter pairing subscribe failed", "error", err)
	}
	if err := s.client.Subscribe(hdp.CommandPrefix+"mock/#", s.handleDeviceCommand); err != nil {
		slog.Warn("mock adapter command subscribe failed", "error", err)
	}
	go s.runHeartbeat()
	go s.runDemoTicker()
	slog.Info("mock adapter running", "devices", len(s.demoDeviceSnapshots()))
	return nil
}

func (s *Service) Stop() {
	slog.Info("mock adapter stopping")
	if s.cancel != nil {
		s.cancel()
	}
	s.publishStatus("offline", "shutdown")
}

func (s *Service) initDemoCatalog() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.devices) != 0 {
		return
	}
	s.devices = demoCatalog()
	s.demoTick = 0
}

func (s *Service) runHeartbeat() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.publishStatus("online", "heartbeat")
		}
	}
}

func (s *Service) runDemoTicker() {
	ticker := time.NewTicker(demoStateTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			for _, snapshot := range s.advanceDemoStates() {
				s.publishState(snapshot.HDPDeviceID, snapshot.State, "")
			}
		}
	}
}

type demoSnapshot struct {
	HDPDeviceID  string
	Type         string
	Manufacturer string
	Model        string
	Description  string
	Icon         string
	Capabilities []map[string]any
	Inputs       []map[string]any
	State        map[string]any
}

func (s *Service) demoDeviceSnapshots() []demoSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]demoSnapshot, 0, len(s.devices))
	for _, device := range s.devices {
		out = append(out, demoSnapshot{
			HDPDeviceID:  device.HDPDeviceID,
			Type:         device.Type,
			Manufacturer: device.Manufacturer,
			Model:        device.Model,
			Description:  device.Description,
			Icon:         device.Icon,
			Capabilities: cloneArrayOfMaps(device.Capabilities),
			Inputs:       cloneArrayOfMaps(device.Inputs),
			State:        cloneAnyMap(device.State),
		})
	}
	return out
}

func (s *Service) advanceDemoStates() []demoSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.devices) == 0 {
		return nil
	}
	s.demoTick++
	step := s.demoTick
	updates := []demoSnapshot{}
	appendUpdate := func(deviceID string) {
		device, ok := s.devices[deviceID]
		if !ok {
			return
		}
		updates = append(updates, demoSnapshot{HDPDeviceID: device.HDPDeviceID, State: cloneAnyMap(device.State)})
	}
	if device, ok := s.devices[normalizeDemoHDPDeviceID("sofa-lamp")]; ok {
		levels := []int{32, 44, 58, 41, 67, 36}
		level := levels[step%len(levels)]
		device.State["brightness"] = level
		device.State["on"] = level > 0
		appendUpdate(device.HDPDeviceID)
	}
	if device, ok := s.devices[normalizeDemoHDPDeviceID("tv-backlight")]; ok && step%2 == 0 {
		levels := []int{18, 24, 30, 22}
		level := levels[(step/2)%len(levels)]
		device.State["brightness"] = level
		device.State["on"] = true
		appendUpdate(device.HDPDeviceID)
	}
	if device, ok := s.devices[normalizeDemoHDPDeviceID("kitchen-pendant")]; ok && step%4 == 0 {
		on := step%8 == 0
		device.State["on"] = on
		if on {
			device.State["brightness"] = 72
		} else {
			device.State["brightness"] = 0
		}
		appendUpdate(device.HDPDeviceID)
	}
	if device, ok := s.devices[normalizeDemoHDPDeviceID("coffee-maker")]; ok {
		cycle := []struct {
			power string
			draw  float64
		}{{"off", 0.6}, {"on", 810}, {"on", 905}, {"on", 640}, {"off", 1.2}}
		state := cycle[step%len(cycle)]
		device.State["power"] = state.power
		device.State["power_draw"] = state.draw
		appendUpdate(device.HDPDeviceID)
	}
	if device, ok := s.devices[normalizeDemoHDPDeviceID("bedroom-blind")]; ok && step%3 == 0 {
		positions := []int{78, 62, 48, 30, 18, 42}
		device.State["position"] = positions[(step/3)%len(positions)]
		appendUpdate(device.HDPDeviceID)
	}
	if device, ok := s.devices[normalizeDemoHDPDeviceID("desk-lamp")]; ok && step%5 == 0 {
		levels := []int{55, 61, 68, 58}
		level := levels[(step/5)%len(levels)]
		device.State["brightness"] = level
		device.State["on"] = true
		appendUpdate(device.HDPDeviceID)
	}
	if device, ok := s.devices[normalizeDemoHDPDeviceID("entry-sensor")]; ok && step%6 < 2 {
		device.State["contact"] = step%6 == 0
		appendUpdate(device.HDPDeviceID)
	}
	if device, ok := s.devices[normalizeDemoHDPDeviceID("air-purifier")]; ok {
		type airSnapshot struct {
			quality int
			speed   string
			on      bool
		}
		sequence := []airSnapshot{
			{quality: 11, speed: "auto", on: true},
			{quality: 13, speed: "low", on: true},
			{quality: 18, speed: "low", on: true},
			{quality: 26, speed: "medium", on: true},
			{quality: 34, speed: "high", on: true},
			{quality: 29, speed: "high", on: true},
			{quality: 21, speed: "medium", on: true},
			{quality: 15, speed: "auto", on: true},
		}
		state := sequence[step%len(sequence)]
		device.State["air_quality"] = state.quality
		device.State["fan_speed"] = state.speed
		device.State["on"] = state.on
		device.State["on"] = true
		appendUpdate(device.HDPDeviceID)
	}
	return updates
}

func (s *Service) applyDemoStatePatch(deviceID string, patch map[string]any) (map[string]any, error) {
	canonical := normalizeDemoHDPDeviceID(deviceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	device, ok := s.devices[canonical]
	if !ok {
		return nil, errUnsupportedDevice
	}
	recognized := false
	for key, raw := range patch {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "transition_ms", "transitionms":
			continue
		case "on", "state":
			if device.Profile != demoProfileLight && device.Profile != demoProfileAirPurifier && device.Profile != demoProfilePlug {
				return nil, errUnsupportedCommand
			}
			on, ok := normalizeBool(raw)
			if !ok {
				return nil, errInvalidCommand
			}
			recognized = true
			switch device.Profile {
			case demoProfilePlug:
				if on {
					device.State["power"] = "on"
					if _, exists := device.State["power_draw"]; !exists {
						device.State["power_draw"] = 720.0
					}
				} else {
					device.State["power"] = "off"
					device.State["power_draw"] = 0.6
				}
			default:
				device.State["on"] = on
				if !on {
					if _, exists := device.State["brightness"]; exists {
						device.State["brightness"] = 0
					}
				}
			}
		case "brightness":
			if device.Profile != demoProfileLight {
				return nil, errUnsupportedCommand
			}
			value, ok := normalizeFloat(raw)
			if !ok {
				return nil, errInvalidCommand
			}
			recognized = true
			level := clampPercent(value)
			device.State["brightness"] = level
			device.State["on"] = level > 0
		case "power":
			if device.Profile != demoProfilePlug {
				return nil, errUnsupportedCommand
			}
			on, ok := normalizeBool(raw)
			if !ok {
				return nil, errInvalidCommand
			}
			recognized = true
			if on {
				device.State["power"] = "on"
				device.State["power_draw"] = 720.0
			} else {
				device.State["power"] = "off"
				device.State["power_draw"] = 0.6
			}
		case "position":
			if device.Profile != demoProfileBlind {
				return nil, errUnsupportedCommand
			}
			value, ok := normalizeFloat(raw)
			if !ok {
				return nil, errInvalidCommand
			}
			recognized = true
			device.State["position"] = clampPercent(value)
		case "fan_speed":
			if device.Profile != demoProfileAirPurifier {
				return nil, errUnsupportedCommand
			}
			speed, ok := normalizeFanSpeed(raw)
			if !ok {
				return nil, errInvalidCommand
			}
			recognized = true
			device.State["fan_speed"] = speed
			device.State["on"] = true
		default:
			return nil, errUnsupportedCommand
		}
	}
	if !recognized {
		return nil, errUnsupportedCommand
	}
	return cloneAnyMap(device.State), nil
}

func (s *Service) hdpDeviceID(deviceID string) string {
	id := strings.Trim(strings.TrimSpace(deviceID), "/")
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, "mock/") {
		return id
	}
	parts := strings.Split(id, "/")
	suffix := strings.TrimSpace(parts[len(parts)-1])
	if suffix == "" {
		return ""
	}
	return "mock/" + suffix
}

func (s *Service) externalFromHDP(deviceID string) (string, string) {
	id := strings.Trim(strings.TrimSpace(deviceID), "/")
	if id == "" {
		return "", ""
	}
	parts := strings.Split(id, "/")
	if len(parts) == 1 {
		return "mock", parts[0]
	}
	proto := strings.ToLower(parts[0])
	if len(parts) >= 3 {
		return proto, strings.Join(parts[2:], "/")
	}
	return proto, strings.Join(parts[1:], "/")
}
