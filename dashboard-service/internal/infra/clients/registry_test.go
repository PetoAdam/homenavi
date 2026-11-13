package clients

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PetoAdam/homenavi/dashboard-service/internal/dashboard"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRegistryClientContinuesRequestTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	var traceparent, authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		traceparent = request.Header.Get("traceparent")
		authorization = request.Header.Get("Authorization")
		_, _ = writer.Write([]byte(`{"integrations":[]}`))
	}))
	defer server.Close()
	original := server.Client()
	client := NewRegistryClient(server.URL, original)
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	if _, err := client.Widgets(ctx, dashboard.AuthContext{Authorization: "Bearer token"}); err != nil {
		t.Fatal(err)
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 2 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() || traceparent != "00-"+parent.SpanContext().TraceID().String()+"-"+spans[0].SpanContext().SpanID().String()+"-01" || authorization != "Bearer token" {
		t.Fatal("registry request did not preserve trace and authorization")
	}
	if original.Transport != nil && original.Transport == client.httpClient.Transport {
		t.Fatal("original client transport was modified")
	}
}
