package observability

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	otelmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var requestCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total requests by service, endpoint, method, and status.",
	},
	[]string{"service", "endpoint", "method", "status"},
)

func init() {
	prometheus.MustRegister(requestCounter)
}

func SetupObservability(serviceName string) (shutdown func(), promHandler http.Handler, tracer oteltrace.Tracer, err error) {
	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	otel.SetTextMapPropagator(propagator)

	promExporter, err := otelprom.New()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create prometheus exporter: %w", err)
	}
	meterProvider := otelmetric.NewMeterProvider(otelmetric.WithReader(promExporter))
	otel.SetMeterProvider(meterProvider)

	res, err := resource.New(context.Background(), resource.WithAttributes(attribute.String("service.name", serviceName)))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create otel resource: %w", err)
	}

	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	var tp *trace.TracerProvider
	if otlpEndpoint != "" {
		exp, err := otlptracehttp.New(context.Background(),
			otlptracehttp.WithEndpointURL(otlpEndpoint),
		)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("create OTLP trace exporter: %w", err)
		}
		tp = trace.NewTracerProvider(trace.WithBatcher(exp), trace.WithResource(res))
	} else {
		tp = trace.NewTracerProvider(trace.WithResource(res))
	}
	otel.SetTracerProvider(tp)

	shutdown = func() { _ = tp.Shutdown(context.Background()) }
	promHandler = promhttp.Handler()
	tracer = otel.Tracer(serviceName)
	return shutdown, promHandler, tracer, nil
}

func MetricsAndTracingMiddleware(tracer oteltrace.Tracer, serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			endpoint := r.URL.Path
			method := r.Method
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			ctx, span := tracer.Start(ctx, method+" "+endpoint)
			span.SetAttributes(
				attribute.String("http.method", method),
				attribute.String("http.target", endpoint),
				attribute.String("service.name", serviceName),
			)
			rw.Header().Set("Trace-ID", span.SpanContext().TraceID().String())
			if rid := middleware.GetReqID(ctx); rid != "" {
				span.SetAttributes(attribute.String("http.request_id", rid))
			}
			if xid := r.Header.Get("X-Request-ID"); xid != "" {
				span.SetAttributes(attribute.String("http.x_request_id", xid))
			}

			next.ServeHTTP(rw, r.WithContext(ctx))

			status := rw.status
			span.SetAttributes(attribute.Int("http.status_code", status))
			requestCounter.WithLabelValues(serviceName, endpoint, method, strconv.Itoa(status)).Inc()
			span.End()
		})
	}
}

func WrapHandler(tracer oteltrace.Tracer, serviceName string, next http.Handler) http.Handler {
	return MetricsAndTracingMiddleware(tracer, serviceName)(next)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
