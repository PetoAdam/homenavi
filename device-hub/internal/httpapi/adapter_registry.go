package httpapi

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	hdpAdapterHelloTopic   = "homenavi/hdp/adapter/hello"
	hdpAdapterStatusPrefix = "homenavi/hdp/adapter/status/"
)

type adapterStatus struct {
	AdapterID string
	Protocol  string
	Status    string
	Reason    string
	Version   string
	LastSeen  time.Time
	Pairing   *PairingConfig
	// SupportsPairing is a lightweight flag derived from features when no detailed pairing config is provided.
	SupportsPairing bool
}

func boolish(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	case float64:
		return t != 0
	case float32:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	case uint64:
		return t != 0
	default:
		return false
	}
}

func intish(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case string:
		// avoid strconv import here; keep simple
		return 0
	default:
		return 0
	}
}

func stringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		if ss, ok := v.([]string); ok {
			out := make([]string, 0, len(ss))
			for _, s := range ss {
				if strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
			return out
		}
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s := strings.TrimSpace(asString(item))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parsePairingConfig(protocol string, msg map[string]any) (*PairingConfig, bool) {
	proto := normalizeProtocol(protocol)
	if proto == "" || msg == nil {
		return nil, false
	}
	// Preferred: explicit pairing object.
	if raw, ok := msg["pairing"].(map[string]any); ok {
		cfg := &PairingConfig{Protocol: proto}
		if label := strings.TrimSpace(asString(raw["label"])); label != "" {
			cfg.Label = label
		}
		cfg.Supported = boolish(raw["supported"])
		cfg.SupportsInterview = boolish(raw["supports_interview"])
		if t := intish(raw["default_timeout_sec"]); t > 0 {
			cfg.DefaultTimeoutSec = t
		}
		cfg.Instructions = stringSlice(raw["instructions"])
		cfg.CTALabel = strings.TrimSpace(asString(raw["cta_label"]))
		cfg.Notes = strings.TrimSpace(asString(raw["notes"]))
		return cfg, true
	}
	// Fallback: features.supports_pairing indicates adapter can accept pairing commands.
	features, _ := msg["features"].(map[string]any)
	if features != nil && boolish(features["supports_pairing"]) {
		cfg := &PairingConfig{Protocol: proto, Label: strings.ToUpper(proto[:1]) + proto[1:], Supported: true, DefaultTimeoutSec: 60}
		cfg.SupportsInterview = boolish(features["supports_interview"])
		return cfg, true
	}
	return nil, false
}

type adapterRegistry struct {
	mu       sync.RWMutex
	byID     map[string]adapterStatus
	ttl      time.Duration
	nowFn    func() time.Time
	onUpdate func()
}

func newAdapterRegistry(ttl time.Duration) *adapterRegistry {
	if ttl <= 0 {
		ttl = 45 * time.Second
	}
	return &adapterRegistry{
		byID:  make(map[string]adapterStatus),
		ttl:   ttl,
		nowFn: time.Now,
	}
}

func (r *adapterRegistry) upsertFromStatusTopic(topic string, payload []byte) {
	if len(payload) == 0 {
		return
	}
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
		slog.Debug("adapter status decode failed", "topic", topic, "error", err)
		return
	}
	adapterID := strings.TrimSpace(asString(msg["adapter_id"]))
	if adapterID == "" {
		// Fallback: topic is homenavi/hdp/adapter/status/{adapter_id}
		adapterID = strings.TrimPrefix(topic, hdpAdapterStatusPrefix)
		adapterID = strings.Trim(adapterID, "/")
	}
	if adapterID == "" {
		return
	}
	protocol := normalizeProtocol(asString(msg["protocol"]))
	status := strings.TrimSpace(asString(msg["status"]))
	reason := strings.TrimSpace(asString(msg["reason"]))
	version := strings.TrimSpace(asString(msg["version"]))
	if status == "" {
		status = "unknown"
	}
	entry := adapterStatus{
		AdapterID: adapterID,
		Protocol:  protocol,
		Status:    status,
		Reason:    reason,
		Version:   version,
		LastSeen:  r.nowFn(),
	}
	if cfg, ok := parsePairingConfig(protocol, msg); ok {
		entry.Pairing = cfg
		entry.SupportsPairing = cfg.Supported
	} else {
		if features, ok := msg["features"].(map[string]any); ok {
			entry.SupportsPairing = boolish(features["supports_pairing"])
		}
	}

	r.mu.Lock()
	r.byID[adapterID] = entry
	r.mu.Unlock()

	if r.onUpdate != nil {
		r.onUpdate()
	}
}

func (r *adapterRegistry) upsertFromHello(payload []byte) {
	if len(payload) == 0 {
		return
	}
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
		slog.Debug("adapter hello decode failed", "error", err)
		return
	}
	adapterID := strings.TrimSpace(asString(msg["adapter_id"]))
	if adapterID == "" {
		return
	}
	protocol := normalizeProtocol(asString(msg["protocol"]))
	version := strings.TrimSpace(asString(msg["version"]))
	var pairingCfg *PairingConfig
	var supportsPairing bool
	if cfg, ok := parsePairingConfig(protocol, msg); ok {
		pairingCfg = cfg
		supportsPairing = cfg.Supported
	} else {
		if features, ok := msg["features"].(map[string]any); ok {
			supportsPairing = boolish(features["supports_pairing"])
		}
	}
	entry := adapterStatus{
		AdapterID:       adapterID,
		Protocol:        protocol,
		Status:          "online",
		Reason:          "hello",
		Version:         version,
		LastSeen:        r.nowFn(),
		Pairing:         pairingCfg,
		SupportsPairing: supportsPairing,
	}
	// Do not clobber a more specific status if we already have one.
	r.mu.Lock()
	existing, ok := r.byID[adapterID]
	if ok {
		if existing.Protocol == "" {
			existing.Protocol = entry.Protocol
		}
		if existing.Version == "" {
			existing.Version = entry.Version
		}
		if existing.Pairing == nil && entry.Pairing != nil {
			existing.Pairing = entry.Pairing
		}
		if !existing.SupportsPairing && entry.SupportsPairing {
			existing.SupportsPairing = true
		}
		existing.LastSeen = entry.LastSeen
		r.byID[adapterID] = existing
	} else {
		r.byID[adapterID] = entry
	}
	r.mu.Unlock()

	if r.onUpdate != nil {
		r.onUpdate()
	}
}

func (r *adapterRegistry) isOnline(entry adapterStatus) bool {
	if entry.AdapterID == "" {
		return false
	}
	if strings.EqualFold(entry.Status, "offline") {
		return false
	}
	return r.nowFn().Sub(entry.LastSeen) <= r.ttl
}

func (r *adapterRegistry) hasOnlineProtocol(protocol string) bool {
	proto := normalizeProtocol(protocol)
	if proto == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.byID {
		if normalizeProtocol(entry.Protocol) != proto {
			continue
		}
		if r.isOnline(entry) {
			return true
		}
	}
	return false
}

func (r *adapterRegistry) integrationsSnapshot() []IntegrationDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	byProto := map[string]IntegrationDescriptor{}
	for _, entry := range r.byID {
		proto := normalizeProtocol(entry.Protocol)
		if proto == "" {
			continue
		}
		desc, ok := byProto[proto]
		if !ok {
			desc = IntegrationDescriptor{Protocol: proto, Label: strings.ToUpper(proto[:1]) + proto[1:]}
		}
		if r.isOnline(entry) {
			desc.Status = "active"
		} else if desc.Status == "" {
			desc.Status = "offline"
		}
		if entry.Reason != "" {
			desc.Notes = entry.Reason
		}
		byProto[proto] = desc
	}
	items := make([]IntegrationDescriptor, 0, len(byProto))
	for _, v := range byProto {
		items = append(items, v)
	}
	return items
}

func (r *adapterRegistry) pairingConfigsSnapshot() []PairingConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	byProto := map[string]PairingConfig{}
	for _, entry := range r.byID {
		proto := normalizeProtocol(entry.Protocol)
		if proto == "" {
			continue
		}
		isOnline := r.isOnline(entry)
		cfg, ok := byProto[proto]
		if !ok {
			cfg = PairingConfig{Protocol: proto, Label: strings.ToUpper(proto[:1]) + proto[1:]}
		}
		if entry.Pairing != nil {
			candidate := *entry.Pairing
			candidate.Protocol = proto
			// Only advertise supported when at least one adapter is online.
			candidate.Supported = candidate.Supported && isOnline
			// Prefer an online adapter's config.
			if cfg.Protocol == "" || (isOnline && !cfg.Supported) || (isOnline && cfg.Label == strings.ToUpper(proto[:1])+proto[1:]) {
				cfg = candidate
			} else if isOnline && candidate.Supported {
				cfg = candidate
			}
		} else if entry.SupportsPairing {
			// Minimal config when adapter indicates pairing support but doesn't provide UX metadata.
			cfg.Supported = isOnline
			if cfg.DefaultTimeoutSec == 0 {
				cfg.DefaultTimeoutSec = 60
			}
		}
		byProto[proto] = cfg
	}
	items := make([]PairingConfig, 0, len(byProto))
	for _, v := range byProto {
		items = append(items, v)
	}
	return items
}

func (r *adapterRegistry) isPairingSupported(protocol string) bool {
	proto := normalizeProtocol(protocol)
	if proto == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.byID {
		if normalizeProtocol(entry.Protocol) != proto {
			continue
		}
		if !r.isOnline(entry) {
			continue
		}
		if entry.Pairing != nil {
			if entry.Pairing.Supported {
				return true
			}
			continue
		}
		if entry.SupportsPairing {
			return true
		}
	}
	return false
}

func (r *adapterRegistry) supportsInterview(protocol string) bool {
	proto := normalizeProtocol(protocol)
	if proto == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.byID {
		if normalizeProtocol(entry.Protocol) != proto {
			continue
		}
		if !r.isOnline(entry) {
			continue
		}
		if entry.Pairing != nil {
			return entry.Pairing.SupportsInterview
		}
	}
	return false
}
