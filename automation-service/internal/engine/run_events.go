package engine

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// RunEvent is streamed to the frontend during execution.
// It is intentionally UI-friendly (node-based) rather than engine-internal.
type RunEvent struct {
	Type             string `json:"type"`
	RunID            string `json:"run_id"`
	WorkflowID       string `json:"workflow_id,omitempty"`
	NodeID           string `json:"node_id,omitempty"`
	StepID           string `json:"step_id,omitempty"`
	NodeKind         string `json:"node_kind,omitempty"`
	Status           string `json:"status,omitempty"`
	Error            string `json:"error,omitempty"`
	TSUnixMillis     int64  `json:"ts"`
	SleepDurationSec int    `json:"sleep_duration_sec,omitempty"`
}

// RunEventHub is an in-memory pub/sub keyed by run ID.
// It keeps a small replay buffer so clients that connect slightly late
// still see early events (like the trigger highlight).
type RunEventHub struct {
	mu        sync.RWMutex
	subs      map[uuid.UUID]map[chan RunEvent]struct{}
	replay    map[uuid.UUID][]RunEvent
	maxReplay int
}

func NewRunEventHub() *RunEventHub {
	return &RunEventHub{
		subs:      map[uuid.UUID]map[chan RunEvent]struct{}{},
		replay:    map[uuid.UUID][]RunEvent{},
		maxReplay: 200,
	}
}

func (h *RunEventHub) Subscribe(runID uuid.UUID) (<-chan RunEvent, func()) {
	ch := make(chan RunEvent, 64)

	h.mu.Lock()
	if _, ok := h.subs[runID]; !ok {
		h.subs[runID] = map[chan RunEvent]struct{}{}
	}
	h.subs[runID][ch] = struct{}{}
	replay := append([]RunEvent(nil), h.replay[runID]...)
	h.mu.Unlock()

	// Best-effort replay in a goroutine so Subscribe never blocks.
	go func() {
		for _, evt := range replay {
			select {
			case ch <- evt:
			default:
				return
			}
		}
	}()

	cancel := func() {
		h.mu.Lock()
		if m, ok := h.subs[runID]; ok {
			delete(m, ch)
			if len(m) == 0 {
				delete(h.subs, runID)
				// Keep replay for a little while; it is bounded anyway.
			}
		}
		h.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

func (h *RunEventHub) Publish(runID uuid.UUID, evt RunEvent) {
	if evt.TSUnixMillis == 0 {
		evt.TSUnixMillis = time.Now().UTC().UnixMilli()
	}
	if evt.RunID == "" {
		evt.RunID = runID.String()
	}

	h.mu.Lock()
	// Append to replay buffer.
	h.replay[runID] = append(h.replay[runID], evt)
	if len(h.replay[runID]) > h.maxReplay {
		// Drop oldest.
		h.replay[runID] = h.replay[runID][len(h.replay[runID])-h.maxReplay:]
	}
	subs := make([]chan RunEvent, 0, len(h.subs[runID]))
	for ch := range h.subs[runID] {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	// Fan-out without blocking the engine.
	for _, ch := range subs {
		select {
		case ch <- evt:
		default:
			// Drop if subscriber is slow.
		}
	}
}
