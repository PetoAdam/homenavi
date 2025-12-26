package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"automation-service/internal/mqtt"
	"automation-service/internal/store"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/datatypes"
)

type Engine struct {
	repo   *store.Repo
	mq     *mqtt.Client
	events *RunEventHub

	httpClient      *http.Client
	emailServiceURL string

	mu          sync.RWMutex
	workflows   map[uuid.UUID]store.Workflow
	defs        map[uuid.UUID]Definition
	lastFiredAt map[string]time.Time

	cron        *cron.Cron
	cronEntries map[string]cron.EntryID
	cronSpecs   map[string]string

	reloadEvery time.Duration
}

type Options struct {
	HTTPClient      *http.Client
	EmailServiceURL string
}

func New(repo *store.Repo, mq *mqtt.Client, opts Options) *Engine {
	c := cron.New(cron.WithSeconds())
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Engine{
		repo:            repo,
		mq:              mq,
		events:          NewRunEventHub(),
		httpClient:      hc,
		emailServiceURL: strings.TrimRight(strings.TrimSpace(opts.EmailServiceURL), "/"),
		workflows:       map[uuid.UUID]store.Workflow{},
		defs:            map[uuid.UUID]Definition{},
		lastFiredAt:     map[string]time.Time{},
		cron:            c,
		cronEntries:     map[string]cron.EntryID{},
		cronSpecs:       map[string]string{},
		reloadEvery:     10 * time.Second,
	}
}

func (e *Engine) SubscribeRunEvents(runID uuid.UUID) (<-chan RunEvent, func()) {
	if e.events == nil {
		ch := make(chan RunEvent)
		close(ch)
		return ch, func() {}
	}
	return e.events.Subscribe(runID)
}

func (e *Engine) publishRunEvent(runID uuid.UUID, evt RunEvent) {
	if e.events == nil {
		return
	}
	e.events.Publish(runID, evt)
}

func (e *Engine) Start(ctx context.Context) error {
	if err := e.reload(ctx); err != nil {
		return err
	}
	e.cron.Start()

	// Subscribe to HDP state + command_result.
	if err := e.mq.Subscribe("homenavi/hdp/device/state/#", func(m mqtt.Message) {
		e.handleState(ctx, m)
	}); err != nil {
		return err
	}
	if err := e.mq.Subscribe("homenavi/hdp/device/command_result/#", func(m mqtt.Message) {
		e.handleCommandResult(ctx, m)
	}); err != nil {
		return err
	}

	go e.reloadLoop(ctx)
	go e.pruneLoop(ctx)
	return nil
}

// ReloadNow refreshes workflow definitions from the database immediately.
// This is used by HTTP handlers so updates take effect without waiting for the periodic reload loop.
func (e *Engine) ReloadNow(ctx context.Context) error {
	return e.reload(ctx)
}

func (e *Engine) Stop() {
	if e.cron != nil {
		e.cron.Stop()
	}
}

func (e *Engine) reloadLoop(ctx context.Context) {
	t := time.NewTicker(e.reloadEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := e.reload(ctx); err != nil {
				slog.Warn("automation reload failed", "error", err)
			}
		}
	}
}

func (e *Engine) pruneLoop(ctx context.Context) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = e.repo.PruneExpiredPending(ctx)
		}
	}
}

func (e *Engine) reload(ctx context.Context) error {
	rows, err := e.repo.ListWorkflows(ctx)
	if err != nil {
		return err
	}

	// Build new maps; then swap.
	newWF := map[uuid.UUID]store.Workflow{}
	newDefs := map[uuid.UUID]Definition{}

	for _, w := range rows {
		newWF[w.ID] = w
		var d Definition
		if err := json.Unmarshal([]byte(w.Definition), &d); err != nil {
			slog.Warn("invalid workflow definition", "workflow_id", w.ID, "error", err)
			continue
		}
		if err := d.NormalizeAndValidate(); err != nil {
			slog.Warn("invalid workflow definition", "workflow_id", w.ID, "error", err)
			continue
		}
		newDefs[w.ID] = d
	}

	e.mu.Lock()
	e.workflows = newWF
	e.defs = newDefs
	e.mu.Unlock()

	// Reconcile cron schedules for enabled schedule triggers.
	e.reconcileCron()
	return nil
}

func (e *Engine) reconcileCron() {
	e.mu.Lock()
	defer e.mu.Unlock()

	expected := map[string]struct{}{}

	for wfID, w := range e.workflows {
		if !w.Enabled {
			continue
		}
		d, ok := e.defs[wfID]
		if !ok {
			continue
		}
		for _, n := range d.Nodes {
			if strings.ToLower(strings.TrimSpace(n.Kind)) != "trigger.schedule" {
				continue
			}
			var t TriggerSchedule
			if err := json.Unmarshal(n.Data, &t); err != nil {
				continue
			}
			cronExpr := strings.TrimSpace(t.Cron)
			if cronExpr == "" {
				continue
			}
			key := wfID.String() + ":" + n.ID
			expected[key] = struct{}{}
			// If schedule changed, recreate.
			if old, ok := e.cronSpecs[key]; ok && old != cronExpr {
				if entryID, okE := e.cronEntries[key]; okE {
					e.cron.Remove(entryID)
					delete(e.cronEntries, key)
				}
				delete(e.cronSpecs, key)
			}
			if _, exists := e.cronEntries[key]; exists {
				continue
			}

			wfIDCopy := wfID
			nodeIDCopy := n.ID
			cronCopy := cronExpr
			cooldownSec := t.CooldownSec
			id, err := e.cron.AddFunc(cronExpr, func() {
				ctx := context.Background()
				coolKey := wfIDCopy.String() + ":" + nodeIDCopy
				if !e.allowFire(coolKey, cooldownSec) {
					return
				}
				_, _ = e.StartWorkflowRun(ctx, wfIDCopy, nodeIDCopy, map[string]any{"type": "schedule", "trigger_node_id": nodeIDCopy, "cron": cronCopy, "ts": time.Now().UTC().UnixMilli()})
			})
			if err != nil {
				slog.Warn("invalid cron expression", "workflow_id", wfID, "trigger_node_id", n.ID, "cron", cronExpr, "error", err)
				continue
			}
			e.cronEntries[key] = id
			e.cronSpecs[key] = cronExpr
		}
	}

	// Remove stale entries.
	for key, entryID := range e.cronEntries {
		if _, ok := expected[key]; ok {
			continue
		}
		e.cron.Remove(entryID)
		delete(e.cronEntries, key)
		delete(e.cronSpecs, key)
	}
}

func (e *Engine) handleState(ctx context.Context, m mqtt.Message) {
	payload := m.Payload()
	st, err := decodeJSON[HDPState](payload)
	if err != nil {
		return
	}
	if st.Schema != "hdp.v1" || st.Type != "state" {
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
			if strings.TrimSpace(t.DeviceID) != st.DeviceID {
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

func (e *Engine) handleCommandResult(ctx context.Context, m mqtt.Message) {
	res, err := decodeJSON[HDPCommandResult](m.Payload())
	if err != nil {
		return
	}
	if res.Schema != "hdp.v1" || res.Type != "command_result" {
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

func (e *Engine) allowFire(key string, cooldownSec int) bool {
	if cooldownSec <= 0 {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	last, ok := e.lastFiredAt[key]
	if ok && time.Since(last) < time.Duration(cooldownSec)*time.Second {
		return false
	}
	e.lastFiredAt[key] = time.Now()
	return true
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

func (e *Engine) StartWorkflowRun(ctx context.Context, wfID uuid.UUID, triggerNodeID string, triggerEvent map[string]any) (uuid.UUID, error) {
	e.mu.RLock()
	w, okW := e.workflows[wfID]
	d, okD := e.defs[wfID]
	e.mu.RUnlock()
	if !okW {
		return uuid.Nil, errors.New("workflow not found")
	}
	if !w.Enabled {
		return uuid.Nil, errors.New("workflow disabled")
	}
	if !okD {
		return uuid.Nil, errors.New("workflow definition not loaded")
	}

	nodeByID := map[string]NodeDef{}
	for _, n := range d.Nodes {
		nodeByID[n.ID] = n
	}
	outgoing := buildOutgoing(d.Edges)
	start := outgoing[triggerNodeID]
	if len(start) == 0 {
		return uuid.Nil, errors.New("trigger has no outgoing edges")
	}

	triggerJSON, _ := json.Marshal(triggerEvent)
	run := &store.WorkflowRun{WorkflowID: wfID, Status: "running", TriggerEvent: datatypes.JSON(triggerJSON), StartedAt: time.Now().UTC()}
	if err := e.repo.CreateRun(ctx, run); err != nil {
		slog.Warn("create run failed", "error", err)
		return uuid.Nil, err
	}

	// Announce run start (WS clients may connect right after receiving run_id).
	e.publishRunEvent(run.ID, RunEvent{Type: "run_started", WorkflowID: wfID.String(), Status: "running"})

	// Run asynchronously so HTTP can return immediately.
	defCopy := d
	go e.executeRun(context.Background(), run.ID, wfID, triggerNodeID, triggerEvent, defCopy, nodeByID, outgoing, start)

	return run.ID, nil
}

func (e *Engine) executeRun(ctx context.Context, runID uuid.UUID, wfID uuid.UUID, triggerNodeID string, triggerEvent map[string]any, d Definition, nodeByID map[string]NodeDef, outgoing map[string][]string, start []string) {
	errWaitForResult := errors.New("wait_for_result")

	emitTrigger := func(nodeID string) {
		n, ok := nodeByID[nodeID]
		kind := "trigger"
		if ok {
			kind = strings.ToLower(strings.TrimSpace(n.Kind))
		}
		e.publishRunEvent(runID, RunEvent{Type: "node_started", WorkflowID: wfID.String(), NodeID: nodeID, NodeKind: kind, Status: "running"})
		e.publishRunEvent(runID, RunEvent{Type: "node_finished", WorkflowID: wfID.String(), NodeID: nodeID, NodeKind: kind, Status: "success"})
	}

	// Always highlight the trigger that fired.
	emitTrigger(triggerNodeID)

	var exec func(nodeID string) error
	exec = func(nodeID string) error {
		n, ok := nodeByID[nodeID]
		if !ok {
			return fmt.Errorf("unknown node: %s", nodeID)
		}
		kind := strings.ToLower(strings.TrimSpace(n.Kind))
		// Triggers mid-graph: highlight them too and treat as pass-through.
		if strings.HasPrefix(kind, "trigger.") {
			emitTrigger(n.ID)
			for _, next := range outgoing[n.ID] {
				if err := exec(next); err != nil {
					return err
				}
			}
			return nil
		}

		stepIn, _ := json.Marshal(map[string]any{"node": n})
		runStep := &store.WorkflowRunStep{RunID: runID, NodeID: n.ID, Status: "running", Input: datatypes.JSON(stepIn), StartedAt: time.Now().UTC()}
		_ = e.repo.CreateStep(ctx, runStep)

		startedEvt := RunEvent{Type: "node_started", WorkflowID: wfID.String(), NodeID: n.ID, StepID: runStep.ID.String(), NodeKind: kind, Status: "running"}
		if kind == "logic.sleep" {
			var sl LogicSleep
			if err := json.Unmarshal(n.Data, &sl); err == nil {
				if sl.DurationSec < 0 {
					sl.DurationSec = 0
				}
				startedEvt.Status = "sleeping"
				startedEvt.SleepDurationSec = sl.DurationSec
			}
		}
		e.publishRunEvent(runID, startedEvt)

		finish := func(status string, errMsg string) {
			_ = e.repo.FinishStep(ctx, runStep.ID, status, errMsg)
			e.publishRunEvent(runID, RunEvent{Type: "node_finished", WorkflowID: wfID.String(), NodeID: n.ID, StepID: runStep.ID.String(), NodeKind: kind, Status: status, Error: errMsg})
		}

		switch kind {
		case "action.send_command":
			var a ActionSendCommand
			if err := json.Unmarshal(n.Data, &a); err != nil {
				finish("failed", "invalid node data")
				return errors.New("invalid node data")
			}
			deviceID := strings.TrimSpace(a.DeviceID)
			if deviceID == "" {
				finish("failed", "device_id required")
				return errors.New("device_id required")
			}
			cmdName := strings.TrimSpace(a.Command)
			if cmdName == "" {
				cmdName = "set_state"
			}
			corr := fmt.Sprintf("auto-%s-%s-%d", wfID.String(), n.ID, time.Now().UTC().UnixMilli())
			cmd := HDPCommand{Schema: "hdp.v1", Type: "command", DeviceID: deviceID, Command: cmdName, Args: a.Args, Corr: corr, TS: time.Now().UTC().UnixMilli()}
			b, _ := json.Marshal(cmd)
			topic := "homenavi/hdp/device/command/" + deviceID
			if err := e.mq.Publish(topic, b); err != nil {
				finish("failed", err.Error())
				return err
			}
			finish("success", "")

			if a.WaitForResult {
				timeout := a.ResultTimeoutSec
				if timeout <= 0 {
					timeout = 15
				}
				exp := time.Now().UTC().Add(time.Duration(timeout) * time.Second)
				_ = e.repo.UpsertPendingCorr(ctx, &store.PendingCorrelation{Corr: corr, RunID: runID, WorkflowID: wfID, DeviceID: deviceID, CreatedAt: time.Now().UTC(), ExpiresAt: exp})
				return errWaitForResult
			}

			for _, next := range outgoing[n.ID] {
				if err := exec(next); err != nil {
					return err
				}
			}
			return nil
		case "action.notify_email":
			var a ActionNotifyEmail
			if err := json.Unmarshal(n.Data, &a); err != nil {
				finish("failed", "invalid node data")
				return errors.New("invalid node data")
			}
			if err := e.executeNotifyEmail(ctx, a); err != nil {
				finish("failed", err.Error())
				return err
			}
			finish("success", "")
			for _, next := range outgoing[n.ID] {
				if err := exec(next); err != nil {
					return err
				}
			}
			return nil
		case "logic.sleep":
			var s LogicSleep
			if err := json.Unmarshal(n.Data, &s); err != nil {
				finish("failed", "invalid node data")
				return errors.New("invalid node data")
			}
			if s.DurationSec < 0 {
				s.DurationSec = 0
			}
			time.Sleep(time.Duration(s.DurationSec) * time.Second)
			finish("success", "")
			for _, next := range outgoing[n.ID] {
				if err := exec(next); err != nil {
					return err
				}
			}
			return nil
		case "logic.if":
			var c LogicIf
			if err := json.Unmarshal(n.Data, &c); err != nil {
				finish("failed", "invalid node data")
				return errors.New("invalid node data")
			}
			ok := evalIf(triggerEvent, c.Path, c.Op, c.Value)
			finish("success", "")
			outs := outgoing[n.ID]
			if ok {
				if len(outs) >= 1 {
					return exec(outs[0])
				}
				return nil
			}
			if len(outs) >= 2 {
				return exec(outs[1])
			}
			return nil
		case "logic.for":
			var f LogicFor
			if err := json.Unmarshal(n.Data, &f); err != nil {
				finish("failed", "invalid node data")
				return errors.New("invalid node data")
			}
			if f.Count < 0 {
				f.Count = 0
			}
			finish("success", "")
			outs := outgoing[n.ID]
			body := ""
			after := ""
			if len(outs) >= 1 {
				body = outs[0]
			}
			if len(outs) >= 2 {
				after = outs[1]
			}
			for i := 0; i < f.Count; i++ {
				if body == "" {
					break
				}
				if err := exec(body); err != nil {
					return err
				}
			}
			if after != "" {
				return exec(after)
			}
			return nil
		default:
			finish("failed", "unsupported node kind")
			return fmt.Errorf("unsupported node kind: %s", n.Kind)
		}
	}

	for _, nodeID := range start {
		if err := exec(nodeID); err != nil {
			if errors.Is(err, errWaitForResult) {
				e.publishRunEvent(runID, RunEvent{Type: "run_waiting", WorkflowID: wfID.String(), Status: "waiting"})
				return
			}
			_ = e.repo.FinishRun(ctx, runID, "failed", err.Error())
			e.publishRunEvent(runID, RunEvent{Type: "run_finished", WorkflowID: wfID.String(), Status: "failed", Error: err.Error()})
			return
		}
	}

	_ = e.repo.FinishRun(ctx, runID, "success", "")
	e.publishRunEvent(runID, RunEvent{Type: "run_finished", WorkflowID: wfID.String(), Status: "success"})
}

func evalIf(triggerEvent map[string]any, path string, op string, raw json.RawMessage) bool {
	path = strings.TrimSpace(path)
	op = strings.ToLower(strings.TrimSpace(op))
	if op == "" {
		op = "exists"
	}
	if path == "" {
		return true
	}

	// Very small dot-path accessor (e.g. "state.motion").
	var cur any = triggerEvent
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			cur = nil
			break
		}
		cur, ok = m[part]
		if !ok {
			cur = nil
			break
		}
	}

	if op == "exists" {
		return cur != nil
	}
	if cur == nil {
		return false
	}

	var want any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &want)
	}

	switch op {
	case "eq":
		return deepEqualLoose(cur, want)
	case "neq":
		return !deepEqualLoose(cur, want)
	case "gt", "gte", "lt", "lte":
		lc, okL := toFloat(cur)
		rc, okR := toFloat(want)
		if !okL || !okR {
			return false
		}
		switch op {
		case "gt":
			return lc > rc
		case "gte":
			return lc >= rc
		case "lt":
			return lc < rc
		case "lte":
			return lc <= rc
		}
	}
	return false
}

func (e *Engine) RunWorkflowNow(ctx context.Context, wfID uuid.UUID) (uuid.UUID, error) {
	e.mu.RLock()
	w, ok := e.workflows[wfID]
	d, okD := e.defs[wfID]
	e.mu.RUnlock()
	if !ok {
		return uuid.Nil, errors.New("workflow not found")
	}
	if !w.Enabled {
		return uuid.Nil, errors.New("workflow disabled")
	}
	if !okD {
		return uuid.Nil, errors.New("workflow definition not loaded")
	}
	manualNodeID := ""
	for _, n := range d.Nodes {
		if strings.ToLower(strings.TrimSpace(n.Kind)) == "trigger.manual" {
			manualNodeID = n.ID
			break
		}
	}
	if manualNodeID == "" {
		return uuid.Nil, errors.New("no manual trigger")
	}
	return e.StartWorkflowRun(ctx, wfID, manualNodeID, map[string]any{"type": "manual", "trigger_node_id": manualNodeID, "ts": time.Now().UTC().UnixMilli()})
}
