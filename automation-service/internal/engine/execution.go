package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/observability"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func normalizeSleepDurationSeconds(seconds float64) float64 {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return 0
	}
	return seconds
}

func sleepDuration(seconds float64) time.Duration {
	return time.Duration(normalizeSleepDurationSeconds(seconds) * float64(time.Second))
}

func (e *Engine) StartWorkflowRun(ctx context.Context, wfID uuid.UUID, triggerNodeID string, triggerEvent map[string]any, idempotencyKey string) (uuid.UUID, error) {
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
	var idempotencyKeyValue *string
	if key := strings.TrimSpace(idempotencyKey); key != "" {
		idempotencyKeyValue = &key
	}
	run := &dbinfra.WorkflowRun{WorkflowID: wfID, IdempotencyKey: idempotencyKeyValue, Status: "running", TriggerEvent: datatypes.JSON(triggerJSON), StartedAt: time.Now().UTC()}
	if err := e.repo.CreateRun(ctx, run); err != nil {
		slog.Warn("create run failed", "error", err)
		return uuid.Nil, err
	}

	// Announce run start (WS clients may connect right after receiving run_id).
	e.publishRunEvent(run.ID, RunEvent{Type: "run_started", WorkflowID: wfID.String(), Status: "running"})

	// Run asynchronously so HTTP can return immediately.
	defCopy := d
	parent := trace.SpanContextFromContext(ctx)
	go func() {
		runContext, span := startWorkflowExecution(parent)
		defer span.End()
		e.executeRun(runContext, run.ID, wfID, triggerNodeID, triggerEvent, defCopy, nodeByID, outgoing, start)
	}()

	return run.ID, nil
}

func startWorkflowExecution(parent trace.SpanContext) (context.Context, trace.Span) {
	ctx := trace.ContextWithSpanContext(context.Background(), parent)
	return otel.Tracer("homenavi/automation").Start(ctx, "workflow.execute")
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
		if err := e.repo.CreateStep(ctx, runStep); err != nil {
			return fmt.Errorf("create workflow step: %w", err)
		}

		startedEvt := RunEvent{Type: "node_started", WorkflowID: wfID.String(), NodeID: n.ID, StepID: runStep.ID.String(), NodeKind: kind, Status: "running"}
		if kind == "logic.sleep" {
			var sl LogicSleep
			if err := json.Unmarshal(n.Data, &sl); err == nil {
				sl.DurationSec = normalizeSleepDurationSeconds(sl.DurationSec)
				startedEvt.Status = "sleeping"
				startedEvt.SleepDurationSec = sl.DurationSec
			}
		}
		e.publishRunEvent(runID, startedEvt)

		finish := func(status string, errMsg string) error {
			if err := e.repo.FinishStep(ctx, runStep.ID, status, errMsg); err != nil {
				return fmt.Errorf("finish workflow step %s: %w", runStep.ID, err)
			}
			e.publishRunEvent(runID, RunEvent{Type: "node_finished", WorkflowID: wfID.String(), NodeID: n.ID, StepID: runStep.ID.String(), NodeKind: kind, Status: status, Error: errMsg})
			return nil
		}
		fail := func(err error) error {
			return errors.Join(err, finish("failed", err.Error()))
		}

		switch kind {
		case "action.send_command":
			var a ActionSendCommand
			if err := json.Unmarshal(n.Data, &a); err != nil {
				return fail(errors.New("invalid node data"))
			}
			targets, err := e.resolveTargets(ctx, a.Targets)
			if err != nil {
				return fail(err)
			}
			if len(targets) == 0 {
				return finish("success", "")
			}
			cmdName := strings.TrimSpace(a.Command)
			if cmdName == "" {
				cmdName = "set_state"
			}

			// Publish to each target.
			baseTS := time.Now().UTC().UnixMilli()
			for _, target := range targets {
				deviceID := target.ExternalID
				corr := fmt.Sprintf("auto-%s-%s-%s-%d", wfID.String(), n.ID, deviceID, baseTS)
				if a.WaitForResult {
					timeout := a.ResultTimeoutSec
					if timeout <= 0 {
						timeout = 15
					}
					now := time.Now().UTC()
					pending := &dbinfra.PendingCorrelation{Corr: corr, RunID: runID, WorkflowID: wfID, DeviceID: deviceID, HDPDeviceID: target.HDPDeviceID, DispatchPending: true, CreatedAt: now, ExpiresAt: now.Add(time.Duration(timeout) * time.Second)}
					if err := dispatchWithPendingCorrelation(ctx, e.repo, pending, func() error {
						return e.dispatchCommand(ctx, deviceID, corr, cmdName, a)
					}); err != nil {
						if errors.Is(err, errCommandOutcomeUnknown) {
							return errWaitForResult
						}
						return fail(err)
					}
					completed, err := retryPendingDispatch(ctx, time.Second, func() (*dbinfra.PendingCorrelation, error) {
						return e.repo.FinishPendingDispatch(ctx, corr, runStep.ID, func() {
							e.publishRunEvent(runID, RunEvent{Type: "node_finished", WorkflowID: wfID.String(), NodeID: n.ID, StepID: runStep.ID.String(), NodeKind: kind, Status: "success"})
							e.publishRunEvent(runID, RunEvent{Type: "run_waiting", WorkflowID: wfID.String(), Status: "waiting"})
						})
					})
					if err != nil {
						return errWaitForResult
					}
					e.publishCompletedCommand(ctx, completed)
					return errWaitForResult
				}
				if err := e.dispatchCommand(ctx, deviceID, corr, cmdName, a); err != nil {
					return fail(err)
				}
			}
			if err := finish("success", ""); err != nil {
				return err
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
				return fail(errors.New("invalid node data"))
			}
			if err := e.executeNotifyEmail(ctx, a); err != nil {
				return fail(err)
			}
			if err := finish("success", ""); err != nil {
				return err
			}
			for _, next := range outgoing[n.ID] {
				if err := exec(next); err != nil {
					return err
				}
			}
			return nil
		case "action.integration":
			var a ActionIntegration
			if err := json.Unmarshal(n.Data, &a); err != nil {
				return fail(errors.New("invalid node data"))
			}
			result, err := e.executeIntegrationAction(ctx, runID, n.ID, a)
			if err != nil {
				return fail(err)
			}
			_ = result
			if err := finish("success", ""); err != nil {
				return err
			}
			for _, next := range outgoing[n.ID] {
				if err := exec(next); err != nil {
					return err
				}
			}
			return nil
		case "logic.sleep":
			var s LogicSleep
			if err := json.Unmarshal(n.Data, &s); err != nil {
				return fail(errors.New("invalid node data"))
			}
			time.Sleep(sleepDuration(s.DurationSec))
			if err := finish("success", ""); err != nil {
				return err
			}
			for _, next := range outgoing[n.ID] {
				if err := exec(next); err != nil {
					return err
				}
			}
			return nil
		case "logic.if":
			var c LogicIf
			if err := json.Unmarshal(n.Data, &c); err != nil {
				return fail(errors.New("invalid node data"))
			}
			ok := evalIf(triggerEvent, c.Path, c.Op, c.Value)
			if err := finish("success", ""); err != nil {
				return err
			}
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
				return fail(errors.New("invalid node data"))
			}
			if f.Count < 0 {
				f.Count = 0
			}
			if err := finish("success", ""); err != nil {
				return err
			}
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
			return fail(fmt.Errorf("unsupported node kind: %s", n.Kind))
		}
	}

	for _, nodeID := range start {
		if err := exec(nodeID); err != nil {
			if errors.Is(err, errWaitForResult) {
				return
			}
			e.failWorkflowRun(ctx, runID, wfID, err)
			return
		}
	}

	if err := e.repo.FinishRun(ctx, runID, "success", ""); err != nil {
		e.failWorkflowRun(ctx, runID, wfID, fmt.Errorf("finish workflow run: %w", err))
		return
	}
	e.publishRunEvent(runID, RunEvent{Type: "run_finished", WorkflowID: wfID.String(), Status: "success"})
}

func (e *Engine) failWorkflowRun(ctx context.Context, runID, workflowID uuid.UUID, cause error) {
	delay := time.Second
	for {
		steps, finished, err := e.repo.FailRun(ctx, runID, cause.Error())
		if err == nil {
			for _, step := range steps {
				e.publishRunEvent(runID, RunEvent{Type: "node_finished", WorkflowID: workflowID.String(), NodeID: step.NodeID, StepID: step.ID.String(), Status: "failed", Error: cause.Error()})
			}
			if finished {
				e.publishRunEvent(runID, RunEvent{Type: "run_finished", WorkflowID: workflowID.String(), Status: "failed", Error: cause.Error()})
			}
			return
		}
		slog.Warn("persist failed workflow run", "run_id", runID, "error", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if delay < 30*time.Second {
			delay = min(delay*2, 30*time.Second)
		}
	}
}

type pendingCorrelationRepository interface {
	UpsertPendingCorr(context.Context, *dbinfra.PendingCorrelation) error
	ConsumePendingCorr(context.Context, string) (*dbinfra.PendingCorrelation, error)
}

var errCommandOutcomeUnknown = errors.New("command outcome unknown")

func retryPendingDispatch(ctx context.Context, delay time.Duration, finish func() (*dbinfra.PendingCorrelation, error)) (*dbinfra.PendingCorrelation, error) {
	for {
		completed, err := finish()
		if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
			return completed, err
		}
		slog.Warn("finalize command dispatch failed; retrying", "error", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		if delay < 30*time.Second {
			delay = min(delay*2, 30*time.Second)
		}
	}
}

func dispatchWithPendingCorrelation(ctx context.Context, repo pendingCorrelationRepository, pending *dbinfra.PendingCorrelation, dispatch func() error) error {
	if err := repo.UpsertPendingCorr(ctx, pending); err != nil {
		return fmt.Errorf("record pending command: %w", err)
	}
	if err := dispatch(); err != nil {
		if !errors.Is(err, errCommandOutcomeUnknown) {
			_, _ = repo.ConsumePendingCorr(ctx, pending.Corr)
		}
		return err
	}
	return nil
}

func (e *Engine) dispatchCommand(ctx context.Context, deviceID, correlationID, command string, action ActionSendCommand) error {
	if strings.TrimSpace(action.CapabilityID) != "" {
		if command != "set_state" {
			return errors.New("capability-targeted actions only support set_state")
		}
		return e.dispatchCapabilityStatePatch(ctx, deviceID, correlationID, action.Args)
	}
	if e.mq == nil {
		return errors.New("mqtt client unavailable")
	}
	ctx, span := observability.StartMessageProducer(ctx)
	defer span.End()
	cmd := HDPCommand{Envelope: hdp.Envelope{Schema: hdp.SchemaV1, Type: "command", DeviceID: deviceID, Corr: correlationID, TS: time.Now().UTC().UnixMilli()}, Command: command, Args: action.Args}
	if observability.MessagePropagationEnabled() {
		cmd.Trace = observability.CaptureTraceContext(ctx)
	}
	payload, err := json.Marshal(cmd)
	if err != nil {
		span.SetStatus(codes.Error, "MQTT message encoding failed")
		return fmt.Errorf("marshal command: %w", err)
	}
	observability.DescribeHDPMessage(span, "publish", payload, false, hdp.Topic(hdp.CommandPrefix, deviceID))
	if action.WaitForResult && e.traceStore != nil {
		if metadata := observability.CaptureTraceContext(ctx); metadata != nil {
			e.tryTraceStore(ctx, func(traceCtx context.Context) {
				_ = e.traceStore.Set(traceCtx, commandTraceKey(correlationID), pendingCommandTrace{DeviceID: deviceID, StartedAt: time.UnixMilli(cmd.TS), Trace: metadata}, observability.DeferredTraceMaxAge)
			})
		}
	}
	if err := e.mq.Publish(hdp.Topic(hdp.CommandPrefix, deviceID), payload); err != nil {
		if action.WaitForResult && e.traceStore != nil {
			e.tryTraceStore(ctx, func(traceCtx context.Context) { _ = e.traceStore.Delete(traceCtx, commandTraceKey(correlationID)) })
		}
		span.SetStatus(codes.Error, "MQTT publish failed")
		return fmt.Errorf("%w: mqtt publish failed: %v", errCommandOutcomeUnknown, err)
	}
	return nil
}

func (e *Engine) dispatchCapabilityStatePatch(ctx context.Context, deviceID, correlationID string, state map[string]any) error {
	if e.deviceHubURL == "" {
		return errors.New("device hub url is not configured")
	}
	payload, err := json.Marshal(map[string]any{"state": state, "correlation_id": correlationID})
	if err != nil {
		return fmt.Errorf("marshal device state patch: %w", err)
	}
	endpoint := e.deviceHubURL + "/api/hdp/devices/" + url.PathEscape(deviceID) + "/commands"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build device hub command request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: dispatch through device hub: %v", errCommandOutcomeUnknown, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("%w: device hub rejected command: %s", errCommandOutcomeUnknown, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("device hub rejected command: %s", strings.TrimSpace(string(body)))
	}
	return nil
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
	return e.RunWorkflowNowWithIdempotency(ctx, wfID, "")
}

func (e *Engine) RunWorkflowNowWithIdempotency(ctx context.Context, wfID uuid.UUID, idempotencyKey string) (uuid.UUID, error) {
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
	return e.StartWorkflowRun(ctx, wfID, manualNodeID, map[string]any{"type": "manual", "trigger_node_id": manualNodeID, "ts": time.Now().UTC().UnixMilli()}, idempotencyKey)
}
