package observability

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type tracingTransport struct {
	base      http.RoundTripper
	propagate bool
}

func InstrumentHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	instrumented := *client
	instrumented.Transport = InstrumentHTTPTransport(client.Transport)
	return &instrumented
}

func InstrumentHTTPTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if _, ok := base.(*tracingTransport); ok {
		return base
	}
	return &tracingTransport{base: base, propagate: true}
}

func InstrumentExternalHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	instrumented := *client
	base := client.Transport
	if existing, ok := base.(*tracingTransport); ok {
		base = existing.base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	instrumented.Transport = &tracingTransport{base: base}
	return &instrumented
}

func (transport *tracingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx, span := otel.Tracer("homenavi/http-client").Start(request.Context(), "HTTP "+request.Method, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	span.SetAttributes(attribute.String("http.request.method", request.Method))

	outbound := request.Clone(ctx)
	if outbound.Header == nil {
		outbound.Header = make(http.Header)
	}
	outbound.Header.Del("traceparent")
	outbound.Header.Del("tracestate")
	outbound.Header.Del("baggage")
	if transport.propagate {
		propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(outbound.Header))
	}
	response, err := transport.base.RoundTrip(outbound)
	if err != nil {
		span.SetStatus(codes.Error, "HTTP request failed")
		return response, err
	}
	span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
	if response.StatusCode >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, "HTTP server error")
	}
	return response, nil
}
