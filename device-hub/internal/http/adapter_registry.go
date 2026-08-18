package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	adapterstate "github.com/PetoAdam/homenavi/device-hub/internal/infra/adapterstate"
	"github.com/PetoAdam/homenavi/shared/hdp"
)

const (
	hdpAdapterHelloTopic   = hdp.AdapterHelloTopic
	hdpAdapterStatusPrefix = hdp.AdapterStatusPrefix
)

type adapterStatus struct {
	AdapterID string
	Protocol  string
	Status    string
	Reason    string
	Version   string
	LastSeen  time.Time
	Pairing   *PairingConfig
}

func mergeFlow(existing, incoming any) any {
	existingMap, existingOK := existing.(map[string]any)
	incomingMap, incomingOK := incoming.(map[string]any)
	if !existingOK {
		if incomingOK {
			return incomingMap
		}
		if incoming != nil {
			return incoming
		}
		return existing
	}
	if !incomingOK {
		if incoming != nil {
			return incoming
		}
		return existingMap
	}
	merged := make(map[string]any, len(existingMap)+len(incomingMap))
	for k, v := range existingMap {
		merged[k] = v
	}
	for k, v := range incomingMap {
		if v == nil {
			continue
		}
		merged[k] = v
	}
	return merged
}

func mergePairingConfig(existing, incoming *PairingConfig) *PairingConfig {
	if existing == nil {
		return incoming
	}
	if incoming == nil {
		return existing
	}
	merged := *existing
	if incoming.Protocol != "" {
		merged.Protocol = incoming.Protocol
	}
	if incoming.SchemaVersion != "" {
		merged.SchemaVersion = incoming.SchemaVersion
	}
	if incoming.Label != "" {
		merged.Label = incoming.Label
	}
	merged.Supported = incoming.Supported
	merged.SupportsInterview = incoming.SupportsInterview
	if incoming.DefaultTimeoutSec > 0 {
		merged.DefaultTimeoutSec = incoming.DefaultTimeoutSec
	}
	if len(incoming.Instructions) > 0 {
		merged.Instructions = incoming.Instructions
	}
	if incoming.CTALabel != "" {
		merged.CTALabel = incoming.CTALabel
	}
	if incoming.Notes != "" {
		merged.Notes = incoming.Notes
	}
	merged.Flow = mergeFlow(existing.Flow, incoming.Flow)
	return &merged
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
	if raw, ok := msg["pairing"].(map[string]any); ok {
		schemaVersion := strings.TrimSpace(asString(raw["schema_version"]))
		if schemaVersion == "" {
			return nil, false
		}
		cfg := &PairingConfig{Protocol: proto, SchemaVersion: schemaVersion}
		if label := strings.TrimSpace(asString(raw["label"])); label != "" {
			cfg.Label = label
		}
		cfg.Supported = boolish(raw["supported"])
		cfg.SupportsInterview = boolish(raw["supports_interview"])
		if t := intish(raw["default_timeout_sec"]); t > 0 {
			cfg.DefaultTimeoutSec = t
		}
		if cfg.Supported && cfg.DefaultTimeoutSec == 0 {
			cfg.DefaultTimeoutSec = 180
		}
		cfg.Instructions = stringSlice(raw["instructions"])
		cfg.CTALabel = strings.TrimSpace(asString(raw["cta_label"]))
		cfg.Notes = strings.TrimSpace(asString(raw["notes"]))
		if flow, ok := raw["flow"]; ok {
			cfg.Flow = flow
		}
		return cfg, true
	}
	return nil, false
}

type adapterRegistry struct {
	store    adapterstate.Store
	ttl      time.Duration
	nowFn    func() time.Time
	onUpdate func()
}

func newAdapterRegistry(store adapterstate.Store, ttl time.Duration) *adapterRegistry {
	if ttl <= 0 {
		ttl = 45 * time.Second
	}
	if store == nil {
		store = adapterstate.NewMemoryStore()
	}
	return &adapterRegistry{store: store, ttl: ttl, nowFn: time.Now}
}

func (r *adapterRegistry) get(adapterID string) (adapterStatus, bool) {
	entry, ok, err := r.store.Get(context.Background(), adapterID)
	if err != nil {
		slog.Warn("adapter registry read failed", "adapter_id", adapterID, "error", err)
		return adapterStatus{}, false
	}
	if !ok {
		return adapterStatus{}, false
	}
	status := adapterStatus{AdapterID: entry.AdapterID, Protocol: entry.Protocol, Status: entry.Status, Reason: entry.Reason, Version: entry.Version, LastSeen: entry.LastSeen}
	if len(entry.Pairing) > 0 {
		if err := json.Unmarshal(entry.Pairing, &status.Pairing); err != nil {
			slog.Warn("adapter pairing config decode failed", "adapter_id", adapterID, "error", err)
		}
	}
	return status, true
}

func (r *adapterRegistry) put(entry adapterStatus) {
	stored := adapterstate.Entry{AdapterID: entry.AdapterID, Protocol: entry.Protocol, Status: entry.Status, Reason: entry.Reason, Version: entry.Version, LastSeen: entry.LastSeen}
	if entry.Pairing != nil {
		payload, err := json.Marshal(entry.Pairing)
		if err != nil {
			slog.Warn("adapter pairing config encode failed", "adapter_id", entry.AdapterID, "error", err)
			return
		}
		stored.Pairing = payload
	}
	if err := r.store.Put(context.Background(), stored, r.ttl); err != nil {
		slog.Warn("adapter registry write failed", "adapter_id", entry.AdapterID, "error", err)
	}
}

func (r *adapterRegistry) entries() []adapterStatus {
	stored, err := r.store.List(context.Background())
	if err != nil {
		slog.Warn("adapter registry list failed", "error", err)
		return nil
	}
	entries := make([]adapterStatus, 0, len(stored))
	for _, entry := range stored {
		status, ok := r.get(entry.AdapterID)
		if ok {
			entries = append(entries, status)
		}
	}
	return entries
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
		adapterID = strings.Trim(strings.TrimPrefix(topic, hdpAdapterStatusPrefix), "/")
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
	entry := adapterStatus{AdapterID: adapterID, Protocol: protocol, Status: status, Reason: reason, Version: version, LastSeen: r.nowFn()}
	if cfg, ok := parsePairingConfig(protocol, msg); ok {
		entry.Pairing = cfg
	}
	if existing, ok := r.get(adapterID); ok {
		if entry.Protocol == "" {
			entry.Protocol = existing.Protocol
		}
		if entry.Version == "" {
			entry.Version = existing.Version
		}
		entry.Pairing = mergePairingConfig(existing.Pairing, entry.Pairing)
	}
	r.put(entry)
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
	if cfg, ok := parsePairingConfig(protocol, msg); ok {
		pairingCfg = cfg
	}
	entry := adapterStatus{AdapterID: adapterID, Protocol: protocol, Status: "online", Reason: "hello", Version: version, LastSeen: r.nowFn(), Pairing: pairingCfg}
	existing, ok := r.get(adapterID)
	if ok {
		if existing.Protocol == "" {
			existing.Protocol = entry.Protocol
		}
		if existing.Version == "" {
			existing.Version = entry.Version
		}
		existing.Pairing = mergePairingConfig(existing.Pairing, entry.Pairing)
		existing.LastSeen = entry.LastSeen
		r.put(existing)
	} else {
		r.put(entry)
	}
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

func (r *adapterRegistry) integrationsSnapshot() []IntegrationDescriptor {
	byProto := map[string]IntegrationDescriptor{}
	for _, entry := range r.entries() {
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
	byProto := map[string]PairingConfig{}
	for _, entry := range r.entries() {
		proto := normalizeProtocol(entry.Protocol)
		if proto == "" || entry.Pairing == nil {
			continue
		}
		isOnline := r.isOnline(entry)
		cfg, ok := byProto[proto]
		if !ok {
			cfg = PairingConfig{Protocol: proto, Label: strings.ToUpper(proto[:1]) + proto[1:]}
		}
		candidate := *entry.Pairing
		candidate.Protocol = proto
		candidate.Supported = candidate.Supported && isOnline
		if cfg.Protocol == "" || (isOnline && !cfg.Supported) || (isOnline && cfg.Label == strings.ToUpper(proto[:1])+proto[1:]) {
			cfg = candidate
		} else if isOnline && candidate.Supported {
			cfg = candidate
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
	for _, entry := range r.entries() {
		if normalizeProtocol(entry.Protocol) != proto || !r.isOnline(entry) {
			continue
		}
		if entry.Pairing != nil {
			if entry.Pairing.Supported {
				return true
			}
			continue
		}
	}
	return false
}

func (r *adapterRegistry) supportsInterview(protocol string) bool {
	proto := normalizeProtocol(protocol)
	if proto == "" {
		return false
	}
	for _, entry := range r.entries() {
		if normalizeProtocol(entry.Protocol) != proto || !r.isOnline(entry) {
			continue
		}
		if entry.Pairing != nil {
			return entry.Pairing.SupportsInterview
		}
	}
	return false
}
