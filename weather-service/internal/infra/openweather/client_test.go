package openweather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWeatherLookupTracesWithoutSendingContextExternally(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("traceparent") != "" || request.Header.Get("baggage") != "" {
			t.Error("trace context sent to weather provider")
		}
		_, _ = writer.Write([]byte(`[]`))
	}))
	defer server.Close()
	client := newClient("secret-key", server.Client(), server.URL, server.URL)
	ctx, parent := provider.Tracer("test").Start(t.Context(), "request")
	if _, err := client.SearchLocations(ctx, "Budapest", 5); err != nil {
		t.Fatal(err)
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 2 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("weather request did not produce a child client span")
	}
}

func TestSearchLocationsFallsBackToMockOnAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newClient("test-key", server.Client(), server.URL, server.URL)
	locations, err := client.SearchLocations(context.Background(), "Bud", 5)
	if err != nil {
		t.Fatalf("expected fallback to mock, got error: %v", err)
	}
	if len(locations) == 0 {
		t.Fatal("expected mock locations fallback")
	}
}

func TestGetWeatherFallsBackToMockOnAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newClient("test-key", server.Client(), server.URL, server.URL)
	weather, err := client.GetWeather(context.Background(), 47.49, 19.04, "Budapest")
	if err != nil {
		t.Fatalf("expected fallback to mock, got error: %v", err)
	}
	if weather.City != "Budapest" {
		t.Fatalf("expected mock weather city, got %q", weather.City)
	}
	if len(weather.Daily) != 24 {
		t.Fatalf("expected mock hourly forecast, got %d items", len(weather.Daily))
	}
}

func TestMapOWMIcon(t *testing.T) {
	cases := map[string]string{
		"01d": "sun",
		"02n": "cloud_sun",
		"10d": "rain",
		"11n": "storm",
		"50d": "fog",
		"??":  "sun",
	}
	for in, want := range cases {
		if got := mapOWMIcon(in); got != want {
			t.Fatalf("mapOWMIcon(%q) = %q, want %q", in, got, want)
		}
	}
}
