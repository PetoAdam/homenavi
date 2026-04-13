package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	mqttinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/hdp"

	"github.com/google/uuid"
)

func (e *Engine) handleState(ctx context.Context, m mqttinfra.Message) {
	payload := m.Payload()
	st, err := decodeJSON[HDPState](payload)
	if err != nil {
		return
	}
	if st.Schema != hdp.SchemaV1 || st.Type != "state" {
		return
	}

	type match struct {
		wfID          uuid.UUID
		triggerNodeID string
		trigger       TriggerDeviceState
	}
	var candidates []match

	e.mu.RLock()
	for id, w := range e.workflows {
		if !w.Enabled {
			continue
		}
		d, ok := e.defs[id]
		if !ok {
			continue
		}
		for _, n := range d.Nodes {
			if strings.ToLower(strings.TrimSpace(n.Kind)) != "trigger.device_state" {
				continue
			}
			var t TriggerDeviceState
			if err := json.Unmarshal(n.Data, &t); err != nil {
				continue
			}
			if t.IgnoreRetained && m.Retained() {
				continue
			}
			// Match device_id against runtime targets (device list or selector).
			ok, err := e.targetMatchesDevice(ctx, t.Targets, st.DeviceID)
			if err != nil || !ok {
				continue
			}
			candidates = append(candidates, match{wfID: id, triggerNodeID: n.ID, trigger: t})
		}
	}
	e.mu.RUnlock()

	for _, c := range candidates {
		if !matchStateTrigger(c.trigger, st.State) {
			continue
		}
		coolKey := c.wfID.String() + ":" + c.triggerNodeID
		if !e.allowFire(coolKey, c.trigger.CooldownSec) {
			continue
		}
		_, _ = e.StartWorkflowRun(ctx, c.wfID, c.triggerNodeID, map[string]any{"type": "state", "trigger_node_id": c.triggerNodeID, "device_id": st.DeviceID, "state": st.State, "ts": st.TS, "retained": m.Retained()})
	}
}

func (e *Engine) targetMatchesDevice(ctx context.Context, targets NodeTargets, deviceID string) (bool, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false, nil
	}
	typ := strings.ToLower(strings.TrimSpace(targets.Type))
	switch typ {
	case "device":
		for _, raw := range targets.IDs {
			if strings.TrimSpace(raw) == deviceID {
				return true, nil
			}
		}
		return false, nil
	case "selector":
		ids, err := e.resolveSelector(ctx, targets.Selector)
		if err != nil {
			return false, err
		}
		for _, id := range ids {
			if id == deviceID {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, nil
	}
}

func (e *Engine) resolveTargets(ctx context.Context, targets NodeTargets) ([]string, error) {
	typ := strings.ToLower(strings.TrimSpace(targets.Type))
	switch typ {
	case "device":
		out := make([]string, 0, len(targets.IDs))
		seen := map[string]struct{}{}
		for _, raw := range targets.IDs {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
		return out, nil
	case "selector":
		return e.resolveSelector(ctx, targets.Selector)
	default:
		return nil, errors.New("unsupported targets")
	}
}

func (e *Engine) resolveSelector(ctx context.Context, selector string) ([]string, error) {
	sel := strings.TrimSpace(selector)
	if sel == "" {
		return []string{}, nil
	}
	if e.ersServiceURL == "" {
		return nil, errors.New("ERS service url not configured")
	}

	// Cache.
	e.selMu.Lock()
	if cached, ok := e.selectorCache[sel]; ok {
		if time.Since(cached.FetchedAt) < e.selectorTTL {
			out := append([]string(nil), cached.IDs...)
			e.selMu.Unlock()
			return out, nil
		}
	}
	e.selMu.Unlock()

	body, _ := json.Marshal(map[string]any{"selector": sel})
	url := e.ersServiceURL + "/api/ers/selectors/resolve"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		HDPExternalIDs []string `json:"hdp_external_ids"`
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("selector resolve failed: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.New("invalid selector response")
	}

	ids := make([]string, 0, len(out.HDPExternalIDs))
	seen := map[string]struct{}{}
	for _, raw := range out.HDPExternalIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	e.selMu.Lock()
	e.selectorCache[sel] = cachedSelector{FetchedAt: time.Now(), IDs: ids}
	e.selMu.Unlock()

	return ids, nil
}

func (e *Engine) handleCommandResult(ctx context.Context, m mqttinfra.Message) {
	res, err := decodeJSON[HDPCommandResult](m.Payload())
	if err != nil {
		return
	}
	if res.Schema != hdp.SchemaV1 || res.Type != "command_result" {
		return
	}
	corr := strings.TrimSpace(res.Corr)
	if corr == "" {
		return
	}
	p, err := e.repo.ConsumePendingCorr(ctx, corr)
	if err != nil {
		return
	}
	if p == nil {
		return
	}
	status := "failed"
	errMsg := res.Error
	if res.Success {
		status = "success"
		errMsg = ""
	}
	_ = e.repo.FinishRun(ctx, p.RunID, status, errMsg)
}

func matchStateTrigger(t TriggerDeviceState, state map[string]any) bool {
	key := strings.TrimSpace(t.Key)
	op := strings.ToLower(strings.TrimSpace(t.Op))
	if op == "" {
		op = "exists"
	}
	if key == "" {
		// No key means "any state message".
		return true
	}
	v, ok := state[key]
	if op == "exists" {
		return ok
	}
	if !ok {
		return false
	}

	var want any
	if len(t.Value) > 0 {
		_ = json.Unmarshal(t.Value, &want)
	}

	switch op {
	case "eq":
		return deepEqualLoose(v, want)
	case "neq":
		return !deepEqualLoose(v, want)
	case "gt", "gte", "lt", "lte":
		left, okL := toFloat(v)
		right, okR := toFloat(want)
		if !okL || !okR {
			return false
		}
		switch op {
		case "gt":
			return left > right
		case "gte":
			return left >= right
		case "lt":
			return left < right
		case "lte":
			return left <= right
		}
	}
	return false
}

func deepEqualLoose(a, b any) bool {
	// Normalize numbers (JSON unmarshalling yields float64).
	fa, oka := toFloat(a)
	fb, okb := toFloat(b)
	if oka && okb {
		return math.Abs(fa-fb) < 1e-9
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case int32:
		return float64(t), true
	case uint:
		return float64(t), true
	case uint64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		// best-effort parse
		var num json.Number = json.Number(strings.TrimSpace(t))
		f, err := num.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
