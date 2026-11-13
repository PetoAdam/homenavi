package observability

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	return setupObservability(serviceName, func(ctx context.Context, endpoint string) (trace.SpanExporter, error) {
		if err := validateCollectorConfig(endpoint); err != nil {
			return nil, err
		}
		return otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	})
}

func validateCollectorConfig(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("invalid trace collector endpoint")
	}
	for _, prefix := range []string{"OTEL_EXPORTER_OTLP_", "OTEL_EXPORTER_OTLP_TRACES_"} {
		if path := os.Getenv(prefix + "CERTIFICATE"); path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return errors.New("invalid trace collector CA certificate")
			}
			if !x509.NewCertPool().AppendCertsFromPEM(data) {
				return errors.New("invalid trace collector CA certificate")
			}
		}
		cert, key := os.Getenv(prefix+"CLIENT_CERTIFICATE"), os.Getenv(prefix+"CLIENT_KEY")
		if (cert == "") != (key == "") {
			return errors.New("trace collector client certificate and key must be configured together")
		}
		if cert != "" {
			if _, err := tls.LoadX509KeyPair(cert, key); err != nil {
				return errors.New("invalid trace collector client certificate")
			}
		}
	}
	return nil
}

func setupObservability(serviceName string, newExporter func(context.Context, string) (trace.SpanExporter, error)) (shutdown func(), promHandler http.Handler, tracer oteltrace.Tracer, err error) {
	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	otel.SetTextMapPropagator(propagator)

	promExporter, err := otelprom.New()
	if err != nil {
		slog.Warn("OpenTelemetry metrics unavailable")
	} else {
		meterProvider := otelmetric.NewMeterProvider(otelmetric.WithReader(promExporter))
		otel.SetMeterProvider(meterProvider)
	}

	res, err := resource.New(context.Background(), resource.WithAttributes(attribute.String("service.name", serviceName)))
	if err != nil {
		slog.Warn("OpenTelemetry resource unavailable; using service name only")
		res = resource.NewSchemaless(attribute.String("service.name", serviceName))
	}

	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	var tp *trace.TracerProvider
	if otlpEndpoint != "" {
		exp, err := newExporter(context.Background(), otlpEndpoint)
		if err != nil {
			slog.Warn("OpenTelemetry trace export unavailable; continuing without trace export")
			tp = trace.NewTracerProvider(trace.WithResource(res))
		} else {
			tp = trace.NewTracerProvider(trace.WithBatcher(exp), trace.WithResource(res))
		}
	} else {
		tp = trace.NewTracerProvider(trace.WithResource(res))
	}
	otel.SetTracerProvider(tp)

	shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}
	promHandler = promhttp.Handler()
	tracer = otel.Tracer(serviceName)
	return shutdown, promHandler, tracer, nil
}

func StartBackgroundOperation(ctx context.Context, name string) (context.Context, oteltrace.Span) {
	return otel.Tracer("homenavi/background").Start(ctx, name, oteltrace.WithNewRoot())
}

func FinishBackgroundOperation(span oteltrace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, "background operation failed")
	}
	span.End()
}

func IndependentIngressEnabled() bool {
	value := os.Getenv("HOMENAVI_TRACE_INGRESS_NEW_ROOT")
	if value == "" {
		return true
	}
	enabled, err := strconv.ParseBool(value)
	return err != nil || enabled
}

func NamespaceIngressTraceID(id oteltrace.TraceID) oteltrace.TraceID {
	hash := sha256.New()
	_, _ = hash.Write([]byte("homenavi-untrusted-ingress-v1:"))
	_, _ = hash.Write(id[:])
	var namespaced oteltrace.TraceID
	copy(namespaced[:], hash.Sum(nil))
	return namespaced
}

type httpCompletionKey struct{}

func FinishHTTPUpgrade(ctx context.Context) {
	if complete, ok := ctx.Value(httpCompletionKey{}).(func(int)); ok {
		complete(http.StatusSwitchingProtocols)
	}
}

func MetricsAndTracingMiddleware(tracer oteltrace.Tracer, serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			method := r.Method
			metricMethod := method
			switch method {
			case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
			default:
				metricMethod = "_OTHER"
			}
			ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			options := []oteltrace.SpanStartOption{oteltrace.WithSpanKind(oteltrace.SpanKindServer)}
			if serviceName == "api-gateway" && IndependentIngressEnabled() {
				incoming := oteltrace.SpanContextFromContext(ctx)
				options = append(options, oteltrace.WithNewRoot())
				if incoming.IsValid() {
					browser := incoming.WithTraceID(NamespaceIngressTraceID(incoming.TraceID()))
					options = append(options, oteltrace.WithLinks(
						oteltrace.Link{SpanContext: incoming, Attributes: []attribute.KeyValue{attribute.String("homenavi.link.source", "incoming")}},
						oteltrace.Link{SpanContext: browser, Attributes: []attribute.KeyValue{attribute.String("homenavi.link.source", "browser")}},
					))
				}
			}
			ctx, span := tracer.Start(ctx, "HTTP "+method, options...)
			if chi.RouteContext(ctx) == nil {
				if routes, ok := next.(chi.Routes); ok {
					routeContext := chi.NewRouteContext()
					routeContext.Routes = routes
					ctx = context.WithValue(ctx, chi.RouteCtxKey, routeContext)
				}
			}
			span.SetAttributes(
				attribute.String("http.method", method),
				attribute.String("service.name", serviceName),
			)
			rw.Header().Set("Trace-ID", span.SpanContext().TraceID().String())

			var once sync.Once
			var request *http.Request
			complete := func(status int) {
				once.Do(func() {
					pattern := request.Pattern
					if routeContext := chi.RouteContext(ctx); routeContext != nil {
						pattern = routeContext.RoutePattern()
					}
					if pattern != "" {
						pattern = strings.TrimPrefix(pattern, method+" ")
						span.SetName(method + " " + pattern)
						span.SetAttributes(attribute.String("http.route", pattern))
					} else {
						pattern = "unmatched"
					}
					span.SetAttributes(attribute.Int("http.status_code", status))
					requestCounter.WithLabelValues(serviceName, pattern, metricMethod, strconv.Itoa(status)).Inc()
					span.End()
				})
			}
			request = r.WithContext(context.WithValue(ctx, httpCompletionKey{}, complete))
			defer func() { complete(rw.status) }()
			next.ServeHTTP(rw, request)
		})
	}
}

func WrapHandler(tracer oteltrace.Tracer, serviceName string, next http.Handler) http.Handler {
	return MetricsAndTracingMiddleware(tracer, serviceName)(next)
}

func WithMetricsEndpoint(promHandler http.Handler, tracer oteltrace.Tracer, serviceName string, next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promHandler)
	mux.Handle("/", WrapHandler(tracer, serviceName, next))
	return mux
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}
