package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/hdp"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

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
	run := &dbinfra.WorkflowRun{WorkflowID: wfID, Status: "running", TriggerEvent: datatypes.JSON(triggerJSON), StartedAt: time.Now().UTC()}
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
		runStep := &dbinfra.WorkflowRunStep{RunID: runID, NodeID: n.ID, Status: "running", Input: datatypes.JSON(stepIn), StartedAt: time.Now().UTC()}
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
			deviceIDs, err := e.resolveTargets(ctx, a.Targets)
			if err != nil {
				finish("failed", err.Error())
				return err
			}
			if len(deviceIDs) == 0 {
				finish("success", "")
				return nil
			}
			cmdName := strings.TrimSpace(a.Command)
			if cmdName == "" {
				cmdName = "set_state"
			}

			// Publish to each target.
			baseTS := time.Now().UTC().UnixMilli()
			for _, deviceID := range deviceIDs {
				corr := fmt.Sprintf("auto-%s-%s-%s-%d", wfID.String(), n.ID, deviceID, baseTS)
				cmd := HDPCommand{Envelope: hdp.Envelope{Schema: hdp.SchemaV1, Type: "command", DeviceID: deviceID, Corr: corr, TS: baseTS}, Command: cmdName, Args: a.Args}
				b, _ := json.Marshal(cmd)
				topic := hdp.Topic(hdp.CommandPrefix, deviceID)
				if err := e.mq.Publish(topic, b); err != nil {
					finish("failed", err.Error())
					return err
				}

				if a.WaitForResult {
					timeout := a.ResultTimeoutSec
					if timeout <= 0 {
						timeout = 15
					}
					exp := time.Now().UTC().Add(time.Duration(timeout) * time.Second)
					_ = e.repo.UpsertPendingCorr(ctx, &dbinfra.PendingCorrelation{Corr: corr, RunID: runID, WorkflowID: wfID, DeviceID: deviceID, CreatedAt: time.Now().UTC(), ExpiresAt: exp})
					finish("success", "")
					return errWaitForResult
				}
			}
			finish("success", "")

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
		case "action.integration":
			var a ActionIntegration
			if err := json.Unmarshal(n.Data, &a); err != nil {
				finish("failed", "invalid node data")
				return errors.New("invalid node data")
			}
			result, err := e.executeIntegrationAction(ctx, runID, n.ID, a)
			if err != nil {
				finish("failed", err.Error())
				return err
			}
			_ = result
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
