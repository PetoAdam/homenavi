package observability

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const messageParentMaxAge = 15 * time.Minute
const messageClockSkew = time.Minute
const DeferredTraceMaxAge = messageParentMaxAge

func MessagePropagationEnabled() bool {
	value := strings.TrimSpace(os.Getenv("HOMENAVI_MQTT_TRACE_ENABLED"))
	if value == "auto" {
		return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != ""
	}
	enabled, err := strconv.ParseBool(value)
	return err == nil && enabled
}

func TraceMetadataEnabled() bool {
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != "" || MessagePropagationEnabled()
}

func StartMessageProducer(ctx context.Context) (context.Context, trace.Span) {
	return otel.Tracer("homenavi/mqtt").Start(ctx, "messaging.publish", trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attribute.String("messaging.system", "mqtt")))
}

func StartRealtimeProducer(ctx context.Context) (context.Context, trace.Span) {
	return otel.Tracer("homenavi/realtime").Start(ctx, "realtime.publish", trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attribute.String("messaging.system", "websocket")))
}

func InjectMessageContext(ctx context.Context, envelope map[string]any) {
	if envelope == nil {
		return
	}
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	if carrier.Get("traceparent") == "" {
		delete(envelope, "trace")
		return
	}
	envelope["trace"] = hdp.TraceContext{Traceparent: carrier.Get("traceparent"), Tracestate: carrier.Get("tracestate")}
}

func CaptureTraceContext(ctx context.Context) *hdp.TraceContext {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	if carrier.Get("traceparent") == "" || len(carrier.Get("traceparent")) > 256 || len(carrier.Get("tracestate")) > 512 {
		return nil
	}
	return &hdp.TraceContext{Traceparent: carrier.Get("traceparent"), Tracestate: carrier.Get("tracestate")}
}

func StartDeferredSpan(metadata *hdp.TraceContext, startedAt, now time.Time, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	ctx := context.Background()
	var remote trace.SpanContext
	if metadata != nil && len(metadata.Traceparent) <= 256 && len(metadata.Tracestate) <= 512 {
		carrier := propagation.MapCarrier{"traceparent": metadata.Traceparent, "tracestate": metadata.Tracestate}
		remote = trace.SpanContextFromContext(propagation.TraceContext{}.Extract(ctx, carrier))
	}
	if remote.IsValid() && !startedAt.IsZero() && !startedAt.After(now.Add(messageClockSkew)) && !startedAt.Before(now.Add(-messageParentMaxAge)) {
		ctx = trace.ContextWithRemoteSpanContext(ctx, remote)
	} else if remote.IsValid() {
		options = append(options, trace.WithNewRoot(), trace.WithLinks(trace.Link{SpanContext: remote}))
	}
	return otel.Tracer("homenavi/deferred").Start(ctx, name, options...)
}

func StartCorrelatedMessageConsumer(payload []byte, retained bool, now time.Time, metadata *hdp.TraceContext, startedAt time.Time) (context.Context, trace.Span) {
	var envelope struct {
		TS    int64           `json:"ts"`
		Trace json.RawMessage `json:"trace"`
	}
	if json.Unmarshal(payload, &envelope) == nil && len(envelope.Trace) == 0 && !retained && metadata != nil && envelope.TS > 0 {
		messageTime := time.UnixMilli(envelope.TS)
		if !messageTime.Before(startedAt.Add(-messageClockSkew)) && !messageTime.Before(now.Add(-messageParentMaxAge)) && !messageTime.After(now.Add(messageClockSkew)) {
			return StartDeferredSpan(metadata, startedAt, now, "messaging.process", trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attribute.String("messaging.system", "mqtt")))
		}
	}
	return StartMessageConsumer(context.Background(), payload, retained, now)
}

func StartMessageConsumer(ctx context.Context, payload []byte, retained bool, now time.Time) (context.Context, trace.Span) {
	var envelope struct {
		TS    int64           `json:"ts"`
		Trace json.RawMessage `json:"trace"`
	}
	var remote trace.SpanContext
	if json.Unmarshal(payload, &envelope) == nil && len(envelope.Trace) > 0 && len(envelope.Trace) <= 1024 {
		var metadata hdp.TraceContext
		if json.Unmarshal(envelope.Trace, &metadata) == nil && len(metadata.Traceparent) <= 256 && len(metadata.Tracestate) <= 512 {
			carrier := propagation.MapCarrier{"traceparent": metadata.Traceparent, "tracestate": metadata.Tracestate}
			remote = trace.SpanContextFromContext(propagation.TraceContext{}.Extract(context.Background(), carrier))
		}
	}
	opts := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attribute.String("messaging.system", "mqtt"))}
	messageTime := time.UnixMilli(envelope.TS)
	fresh := envelope.TS > 0 && !messageTime.After(now.Add(messageClockSkew)) && !messageTime.Before(now.Add(-messageParentMaxAge))
	if remote.IsValid() && fresh && !retained {
		ctx = trace.ContextWithRemoteSpanContext(ctx, remote)
	} else {
		opts = append(opts, trace.WithNewRoot())
		if remote.IsValid() {
			opts = append(opts, trace.WithLinks(trace.Link{SpanContext: remote}))
		}
	}
	return otel.Tracer("homenavi/mqtt").Start(ctx, "messaging.process", opts...)
}
