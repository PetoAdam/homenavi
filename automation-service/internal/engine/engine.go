package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/hdp"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type Engine struct {
	repo   *dbinfra.Repository
	mq     *mqttinfra.Client
	events *RunEventHub

	httpClient          *http.Client
	emailServiceURL     string
	ersServiceURL       string
	integrationProxyURL string

	selMu         sync.Mutex
	selectorTTL   time.Duration
	selectorCache map[string]cachedSelector

	mu          sync.RWMutex
	workflows   map[uuid.UUID]dbinfra.Workflow
	defs        map[uuid.UUID]Definition
	lastFiredAt map[string]time.Time

	cron        *cron.Cron
	cronEntries map[string]cron.EntryID
	cronSpecs   map[string]string

	reloadEvery time.Duration
}

type cachedSelector struct {
	FetchedAt time.Time
	IDs       []string
}

type Options struct {
	HTTPClient          *http.Client
	EmailServiceURL     string
	ERSServiceURL       string
	IntegrationProxyURL string
}

func New(repo *dbinfra.Repository, mq *mqttinfra.Client, opts Options) *Engine {
	c := cron.New(cron.WithSeconds())
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Engine{
		repo:                repo,
		mq:                  mq,
		events:              NewRunEventHub(),
		httpClient:          hc,
		emailServiceURL:     strings.TrimRight(strings.TrimSpace(opts.EmailServiceURL), "/"),
		ersServiceURL:       strings.TrimRight(strings.TrimSpace(opts.ERSServiceURL), "/"),
		integrationProxyURL: strings.TrimRight(strings.TrimSpace(opts.IntegrationProxyURL), "/"),
		workflows:           map[uuid.UUID]dbinfra.Workflow{},
		defs:                map[uuid.UUID]Definition{},
		lastFiredAt:         map[string]time.Time{},
		cron:                c,
		cronEntries:         map[string]cron.EntryID{},
		cronSpecs:           map[string]string{},
		selectorTTL:         15 * time.Second,
		selectorCache:       map[string]cachedSelector{},
		reloadEvery:         10 * time.Second,
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
	if err := e.mq.Subscribe(hdp.StatePrefix+"#", func(m mqttinfra.Message) {
		e.handleState(ctx, m)
	}); err != nil {
		return err
	}
	if err := e.mq.Subscribe(hdp.CommandResultPrefix+"#", func(m mqttinfra.Message) {
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
