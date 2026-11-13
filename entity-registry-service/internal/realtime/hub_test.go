package realtime

import (
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestBroadcastTraceIsOptionalAndFollowsParent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	hub := NewHub()
	first := &client{send: make(chan []byte, 2)}
	second := &client{send: make(chan []byte, 2)}
	hub.addClient(first)
	hub.addClient(second)
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	hub.Broadcast(ctx, Event{Type: "ers.device.updated", Entity: "device", ID: "device-1"})
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("realtime producer does not follow request")
	}
	for _, subscriber := range []*client{first, second} {
		var event Event
		if err := json.Unmarshal(<-subscriber.send, &event); err != nil {
			t.Fatal(err)
		}
		expected := "00-" + parent.SpanContext().TraceID().String() + "-" + spans[0].SpanContext().SpanID().String() + "-01"
		if event.Trace == nil || event.Trace.Traceparent != expected || event.ID != "device-1" {
			t.Fatal("realtime event did not include the expected trace carrier")
		}
	}
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "false")
	hub.Broadcast(ctx, Event{Type: "ers.device.updated"})
	var legacy map[string]any
	if err := json.Unmarshal(<-first.send, &legacy); err != nil {
		t.Fatal(err)
	}
	if _, exists := legacy["trace"]; exists {
		t.Fatal("disabled propagation changed the event payload")
	}
	parent.End()
}
