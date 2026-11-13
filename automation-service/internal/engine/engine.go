package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/cachex"
	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/PetoAdam/homenavi/shared/observability"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type Engine struct {
	repo   *dbinfra.Repository
	mq     mqttClient
	events RunEventBus

	httpClient          *http.Client
	emailServiceURL     string
	ersServiceURL       string
	deviceHubURL        string
	integrationProxyURL string

	selMu          sync.Mutex
	selectorTTL    time.Duration
	selectorCache  map[string]cachedSelector
	selectorStore  *cachex.JSONStore
	traceStore     traceContextStore
	traceSlots     chan struct{}
	traceSlotsOnce sync.Once
	stateMu        sync.Mutex
	deviceStates   map[string]map[string]any
	debounces      map[string]time.Time

	mu          sync.RWMutex
	workflows   map[uuid.UUID]dbinfra.Workflow
	defs        map[uuid.UUID]Definition
	cron        *cron.Cron
	cronEntries map[string]cron.EntryID
	cronSpecs   map[string]string

	reloadEvery     time.Duration
	mqttConnectedAt atomic.Int64
	mqttSharedGroup string
}

type mqttClient interface {
	Publish(topic string, payload []byte) error
	SubscribeWithOptions(opts mqttx.SubscriptionOptions, handler func(mqttinfra.Message)) error
}

type traceContextStore interface {
	Get(context.Context, string, any) error
	Set(context.Context, string, any, time.Duration) error
	Delete(context.Context, ...string) error
}

const (
	maxTraceStoreOperations = 16
	traceStoreTimeout       = 25 * time.Millisecond
)

func (e *Engine) tryTraceStore(ctx context.Context, operation func(context.Context)) bool {
	if ctx.Err() != nil {
		return false
	}
	e.traceSlotsOnce.Do(func() { e.traceSlots = make(chan struct{}, maxTraceStoreOperations) })
	select {
	case e.traceSlots <- struct{}{}:
	default:
		return false
	}
	operationCtx, cancel := context.WithTimeout(ctx, traceStoreTimeout)
	defer cancel()
	done := make(chan struct{}, 1)
	go func() {
		defer func() { <-e.traceSlots }()
		operation(operationCtx)
		done <- struct{}{}
	}()
	select {
	case <-done:
		return true
	case <-operationCtx.Done():
		return false
	}
}

type cachedSelector struct {
	FetchedAt time.Time
	Targets   []resolvedTarget
}

type pendingCommandTrace struct {
	DeviceID  string            `json:"device_id"`
	StartedAt time.Time         `json:"started_at"`
	Trace     *hdp.TraceContext `json:"trace"`
}

func commandTraceKey(correlation string) string {
	digest := sha256.Sum256([]byte(correlation))
	return fmt.Sprintf("automation-service:command-trace:%x", digest[:])
}

type resolvedTarget struct {
	ExternalID  string
	HDPDeviceID *uuid.UUID
}

type Options struct {
	HTTPClient          *http.Client
	EmailServiceURL     string
	ERSServiceURL       string
	DeviceHubURL        string
	IntegrationProxyURL string
	MQTTSharedGroup     string
	RunEvents           RunEventBus
	SelectorStore       *cachex.JSONStore
	TraceStore          *cachex.JSONStore
}

func New(repo *dbinfra.Repository, mq *mqttinfra.Client, opts Options) *Engine {
	c := cron.New(cron.WithSeconds())
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	client := *hc
	client.Transport = observability.InstrumentHTTPTransport(client.Transport)
	hc = &client
	eng := &Engine{
		repo:                repo,
		events:              opts.RunEvents,
		httpClient:          hc,
		emailServiceURL:     strings.TrimRight(strings.TrimSpace(opts.EmailServiceURL), "/"),
		ersServiceURL:       strings.TrimRight(strings.TrimSpace(opts.ERSServiceURL), "/"),
		deviceHubURL:        strings.TrimRight(strings.TrimSpace(opts.DeviceHubURL), "/"),
		integrationProxyURL: strings.TrimRight(strings.TrimSpace(opts.IntegrationProxyURL), "/"),
		workflows:           map[uuid.UUID]dbinfra.Workflow{},
		defs:                map[uuid.UUID]Definition{},
		cron:                c,
		cronEntries:         map[string]cron.EntryID{},
		cronSpecs:           map[string]string{},
		selectorTTL:         15 * time.Second,
		selectorCache:       map[string]cachedSelector{},
		selectorStore:       opts.SelectorStore,
		deviceStates:        map[string]map[string]any{},
		debounces:           map[string]time.Time{},
		reloadEvery:         10 * time.Second,
		mqttSharedGroup:     strings.TrimSpace(opts.MQTTSharedGroup),
	}
	if eng.events == nil {
		eng.events = NewRunEventHub()
	}
	if opts.TraceStore != nil {
		eng.traceStore = opts.TraceStore
	}
	if mq != nil {
		eng.mq = mq
	}
	eng.noteMQTTConnected(time.Now())
	if mq != nil {
		mq.AddOnConnectHandler(func() {
			eng.noteMQTTConnected(time.Now())
		})
	}
	return eng
}

func (e *Engine) selectorCacheKey(selector string) string {
	digest := sha256.Sum256([]byte(selector))
	return fmt.Sprintf("automation-service:selector:%x", digest[:])
}

func (e *Engine) noteMQTTConnected(at time.Time) {
	if e == nil {
		return
	}
	e.mqttConnectedAt.Store(at.UTC().UnixMilli())
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

	mode := mqttx.SubscriptionModeExclusive
	group := e.mqttSharedGroup
	if group != "" {
		mode = mqttx.SubscriptionModeShared
	}
	if err := e.mq.SubscribeWithOptions(mqttx.SubscriptionOptions{Topic: hdp.StatePrefix + "#", QoS: 1, Mode: mode, Group: group}, func(m mqttinfra.Message) {
		e.handleState(ctx, m)
	}); err != nil {
		return err
	}
	if err := e.mq.SubscribeWithOptions(mqttx.SubscriptionOptions{Topic: hdp.CommandResultPrefix + "#", QoS: 1, Mode: mode, Group: group}, func(m mqttinfra.Message) {
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
	if e.events != nil {
		_ = e.events.Close()
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
			e.expirePendingCommands(ctx, time.Now().UTC())
			_ = e.repo.PruneScheduledTriggerClaims(ctx, time.Now().UTC().Add(-24*time.Hour))
		}
	}
}

func (e *Engine) expirePendingCommands(ctx context.Context, now time.Time) {
	for {
		expired, more, err := e.repo.ExpirePendingCommands(ctx, now)
		if err != nil {
			slog.Warn("expire pending commands", "error", err)
			return
		}
		for _, command := range expired {
			if e.traceStore != nil {
				e.tryTraceStore(ctx, func(traceCtx context.Context) {
					_ = e.traceStore.Delete(traceCtx, commandTraceKey(command.Pending.Corr))
				})
			}
			stepStatus, errMsg := "unknown", "command outcome unknown: dispatch or result timed out"
			if command.Pending.ResultStatus != nil {
				stepStatus, errMsg = "success", command.Pending.ResultError
			}
			for _, step := range command.Steps {
				e.publishRunEvent(command.Pending.RunID, RunEvent{Type: "node_finished", WorkflowID: command.Pending.WorkflowID.String(), NodeID: step.NodeID, StepID: step.ID.String(), NodeKind: "action.send_command", Status: stepStatus})
			}
			e.publishRunEvent(command.Pending.RunID, RunEvent{Type: "run_finished", WorkflowID: command.Pending.WorkflowID.String(), Status: command.Status, Error: errMsg})
		}
		if !more {
			return
		}
	}
}

func (e *Engine) reload(ctx context.Context) error {
	rows, err := e.repo.ListWorkflows(ctx)
	if err != nil {
		return err
	}

	// Build new maps; then swap.
	newWF := map[uuid.UUID]dbinfra.Workflow{}
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
				occurredAt := time.Now().UTC().Truncate(time.Second)
				claimed, err := e.repo.ClaimScheduledTrigger(ctx, wfIDCopy, nodeIDCopy, occurredAt)
				if err != nil || !claimed {
					if err != nil {
						slog.Warn("automation schedule trigger claim failed", "workflow_id", wfIDCopy, "trigger_node_id", nodeIDCopy, "error", err)
					}
					return
				}
				allowed, err := e.repo.ClaimTriggerCooldown(ctx, wfIDCopy, nodeIDCopy, time.Duration(cooldownSec)*time.Second, occurredAt)
				if err != nil || !allowed {
					if err != nil {
						slog.Warn("automation schedule trigger cooldown claim failed", "workflow_id", wfIDCopy, "trigger_node_id", nodeIDCopy, "error", err)
					}
					return
				}
				_, _ = e.StartWorkflowRun(ctx, wfIDCopy, nodeIDCopy, map[string]any{"type": "schedule", "trigger_node_id": nodeIDCopy, "cron": cronCopy, "ts": occurredAt.UnixMilli()}, "")
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
