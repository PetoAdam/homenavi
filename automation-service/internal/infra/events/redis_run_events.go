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
)

// RedisRunEventBus stores a bounded replay list in Redis so all automation
// replicas can serve run-event WebSockets without Redis Pub/Sub.
type RedisRunEventBus struct {
	client redis.UniversalClient
}

func NewRedisRunEventBus(ctx context.Context, cfg redisx.Config) (*RedisRunEventBus, error) {
	client, err := redisx.Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &RedisRunEventBus{client: client}, nil
}

func runEventReplayKey(runID uuid.UUID) string {
	return "homenavi:automation:run:" + runID.String() + ":replay"
}

func (b *RedisRunEventBus) Publish(runID uuid.UUID, evt engine.RunEvent) {
	if b == nil || b.client == nil || runID == uuid.Nil {
		return
	}
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

	ctx := context.Background()
	pipeline := b.client.TxPipeline()
	pipeline.RPush(ctx, runEventReplayKey(runID), payload)
	pipeline.LTrim(ctx, runEventReplayKey(runID), -runEventReplayLimit, -1)
	pipeline.Expire(ctx, runEventReplayKey(runID), runEventReplayTTL)
	if _, err := pipeline.Exec(ctx); err != nil {
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
	return b.client.Close()
}

var _ engine.RunEventBus = (*RedisRunEventBus)(nil)
