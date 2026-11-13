package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/PetoAdam/homenavi/shared/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type brokerTestClient struct{ *mqttx.Client }

func (client brokerTestClient) Subscribe(topic string, handler Handler) error {
	return client.Client.SubscribeWithOptions(mqttx.SubscriptionOptions{Topic: topic, Mode: mqttx.SubscriptionModeExclusive}, func(message mqttx.Message) {
		handler(message)
	})
}

func TestBrokerAndJaegerCommandTrace(t *testing.T) {
	brokerURL := os.Getenv("HOMENAVI_TEST_MQTT_URL")
	collectorURL := os.Getenv("HOMENAVI_TEST_JAEGER_OTLP")
	queryURL := os.Getenv("HOMENAVI_TEST_JAEGER_QUERY")
	if brokerURL == "" || collectorURL == "" || queryURL == "" {
		t.Skip("isolated MQTT and Jaeger endpoints are not configured")
	}
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	exporter, err := otlptracehttp.New(t.Context(), otlptracehttp.WithEndpointURL(collectorURL+"/v1/traces"))
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
	adapterMQTT, err := mqttx.Connect(mqttx.Options{BrokerURL: brokerURL, BrokerKind: mqttx.BrokerKindGeneric, ClientIDPrefix: "trace-adapter"})
	if err != nil {
		t.Fatal(err)
	}
	defer adapterMQTT.Close()
	clientMQTT, err := mqttx.Connect(mqttx.Options{BrokerURL: brokerURL, BrokerKind: mqttx.BrokerKindGeneric, ClientIDPrefix: "trace-client"})
	if err != nil {
		t.Fatal(err)
	}
	defer clientMQTT.Close()
	results := make(chan []byte, 1)
	if err := clientMQTT.SubscribeWithOptions(mqttx.SubscriptionOptions{Topic: hdp.CommandResultPrefix + "mock/trace-test", Mode: mqttx.SubscriptionModeExclusive}, func(message mqttx.Message) {
		results <- append([]byte(nil), message.Payload()...)
	}); err != nil {
		t.Fatal(err)
	}
	service := New(brokerTestClient{adapterMQTT}, Config{Enabled: true, AdapterID: "trace-test"})
	if err := service.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer service.Stop()
	ctx, root := provider.Tracer("test").Start(t.Context(), "request")
	producerCtx, producer := observability.StartMessageProducer(ctx)
	envelope := map[string]any{"device_id": "mock/trace-test", "corr": "corr-1", "ts": time.Now().UnixMilli()}
	observability.InjectMessageContext(producerCtx, envelope)
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := clientMQTT.Publish(hdp.CommandPrefix+"mock/trace-test", payload); err != nil {
		t.Fatal(err)
	}
	producer.End()
	select {
	case payload = <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("MQTT command result not received")
	}
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil || result["corr"] != "corr-1" {
		t.Fatalf("MQTT correlation missing: %v %v", result, err)
	}
	_, consumer := observability.StartMessageConsumer(t.Context(), payload, false, time.Now())
	consumer.End()
	root.End()
	if err := provider.ForceFlush(t.Context()); err != nil {
		t.Fatal(err)
	}
	var responseData struct {
		Data []struct {
			Spans []struct {
				SpanID     string `json:"spanID"`
				References []struct {
					RefType string `json:"refType"`
					SpanID  string `json:"spanID"`
				} `json:"references"`
			} `json:"spans"`
		} `json:"data"`
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	httpClient := &http.Client{Timeout: time.Second}
	for {
		response, queryErr := httpClient.Get(queryURL + "/api/traces/" + root.SpanContext().TraceID().String())
		if queryErr == nil {
			decodeErr := json.NewDecoder(response.Body).Decode(&responseData)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && len(responseData.Data) > 0 && len(responseData.Data[0].Spans) >= 5 {
				break
			}
		}
		select {
		case <-deadline.C:
			t.Fatal("Jaeger did not return the MQTT command trace")
		case <-ticker.C:
		}
	}
	parents := make(map[string]string)
	for _, span := range responseData.Data[0].Spans {
		for _, reference := range span.References {
			if reference.RefType == "CHILD_OF" {
				parents[span.SpanID] = reference.SpanID
			}
		}
	}
	spans := recorder.Ended()
	if len(spans) != 5 {
		t.Fatalf("expected five spans, got %d", len(spans))
	}
	for _, span := range spans {
		if span.Parent().IsValid() && parents[span.SpanContext().SpanID().String()] != span.Parent().SpanID().String() {
			t.Fatalf("Jaeger did not preserve the parent of %s", span.Name())
		}
	}
}

type publishedMessage struct {
	topic   string
	payload []byte
	retain  bool
}

type fakeClient struct {
	subscribed map[string]Handler
	published  []publishedMessage
}

func newFakeClient() *fakeClient {
	return &fakeClient{subscribed: map[string]Handler{}}
}

func (f *fakeClient) Publish(topic string, payload []byte) error {
	f.published = append(f.published, publishedMessage{topic: topic, payload: payload})
	return nil
}

func (f *fakeClient) PublishWith(topic string, payload []byte, retain bool) error {
	f.published = append(f.published, publishedMessage{topic: topic, payload: payload, retain: retain})
	return nil
}

func (f *fakeClient) Subscribe(topic string, cb Handler) error {
	f.subscribed[topic] = cb
	return nil
}

type fakeMessage struct {
	topic   string
	payload []byte
}

func (f fakeMessage) Duplicate() bool   { return false }
func (f fakeMessage) Qos() byte         { return 0 }
func (f fakeMessage) Retained() bool    { return false }
func (f fakeMessage) Topic() string     { return f.topic }
func (f fakeMessage) MessageID() uint16 { return 0 }
func (f fakeMessage) Payload() []byte   { return f.payload }
func (f fakeMessage) Ack()              {}

func TestStartPublishesHelloAndStatusAndSubscribes(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "mock-adapter-1", Version: "dev"})

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer svc.Stop()

	if len(client.published) < 2 {
		t.Fatalf("expected hello and status messages, got %d", len(client.published))
	}
	if _, ok := client.subscribed[hdp.PairingCommandPrefix+"mock"]; !ok {
		t.Fatal("expected pairing subscription")
	}
	if _, ok := client.subscribed[hdp.CommandPrefix+"mock/#"]; !ok {
		t.Fatal("expected command subscription")
	}
	if client.published[1].topic != hdp.AdapterStatusPrefix+"mock-adapter-1" || !client.published[1].retain {
		t.Fatal("expected retained adapter status publish")
	}
}

func TestHandlePairingStartPublishesProgress(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "mock-adapter-1", Version: "dev"})

	svc.handlePairingCommand(fakeMessage{topic: hdp.PairingCommandPrefix + "mock", payload: []byte(`{"action":"start","mode":"default","flow_id":"flow-a"}`)})

	if len(client.published) != 2 {
		t.Fatalf("expected two progress messages, got %d", len(client.published))
	}
	for _, msg := range client.published {
		if msg.topic != hdp.PairingProgressPrefix+"mock" {
			t.Fatalf("unexpected topic %q", msg.topic)
		}
	}
	var completed map[string]any
	if err := json.Unmarshal(client.published[1].payload, &completed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if completed["status"] != "completed" {
		t.Fatalf("expected completed status, got %v", completed["status"])
	}
	if completed["mode"] != "default" {
		t.Fatalf("expected mode to be echoed, got %v", completed["mode"])
	}
	if completed["flow_id"] != "flow-a" {
		t.Fatalf("expected flow_id to be echoed, got %v", completed["flow_id"])
	}
}

func TestPairingProgressPropagatesTrace(t *testing.T) {
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); _ = provider.Shutdown(t.Context()) })
	ctx, parent := provider.Tracer("test").Start(t.Context(), "pairing")
	envelope := map[string]any{"action": "start", "ts": time.Now().UnixMilli()}
	observability.InjectMessageContext(ctx, envelope)
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	client := newFakeClient()
	New(client, Config{Enabled: true}).handlePairingCommand(fakeMessage{topic: hdp.PairingCommandPrefix + "mock", payload: payload})
	if len(client.published) != 2 {
		t.Fatalf("got %d progress messages", len(client.published))
	}
	for _, message := range client.published {
		_, consumer := observability.StartMessageConsumer(t.Context(), message.payload, false, time.Now())
		if consumer.SpanContext().TraceID() != parent.SpanContext().TraceID() {
			t.Fatal("pairing progress lost trace context")
		}
		consumer.End()
	}
	parent.End()
	if len(recorder.Ended()) != 6 {
		t.Fatalf("unexpected spans: %d", len(recorder.Ended()))
	}
}

func TestHandlePairingStartNeedsInputForQRCodeMode(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "mock-adapter-1", Version: "dev"})

	svc.handlePairingCommand(fakeMessage{topic: hdp.PairingCommandPrefix + "mock", payload: []byte(`{"action":"start","mode":"qr_code","inputs":{}}`)})

	if len(client.published) != 1 {
		t.Fatalf("expected one progress message, got %d", len(client.published))
	}
	var payload map[string]any
	if err := json.Unmarshal(client.published[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["status"] != "needs_input" {
		t.Fatalf("expected needs_input status, got %v", payload["status"])
	}
	required, ok := payload["required_inputs"].([]any)
	if !ok || len(required) != 1 || required[0] != "onboarding_payload" {
		t.Fatalf("expected required onboarding_payload input, got %#v", payload["required_inputs"])
	}
}

func TestHandleDeviceCommandRejectsAndPublishesResult(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "mock-adapter-1", Version: "dev"})
	payload := []byte(`{"device_id":"mock/node-1","corr":"corr-1"}`)

	svc.handleDeviceCommand(fakeMessage{topic: hdp.CommandPrefix + "mock/node-1", payload: payload})

	if len(client.published) != 1 {
		t.Fatalf("expected one command_result publish, got %d", len(client.published))
	}
	if client.published[0].topic != hdp.CommandResultPrefix+"mock/node-1" {
		t.Fatalf("unexpected topic %q", client.published[0].topic)
	}
	var body map[string]any
	if err := json.Unmarshal(client.published[0].payload, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "rejected" {
		t.Fatalf("expected rejected status, got %v", body["status"])
	}
}

func TestHandleDeviceCommandContinuesTraceToResult(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	client := newFakeClient()
	service := New(client, Config{Enabled: true, AdapterID: "mock-adapter-1"})
	ctx, root := provider.Tracer("test").Start(t.Context(), "request")
	producerCtx, producer := observability.StartMessageProducer(ctx)
	envelope := map[string]any{"device_id": "mock/node-1", "corr": "corr-1", "ts": time.Now().UnixMilli()}
	observability.InjectMessageContext(producerCtx, envelope)
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	producer.End()
	service.handleDeviceCommand(fakeMessage{topic: hdp.CommandPrefix + "mock/node-1", payload: payload})
	if len(client.published) != 1 {
		t.Fatalf("expected one result, got %d", len(client.published))
	}
	_, resultConsumer := observability.StartMessageConsumer(t.Context(), client.published[0].payload, false, time.Now())
	resultConsumer.End()
	root.End()
	spans := recorder.Ended()
	if len(spans) != 5 {
		t.Fatalf("expected five spans, got %d", len(spans))
	}
	if spans[1].Parent().SpanID() != spans[2].SpanContext().SpanID() || spans[2].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[3].Parent().SpanID() != spans[1].SpanContext().SpanID() || spans[3].SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("command and result spans do not form a connected trace")
	}
	var result map[string]any
	if err := json.Unmarshal(client.published[0].payload, &result); err != nil || result["corr"] != "corr-1" {
		t.Fatalf("result correlation was not preserved: %v, %v", result, err)
	}
}

func TestDeviceIDHelpers(t *testing.T) {
	svc := New(newFakeClient(), Config{})
	if got := svc.hdpDeviceID("node-1"); got != "mock/node-1" {
		t.Fatalf("unexpected hdpDeviceID: %q", got)
	}
	proto, external := svc.externalFromHDP("mock/floor1/node-1")
	if proto != "mock" || external != "node-1" {
		t.Fatalf("unexpected externalFromHDP result: %q %q", proto, external)
	}
}
