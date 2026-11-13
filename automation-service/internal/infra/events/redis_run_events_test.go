package events

import (
	"context"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/automation-service/internal/engine"
	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type blockingPipelineHook struct {
	started chan struct{}
	release chan struct{}
}

func (hook blockingPipelineHook) DialHook(next redis.DialHook) redis.DialHook          { return next }
func (hook blockingPipelineHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }
func (hook blockingPipelineHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, commands []redis.Cmder) error {
		select {
		case hook.started <- struct{}{}:
		default:
		}
		select {
		case <-hook.release:
			return next(ctx, commands)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func TestRedisRunEventBusPublishDoesNotWaitForRedis(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	bus, err := NewRedisRunEventBus(context.Background(), redisx.Config{Mode: redisx.ModeStandalone, Addrs: []string{server.Addr()}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bus.Close() }()
	hook := blockingPipelineHook{started: make(chan struct{}, 1), release: make(chan struct{})}
	bus.client.AddHook(hook)
	runID := uuid.New()
	bus.Publish(runID, engine.RunEvent{Type: "first"})
	select {
	case <-hook.started:
	case <-time.After(time.Second):
		t.Fatal("Redis write did not start")
	}
	finished := make(chan struct{})
	go func() {
		for index := 0; index < runEventQueueLimit*2; index++ {
			bus.Publish(runID, engine.RunEvent{Type: "queued"})
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("event publishing blocked behind Redis")
	}
	if got := len(bus.queue); got != runEventQueueLimit {
		t.Fatalf("expected a bounded full queue, got %d events", got)
	}
	closed := make(chan struct{})
	go func() {
		_ = bus.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("event bus shutdown blocked behind Redis")
	}
	bus.Publish(runID, engine.RunEvent{Type: "after_close"})
}

func TestRedisRunEventBusReplaysAcrossInstances(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run() error = %v", err)
	}
	defer server.Close()

	publisher, err := NewRedisRunEventBus(context.Background(), redisx.Config{Mode: redisx.ModeStandalone, Addrs: []string{server.Addr()}})
	if err != nil {
		t.Fatalf("NewRedisRunEventBus() publisher error = %v", err)
	}
	defer func() { _ = publisher.Close() }()
	subscriber, err := NewRedisRunEventBus(context.Background(), redisx.Config{Mode: redisx.ModeStandalone, Addrs: []string{server.Addr()}})
	if err != nil {
		t.Fatalf("NewRedisRunEventBus() subscriber error = %v", err)
	}
	defer func() { _ = subscriber.Close() }()

	runID := uuid.New()
	publisher.Publish(runID, engine.RunEvent{Type: "run_started"})
	publisher.Publish(runID, engine.RunEvent{Type: "run_finished", Status: "success"})
	ch, cancel := subscriber.Subscribe(runID)
	defer cancel()

	replayed := receiveRunEvent(t, ch)
	if replayed.Type != "run_started" || replayed.RunID != runID.String() || replayed.EventID == "" {
		t.Fatalf("unexpected replayed event: %#v", replayed)
	}

	live := receiveRunEvent(t, ch)
	if live.Type != "run_finished" || live.Status != "success" || live.EventID == "" {
		t.Fatalf("unexpected live event: %#v", live)
	}
}

func TestRedisRunEventBusCancelClosesSubscription(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run() error = %v", err)
	}
	defer server.Close()

	bus, err := NewRedisRunEventBus(context.Background(), redisx.Config{Mode: redisx.ModeStandalone, Addrs: []string{server.Addr()}})
	if err != nil {
		t.Fatalf("NewRedisRunEventBus() error = %v", err)
	}
	defer func() { _ = bus.Close() }()

	ch, cancel := bus.Subscribe(uuid.New())
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected canceled subscription channel to close")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for canceled subscription to close")
	}
}

func receiveRunEvent(t *testing.T, ch <-chan engine.RunEvent) engine.RunEvent {
	t.Helper()
	select {
	case evt, ok := <-ch:
		if !ok {
			t.Fatal("run event subscription closed unexpectedly")
		}
		return evt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for run event")
		return engine.RunEvent{}
	}
}
