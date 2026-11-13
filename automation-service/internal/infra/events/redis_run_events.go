package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/PetoAdam/homenavi/automation-service/internal/engine"
	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	runEventReplayLimit  = int64(200)
	runEventReplayTTL    = 15 * time.Minute
	runEventPollInterval = 100 * time.Millisecond
	runEventQueueLimit   = 256
)

// RedisRunEventBus stores a bounded replay list in Redis so all automation
// replicas can serve run-event WebSockets without Redis Pub/Sub.
type RedisRunEventBus struct {
	client    redis.UniversalClient
	queue     chan queuedRunEvent
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	closeErr  error
}

type queuedRunEvent struct {
	runID uuid.UUID
	event engine.RunEvent
}

func NewRedisRunEventBus(ctx context.Context, cfg redisx.Config) (*RedisRunEventBus, error) {
	client, err := redisx.Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	bus := &RedisRunEventBus{client: client, queue: make(chan queuedRunEvent, runEventQueueLimit), ctx: workerCtx, cancel: cancel}
	go bus.publishLoop()
	return bus, nil
}

func runEventReplayKey(runID uuid.UUID) string {
	return "homenavi:automation:run:" + runID.String() + ":replay"
}

func (b *RedisRunEventBus) Publish(runID uuid.UUID, evt engine.RunEvent) {
	if b == nil || b.client == nil || runID == uuid.Nil {
		return
	}
	select {
	case <-b.ctx.Done():
	case b.queue <- queuedRunEvent{runID: runID, event: evt}:
	default:
	}
}

func (b *RedisRunEventBus) publishLoop() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case queued := <-b.queue:
			b.storeEvent(queued.runID, queued.event)
		}
	}
}

func (b *RedisRunEventBus) storeEvent(runID uuid.UUID, evt engine.RunEvent) {
	if evt.EventID == "" {
		evt.EventID = uuid.NewString()
	}
	if evt.TSUnixMillis == 0 {
		evt.TSUnixMillis = time.Now().UTC().UnixMilli()
	}
	if evt.RunID == "" {
		evt.RunID = runID.String()
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		slog.Warn("automation run event encode failed", "run_id", runID, "error", err)
		return
	}

	pipeline := b.client.TxPipeline()
	pipeline.RPush(b.ctx, runEventReplayKey(runID), payload)
	pipeline.LTrim(b.ctx, runEventReplayKey(runID), -runEventReplayLimit, -1)
	pipeline.Expire(b.ctx, runEventReplayKey(runID), runEventReplayTTL)
	if _, err := pipeline.Exec(b.ctx); err != nil && b.ctx.Err() == nil {
		slog.Warn("automation run event store failed", "run_id", runID, "error", err)
	}
}

func (b *RedisRunEventBus) Subscribe(runID uuid.UUID) (<-chan engine.RunEvent, func()) {
	ch := make(chan engine.RunEvent, 64)
	if b == nil || b.client == nil || runID == uuid.Nil {
		close(ch)
		return ch, func() {}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(ch)

		seen := make(map[string]struct{}, runEventReplayLimit)
		seenOrder := make([]string, 0, runEventReplayLimit)
		emit := func(payload string) bool {
			var evt engine.RunEvent
			if err := json.Unmarshal([]byte(payload), &evt); err != nil {
				slog.Debug("automation run event decode failed", "run_id", runID, "error", err)
				return true
			}
			key := evt.EventID
			if key == "" {
				key = payload
			}
			if _, ok := seen[key]; ok {
				return true
			}
			seen[key] = struct{}{}
			seenOrder = append(seenOrder, key)
			if int64(len(seenOrder)) > runEventReplayLimit {
				delete(seen, seenOrder[0])
				seenOrder = seenOrder[1:]
			}
			select {
			case ch <- evt:
				return true
			case <-ctx.Done():
				return false
			default:
				return true
			}
		}
		read := func() bool {
			payloads, err := b.client.LRange(ctx, runEventReplayKey(runID), 0, -1).Result()
			if err != nil {
				slog.Debug("automation run event replay read failed", "run_id", runID, "error", err)
				return true
			}
			for _, payload := range payloads {
				if !emit(payload) {
					return false
				}
			}
			return true
		}

		if !read() {
			return
		}
		ticker := time.NewTicker(runEventPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !read() {
					return
				}
			}
		}
	}()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (b *RedisRunEventBus) Close() error {
	if b == nil || b.client == nil {
		return nil
	}
	b.closeOnce.Do(func() {
		b.cancel()
		b.closeErr = b.client.Close()
	})
	return b.closeErr
}

var _ engine.RunEventBus = (*RedisRunEventBus)(nil)
