package events

import (
	"context"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/automation-service/internal/engine"
	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
)

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
	ch, cancel := subscriber.Subscribe(runID)
	defer cancel()

	replayed := receiveRunEvent(t, ch)
	if replayed.Type != "run_started" || replayed.RunID != runID.String() || replayed.EventID == "" {
		t.Fatalf("unexpected replayed event: %#v", replayed)
	}

	publisher.Publish(runID, engine.RunEvent{Type: "run_finished", Status: "success"})
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
