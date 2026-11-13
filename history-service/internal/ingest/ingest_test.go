package ingest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/history-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeMsg struct {
	topic    string
	payload  []byte
	retained bool
}

func (m fakeMsg) Topic() string   { return m.topic }
func (m fakeMsg) Payload() []byte { return m.payload }
func (m fakeMsg) Retained() bool  { return m.retained }

func openRepo(t *testing.T) *dbinfra.Repository {
	t.Helper()
	// Use a unique in-memory DB per test (still shared cache within that DB)
	// to avoid cross-test contamination when tests run in parallel.
	dsn := "file:ingest_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := dbinfra.New(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repo
}

func TestParseDeviceID(t *testing.T) {
	id, err := ParseDeviceID("homenavi/hdp/device/state/", "homenavi/hdp/device/state/zigbee/0xabc")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id != "zigbee/0xabc" {
		t.Fatalf("expected zigbee/0xabc, got %q", id)
	}
}

func TestHandleMessageStoresValidJSON(t *testing.T) {
	repo := openRepo(t)
	ing := &Ingestor{Repo: repo, StatePrefix: "homenavi/hdp/device/state/", AllowRetains: true}
	msg := fakeMsg{topic: "homenavi/hdp/device/state/zigbee/0x1", payload: []byte(`{"on":true}`), retained: false}
	ing.HandleMessage(context.Background(), msg, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	page, err := repo.ListStatePoints(context.Background(), "zigbee/0x1", time.Time{}, time.Time{}, 10, nil, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Points) != 1 {
		t.Fatalf("expected 1 row, got %d", len(page.Points))
	}
}

func TestHandleMessageRejectsInvalidJSON(t *testing.T) {
	repo := openRepo(t)
	ing := &Ingestor{Repo: repo, StatePrefix: "homenavi/hdp/device/state/", AllowRetains: true}
	msg := fakeMsg{topic: "homenavi/hdp/device/state/zigbee/0x1", payload: []byte(`{not-json}`), retained: false}
	ing.HandleMessage(context.Background(), msg, time.Now().UTC())

	page, err := repo.ListStatePoints(context.Background(), "zigbee/0x1", time.Time{}, time.Time{}, 10, nil, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Points) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(page.Points))
	}
}

func TestHandleMessageContinuesTraceAndLinksRetainedReplay(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	repo := openRepo(t)
	ing := &Ingestor{Repo: repo, StatePrefix: "homenavi/hdp/device/state/", AllowRetains: true}
	producerCtx, producer := observability.StartMessageProducer(context.Background())
	now := time.Now()
	envelope := map[string]any{"schema": "hdp.v1", "type": "state", "ts": now.UnixMilli(), "state": map[string]any{"on": true}}
	observability.InjectMessageContext(producerCtx, envelope)
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	producer.End()
	msg := fakeMsg{topic: "homenavi/hdp/device/state/zigbee/0x1", payload: payload}
	ing.HandleMessage(context.Background(), msg, now)
	msg.retained = true
	ing.HandleMessage(context.Background(), msg, now)
	msg.retained = false
	msg.payload = []byte(`{"trace":"invalid","state":{"on":false}}`)
	ing.HandleMessage(context.Background(), msg, now)
	spans := recorder.Ended()
	if len(spans) != 3 || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[1].SpanContext().TraceID() != spans[0].SpanContext().TraceID() {
		t.Fatal("history did not continue the producer trace")
	}
	if spans[2].SpanContext().TraceID() == spans[0].SpanContext().TraceID() || len(spans[2].Links()) != 1 || spans[2].Links()[0].SpanContext.SpanID() != spans[0].SpanContext().SpanID() {
		t.Fatal("retained history ingestion did not link a new trace")
	}
	page, err := repo.ListStatePoints(context.Background(), "zigbee/0x1", time.Time{}, time.Time{}, 10, nil, false)
	if err != nil || len(page.Points) != 3 {
		t.Fatalf("optional trace metadata changed history ingestion: %d %v", len(page.Points), err)
	}
}
