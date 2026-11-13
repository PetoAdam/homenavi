package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestUserClientPropagatesRequestTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	var traceparent, authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		traceparent = request.Header.Get("traceparent")
		authorization = request.Header.Get("Authorization")
		_, _ = writer.Write([]byte(`{"id":"user-1"}`))
	}))
	defer server.Close()
	client := NewUserClient(UserConfig{BaseURL: server.URL, IssueToken: func() (string, error) { return "internal-token", nil }})
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	if _, err := client.WithContext(ctx).GetUser("user-1"); err != nil {
		t.Fatal(err)
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 2 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() || traceparent != "00-"+parent.SpanContext().TraceID().String()+"-"+spans[0].SpanContext().SpanID().String()+"-01" || authorization != "Bearer internal-token" {
		t.Fatal("user request did not preserve trace and authorization")
	}
	if client.ctx != nil {
		t.Fatal("request context leaked into the shared client")
	}
}

func TestEmailAndProfileClientsPropagateRequestTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	var headers []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		headers = append(headers, request.Header.Get("traceparent"))
		if request.URL.Path == "/generate/user-1" {
			_, _ = writer.Write([]byte(`{"success":true,"url":"/avatar"}`))
		}
	}))
	defer server.Close()
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	email := NewEmailClient(server.URL)
	profile := NewProfilePictureClient(server.URL)
	if err := email.WithContext(ctx).SendVerificationEmail("test@example.com", "Test", "123456"); err != nil {
		t.Fatal(err)
	}
	if _, err := profile.WithContext(ctx).GenerateAvatar("user-1"); err != nil {
		t.Fatal(err)
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 3 || len(headers) != 2 {
		t.Fatalf("expected two downstream spans, got %d", len(spans))
	}
	for index := 0; index < 2; index++ {
		if spans[index].Parent().SpanID() != parent.SpanContext().SpanID() || headers[index] != "00-"+parent.SpanContext().TraceID().String()+"-"+spans[index].SpanContext().SpanID().String()+"-01" {
			t.Fatal("downstream client did not propagate its request span")
		}
	}
	if email.ctx != nil || profile.ctx != nil {
		t.Fatal("request context leaked into a shared client")
	}
}
