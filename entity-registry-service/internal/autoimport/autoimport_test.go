package autoimport

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testMessage struct {
	topic    string
	payload  []byte
	retained bool
}

func (message testMessage) Topic() string   { return message.topic }
func (message testMessage) Payload() []byte { return message.payload }
func (message testMessage) Retained() bool  { return message.retained }
func (testMessage) Qos() byte               { return 0 }
func (testMessage) Duplicate() bool         { return false }

func TestConsumeMessageContinuesTrace(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:trace-autoimport?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := dbinfra.New(database)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.EnsureDeviceForHDP(t.Context(), "zigbee/trace", "Trace", ""); err != nil {
		t.Fatal(err)
	}
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	ctx, producer := observability.StartMessageProducer(t.Context())
	envelope := map[string]any{"schema": "hdp.v1", "type": "event", "event": "device_removed", "device_id": "zigbee/trace", "ts": time.Now().UnixMilli()}
	observability.InjectMessageContext(ctx, envelope)
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	producer.End()
	runner := &Runner{repo: repo}
	msg := testMessage{topic: "homenavi/hdp/device/event/zigbee/trace", payload: payload}
	runner.consumeMessage(t.Context(), msg)
	msg.retained = true
	runner.consumeMessage(t.Context(), msg)
	spans := recorder.Ended()
	if len(spans) != 3 || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[1].SpanContext().TraceID() != spans[0].SpanContext().TraceID() {
		t.Fatal("registry consumer did not continue the event trace")
	}
	if spans[2].SpanContext().TraceID() == spans[0].SpanContext().TraceID() || len(spans[2].Links()) != 1 {
		t.Fatal("retained event did not start a linked trace")
	}
	if _, found, err := repo.FindDeviceIDByHDPExternalID(t.Context(), "zigbee/trace"); err != nil || found {
		t.Fatalf("device removal failed: found=%v err=%v", found, err)
	}
}

func TestAutoImport_DeviceRemovedDeletesBoundDevice(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := dbinfra.New(db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	r := &Runner{repo: repo}

	hdpID := "zigbee/0xdeadbeef"
	// Ensure autoimport created it.
	if _, _, err := repo.EnsureDeviceForHDP(ctx, hdpID, "Test Device", ""); err != nil {
		t.Fatalf("ensure device: %v", err)
	}

	env := map[string]any{
		"schema":    "hdp.v1",
		"type":      "event",
		"device_id": hdpID,
		"event":     "device_removed",
		"ts":        123,
		"data": map[string]any{
			"reason": "test",
		},
	}
	payload, _ := json.Marshal(env)
	r.handleMessage(ctx, "homenavi/hdp/device/event/"+hdpID, payload)

	// Should be gone.
	_, ok, err := repo.FindDeviceIDByHDPExternalID(ctx, hdpID)
	if err != nil {
		t.Fatalf("find device id: %v", err)
	}
	if ok {
		t.Fatalf("expected device to be deleted")
	}
}

func TestAutoImport_DeviceRemovedUnbindsWhenMultipleBindings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := dbinfra.New(db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	r := &Runner{repo: repo}

	id, _, err := repo.EnsureDeviceForHDP(ctx, "zigbee/0x001", "Lamp", "")
	if err != nil {
		t.Fatalf("ensure device: %v", err)
	}
	// Add a second binding.
	if err := repo.SetDeviceHDPBindings(ctx, id, []string{"zigbee/0x001", "thread/abcd"}); err != nil {
		t.Fatalf("set bindings: %v", err)
	}

	env := map[string]any{
		"schema":    "hdp.v1",
		"type":      "event",
		"device_id": "zigbee/0x001",
		"event":     "device_removed",
		"ts":        123,
	}
	payload, _ := json.Marshal(env)
	r.handleMessage(ctx, "homenavi/hdp/device/event/zigbee/0x001", payload)

	view, err := repo.GetDeviceView(ctx, id)
	if err != nil {
		t.Fatalf("get view: %v", err)
	}
	if len(view.HDPExternalIDs) != 1 || view.HDPExternalIDs[0] != "thread/abcd" {
		t.Fatalf("expected remaining binding [thread/abcd], got %v", view.HDPExternalIDs)
	}
}
