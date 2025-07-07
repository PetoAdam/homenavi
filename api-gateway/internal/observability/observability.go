package observability

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var (
	RequestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_gateway_requests_total",
			Help: "Total requests by endpoint and method.",
		},
		[]string{"endpoint", "method"},
	)
)

func init() {
	prometheus.MustRegister(RequestCounter)
}

func SetupObservability() (shutdown func(), promHandler http.Handler, tracer oteltrace.Tracer) {
	// Prometheus exporter
	promExporter, err := otelprom.New()
	if err != nil {
		log.Fatalf("failed to create prometheus exporter: %v", err)
	}
	meterProvider := otelmetric.NewMeterProvider(otelmetric.WithReader(promExporter))
	otel.SetMeterProvider(meterProvider)

	// Set service name for resource
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName("api-gateway"),
		),
	)
	if err != nil {
		log.Fatalf("failed to create otel resource: %v", err)
	}

	// Jaeger exporter
	jaegerURL := os.Getenv("JAEGER_ENDPOINT")
	var tp *trace.TracerProvider
	if jaegerURL != "" {
		exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(jaegerURL)))
		if err != nil {
			log.Fatalf("failed to create Jaeger exporter: %v", err)
		}
		tp = trace.NewTracerProvider(
			trace.WithBatcher(exp),
			trace.WithResource(res),
		)
		otel.SetTracerProvider(tp)
	} else {
		tp = trace.NewTracerProvider(trace.WithResource(res))
		otel.SetTracerProvider(tp)
	}

	shutdown = func() {
		_ = tp.Shutdown(context.Background())
	}
	promHandler = promhttp.Handler()
	tracer = otel.Tracer("api-gateway")
	return shutdown, promHandler, tracer
}

// Middleware for per-endpoint Prometheus counting and tracing
func MetricsAndTracingMiddleware(tracer oteltrace.Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			endpoint := r.URL.Path
			method := r.Method
			RequestCounter.WithLabelValues(endpoint, method).Inc()

			// Wrap ResponseWriter to capture status code
			rw := &statusRecorder{ResponseWriter: w, status: 200}
			ctx, span := tracer.Start(r.Context(), method+" "+endpoint)
			span.SetAttributes(
				semconv.HTTPMethod(method),
				semconv.HTTPTarget(endpoint),
			)
			next.ServeHTTP(rw, r.WithContext(ctx))
			span.SetAttributes(semconv.HTTPStatusCode(rw.status))
			span.End()
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
