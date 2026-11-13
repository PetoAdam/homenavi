package http

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/api-gateway/internal/gateway"
	apiMiddleware "github.com/PetoAdam/homenavi/api-gateway/internal/middleware"
	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/PetoAdam/homenavi/shared/observability"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type captureTraceTransport struct {
	traceparent chan string
}

func TestBrowserTraceExportToJaeger(t *testing.T) {
	collector := os.Getenv("HOMENAVI_TEST_JAEGER_OTLP")
	query := os.Getenv("HOMENAVI_TEST_JAEGER_QUERY")
	if collector == "" || query == "" {
		t.Skip("isolated Jaeger endpoints are required")
	}
	t.Setenv("HOMENAVI_TRACE_INGRESS_NEW_ROOT", "true")
	var incomingID oteltrace.TraceID
	if _, err := rand.Read(incomingID[:]); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Second).UnixNano()
	payload := fmt.Sprintf(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"auth-service"}}]},"scopeSpans":[{"spans":[{"traceId":"%s","spanId":"2222222222222222","name":"browser request","kind":2,"startTimeUnixNano":"%d","endTimeUnixNano":"%d"}]}]}]}`, incomingID.String(), started, started+int64(time.Millisecond))
	request := httptest.NewRequest(http.MethodPost, "/api/telemetry/traces", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	browserTraceHandler(collector+"/v1/traces").ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("collector rejected browser payload: %d %s", response.Code, response.Body.String())
	}
	traceID := observability.NamespaceIngressTraceID(incomingID).String()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}
	for {
		result, err := client.Get(query + "/api/traces/" + traceID)
		if err == nil {
			var data struct {
				Data []struct {
					Processes map[string]struct {
						ServiceName string `json:"serviceName"`
					} `json:"processes"`
					Spans []struct {
						OperationName string `json:"operationName"`
					} `json:"spans"`
				} `json:"data"`
			}
			decodeErr := json.NewDecoder(result.Body).Decode(&data)
			_ = result.Body.Close()
			if result.StatusCode == http.StatusOK && decodeErr == nil && len(data.Data) > 0 {
				if len(data.Data[0].Spans) != 1 || data.Data[0].Spans[0].OperationName != "browser request" {
					t.Fatalf("unexpected spans: %+v", data.Data[0].Spans)
				}
				for _, process := range data.Data[0].Processes {
					if process.ServiceName != "homenavi-frontend" {
						t.Fatalf("unexpected service: %s", process.ServiceName)
					}
				}
				return
			}
		}
		select {
		case <-deadline.C:
			t.Fatal("Jaeger did not return the namespaced browser trace")
		case <-ticker.C:
		}
	}
}

func TestGatewayBrowserLinkInJaeger(t *testing.T) {
	collector := os.Getenv("HOMENAVI_TEST_JAEGER_OTLP")
	query := os.Getenv("HOMENAVI_TEST_JAEGER_QUERY")
	if collector == "" || query == "" {
		t.Skip("isolated Jaeger endpoints are required")
	}
	t.Setenv("HOMENAVI_TRACE_INGRESS_NEW_ROOT", "true")
	exporter, err := otlptracehttp.New(t.Context(), otlptracehttp.WithEndpointURL(collector+"/v1/traces"))
	if err != nil {
		t.Fatal(err)
	}
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	handler := observability.WrapHandler(provider.Tracer("gateway"), "api-gateway", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	incomingID, _ := oteltrace.TraceIDFromHex("11111111111111111111111111111111")
	wantID := observability.NamespaceIngressTraceID(incomingID).String()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}
	for {
		result, err := client.Get(query + "/api/traces/" + response.Header().Get("Trace-ID"))
		if err == nil {
			var data struct {
				Data []struct {
					Spans []struct {
						References []struct {
							TraceID string `json:"traceID"`
							SpanID  string `json:"spanID"`
						} `json:"references"`
					} `json:"spans"`
				} `json:"data"`
			}
			decodeErr := json.NewDecoder(result.Body).Decode(&data)
			_ = result.Body.Close()
			if result.StatusCode == http.StatusOK && decodeErr == nil && len(data.Data) > 0 {
				found := map[string]bool{}
				for _, span := range data.Data[0].Spans {
					for _, reference := range span.References {
						if reference.SpanID == "2222222222222222" {
							found[reference.TraceID] = true
						}
					}
				}
				if found[wantID] && found[incomingID.String()] {
					return
				}
				t.Fatalf("Jaeger omitted an ingress link: %+v", data.Data[0].Spans)
			}
		}
		select {
		case <-deadline.C:
			t.Fatal("Jaeger did not return the gateway trace")
		case <-ticker.C:
		}
	}
}

func TestBrowserTraceEndpointAuthenticatedAndRateLimited(t *testing.T) {
	redisAddr := os.Getenv("HOMENAVI_TEST_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("isolated Redis endpoint is required")
	}
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := authx.KeyIDForRSAPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := fmt.Sprintf("trace-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = redisClient.Del(context.Background(), authx.SessionStatusKey(sessionID), "browser-traces:"+sessionID).Err()
		_ = redisClient.Close()
	})
	if err := redisClient.Set(t.Context(), authx.SessionStatusKey(sessionID), authx.SessionStatusActive, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, apiMiddleware.Claims{
		Role: authx.RoleResident, SessionID: sessionID, TokenType: authx.TokenTypeAPI,
		RegisteredClaims: jwt.RegisteredClaims{Subject: sessionID, ID: "trace-test", ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)), NotBefore: jwt.NewNumericDate(now.Add(-time.Second)), IssuedAt: jwt.NewNumericDate(now)},
	})
	token.Header["kid"] = keyID
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("browser credentials reached collector")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	t.Setenv("HOMENAVI_BROWSER_OTEL_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", collector.URL)
	router := NewMainRouter(gateway.Config{}, redisClient, apiMiddleware.RSAPublicKeySet{keyID: &privateKey.PublicKey}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), otel.Tracer("test"), "")
	for index := 0; index < 6; index++ {
		request := httptest.NewRequest(http.MethodPost, "/api/telemetry/traces", strings.NewReader("{}"))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{Name: "auth_token", Value: signed})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		wanted := http.StatusOK
		if index == 5 {
			wanted = http.StatusTooManyRequests
		}
		if response.Code != wanted {
			t.Fatalf("upload %d: got %d, want %d: %s", index, response.Code, wanted, response.Body.String())
		}
	}
}

func (transport captureTraceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.traceparent <- request.Header.Get("traceparent")
	return http.DefaultTransport.RoundTrip(request)
}

func TestLongHTTPMQTTAndPythonTraceInJaeger(t *testing.T) {
	collector := os.Getenv("HOMENAVI_TEST_JAEGER_OTLP")
	query := os.Getenv("HOMENAVI_TEST_JAEGER_QUERY")
	profileURL := os.Getenv("HOMENAVI_TEST_PROFILE_URL")
	broker := os.Getenv("HOMENAVI_TEST_MQTT_URL")
	if collector == "" || query == "" || profileURL == "" || broker == "" {
		t.Skip("isolated Jaeger, MQTT and profile service endpoints are required")
	}
	t.Setenv("HOMENAVI_TRACE_INGRESS_NEW_ROOT", "false")
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	exporter, err := otlptracehttp.New(t.Context(), otlptracehttp.WithEndpointURL(collector+"/v1/traces"))
	if err != nil {
		t.Fatal(err)
	}
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter), trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	mqtt, err := mqttx.Connect(mqttx.Options{BrokerURL: broker, BrokerKind: mqttx.BrokerKindGeneric, ClientIDPrefix: "long-trace"})
	if err != nil {
		t.Fatal(err)
	}
	defer mqtt.Close()
	consumed := make(chan string, 1)
	produced := make(chan string, 1)
	topic := fmt.Sprintf("homenavi/test/long-trace/%d", time.Now().UnixNano())
	if err := mqtt.SubscribeWithOptions(mqttx.SubscriptionOptions{Topic: topic, Mode: mqttx.SubscriptionModeExclusive}, func(message mqttx.Message) {
		_, span := observability.StartMessageConsumer(context.Background(), message.Payload(), false, time.Now())
		spanID := span.SpanContext().SpanID().String()
		span.End()
		consumed <- spanID
	}); err != nil {
		t.Fatal(err)
	}

	profileParent := make(chan string, 1)
	profileClient := &http.Client{Transport: observability.InstrumentHTTPTransport(captureTraceTransport{traceparent: profileParent})}
	last := httptest.NewServer(observability.WrapHandler(provider.Tracer("last"), "test-last", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, requestErr := http.NewRequestWithContext(r.Context(), http.MethodGet, profileURL+"/health?token=secret-value", nil)
		if requestErr != nil {
			http.Error(w, "request failed", http.StatusInternalServerError)
			return
		}
		request.Header.Set("baggage", "secret=value")
		response, requestErr := profileClient.Do(request)
		if requestErr != nil {
			http.Error(w, "profile service unavailable", http.StatusBadGateway)
			return
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			http.Error(w, "profile service unhealthy", http.StatusBadGateway)
			return
		}
		producerContext, producer := observability.StartMessageProducer(r.Context())
		envelope := map[string]any{"ts": time.Now().UnixMilli(), "corr": "independent-correlation"}
		observability.InjectMessageContext(producerContext, envelope)
		payload, requestErr := json.Marshal(envelope)
		if requestErr == nil {
			requestErr = mqtt.Publish(topic, payload)
		}
		produced <- producer.SpanContext().SpanID().String()
		producer.End()
		if requestErr != nil {
			http.Error(w, "MQTT publish failed", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
	defer last.Close()
	client := observability.InstrumentHTTPClient(&http.Client{Timeout: 5 * time.Second})
	middle := httptest.NewServer(observability.WrapHandler(provider.Tracer("middle"), "test-middle", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, requestErr := http.NewRequestWithContext(r.Context(), http.MethodGet, last.URL, nil)
		if requestErr != nil {
			http.Error(w, "request failed", http.StatusInternalServerError)
			return
		}
		response, requestErr := client.Do(request)
		if requestErr != nil {
			http.Error(w, "downstream unavailable", http.StatusBadGateway)
			return
		}
		_ = response.Body.Close()
		w.WriteHeader(response.StatusCode)
	})))
	defer middle.Close()
	cfg := gateway.Config{Routes: []gateway.RouteConfig{{Path: "/api/trace-test", Upstream: middle.URL, Methods: []string{http.MethodGet}, Access: "public"}}}
	server := httptest.NewServer(NewMainRouter(cfg, nil, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), provider.Tracer("gateway"), ""))
	defer server.Close()
	ctx, root := provider.Tracer("browser-test").Start(t.Context(), "browser request")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/trace-test?token=secret-value", nil)
	if err != nil {
		t.Fatal(err)
	}
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(request.Header))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("chain returned %s", response.Status)
	}
	var consumerID string
	select {
	case consumerID = <-consumed:
	case <-time.After(5 * time.Second):
		t.Fatal("MQTT message was not consumed")
	}
	parentParts := strings.Split(<-profileParent, "-")
	if len(parentParts) != 4 {
		t.Fatal("profile request did not carry traceparent")
	}
	root.End()
	if err := provider.ForceFlush(t.Context()); err != nil {
		t.Fatal(err)
	}

	var result struct {
		Data []struct {
			Spans []struct {
				TraceID, SpanID, ProcessID string
				Tags                       []map[string]any
				References                 []struct{ RefType, SpanID string }
			} `json:"spans"`
			Processes map[string]struct{ ServiceName string } `json:"processes"`
		} `json:"data"`
	}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, queryErr := (&http.Client{Timeout: time.Second}).Get(query + "/api/traces/" + root.SpanContext().TraceID().String())
		if queryErr == nil {
			decodeErr := json.NewDecoder(response.Body).Decode(&result)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && len(result.Data) > 0 && len(result.Data[0].Spans) >= 10 {
				break
			}
		}
		select {
		case <-deadline.C:
			t.Fatal("Jaeger did not return the complete cross-language and MQTT trace")
		case <-ticker.C:
		}
	}
	parents := make(map[string]string)
	profileID := ""
	for _, span := range result.Data[0].Spans {
		if span.TraceID != root.SpanContext().TraceID().String() {
			t.Fatal("span trace ID mismatch")
		}
		if result.Data[0].Processes[span.ProcessID].ServiceName == "profile-picture-service" {
			profileID = span.SpanID
		}
		for _, reference := range span.References {
			if reference.RefType == "CHILD_OF" {
				parents[span.SpanID] = reference.SpanID
			}
		}
		tags, _ := json.Marshal(span.Tags)
		if strings.Contains(string(tags), "secret-value") {
			t.Fatal("sensitive query value was exported")
		}
	}
	if profileID == "" || parents[profileID] != parentParts[2] {
		t.Fatalf("Python span parent mismatch: %q, want %q", parents[profileID], parentParts[2])
	}
	if producerID := <-produced; parents[consumerID] != producerID {
		t.Fatalf("MQTT consumer parent mismatch: %q, want %q", parents[consumerID], producerID)
	}
	for _, span := range recorder.Ended() {
		if span.Parent().IsValid() && parents[span.SpanContext().SpanID().String()] != span.Parent().SpanID().String() {
			t.Fatalf("Jaeger parent mismatch for %s", span.Name())
		}
	}
}
