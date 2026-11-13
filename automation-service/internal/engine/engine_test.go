package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/cachex"
	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/PetoAdam/homenavi/shared/observability"
	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type testCommandMQTT struct {
	topic     string
	payload   []byte
	onPublish func()
	err       error
}

func (client *testCommandMQTT) Publish(topic string, payload []byte) error {
	client.topic = topic
	client.payload = append([]byte(nil), payload...)
	if client.onPublish != nil {
		client.onPublish()
	}
	return client.err
}

func (*testCommandMQTT) SubscribeWithOptions(mqttx.SubscriptionOptions, func(mqttinfra.Message)) error {
	return nil
}

func TestDispatchCommandPropagatesWorkflowTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	client := &testCommandMQTT{}
	engine := New(nil, nil, Options{})
	engine.mq = client
	ctx, parent := provider.Tracer("test").Start(t.Context(), "workflow")
	if err := engine.dispatchCommand(ctx, "mock/device", "cmd-1", "set_state", ActionSendCommand{}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(client.payload, &payload); err != nil || client.topic != "homenavi/hdp/device/command/mock/device" || payload["corr"] != "cmd-1" || payload["trace"] == nil {
		t.Fatalf("unexpected command envelope: %v %v", payload, err)
	}
	_, consumer := observability.StartMessageConsumer(t.Context(), client.payload, false, time.Now())
	consumer.End()
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 3 || spans[0].Parent().SpanID() != spans[2].SpanContext().SpanID() || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[1].SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Fatal("automation MQTT command did not continue the workflow trace")
	}
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "false")
	if err := engine.dispatchCommand(ctx, "mock/device", "cmd-2", "set_state", ActionSendCommand{}); err != nil {
		t.Fatal(err)
	}
	payload = nil
	if err := json.Unmarshal(client.payload, &payload); err != nil || payload["trace"] != nil || payload["corr"] != "cmd-2" {
		t.Fatalf("disabled propagation changed command compatibility: %v %v", payload, err)
	}
}

type testStateMessage struct {
	payload  []byte
	retained bool
	topic    string
}

func (message testStateMessage) Topic() string {
	if message.topic != "" {
		return message.topic
	}
	return "homenavi/hdp/device/state/test"
}
func (message testStateMessage) Payload() []byte { return message.payload }
func (testStateMessage) Qos() byte               { return 0 }
func (message testStateMessage) Retained() bool  { return message.retained }
func (testStateMessage) Duplicate() bool         { return false }

type testTraceStore struct {
	values map[string][]byte
	ttl    time.Duration
	err    error
}

type blockedTraceStore struct {
	release <-chan struct{}
}

func (store *blockedTraceStore) Get(context.Context, string, any) error {
	<-store.release
	return errors.New("unavailable")
}

func (store *blockedTraceStore) Set(context.Context, string, any, time.Duration) error {
	<-store.release
	return errors.New("unavailable")
}

func (store *blockedTraceStore) Delete(context.Context, ...string) error {
	<-store.release
	return errors.New("unavailable")
}

func TestBlockedTraceStoreDoesNotBlockCommands(t *testing.T) {
	provider := trace.NewTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	release := make(chan struct{})
	defer close(release)
	client := &testCommandMQTT{}
	engine := New(nil, nil, Options{})
	engine.mq = client
	engine.traceStore = &blockedTraceStore{release: release}
	ctx, span := provider.Tracer("test").Start(t.Context(), "workflow")
	defer span.End()
	started := time.Now()
	if err := engine.dispatchCommand(ctx, "mock/device", "blocked-cache", "set_state", ActionSendCommand{WaitForResult: true}); err != nil {
		t.Fatal(err)
	}
	if client.topic == "" || time.Since(started) > 500*time.Millisecond {
		t.Fatal("trace cache blocked command dispatch")
	}
	started = time.Now()
	_, consumerSpan := engine.startCommandResultConsumer(ctx, testStateMessage{topic: hdp.CommandResultPrefix + "mock/device"}, HDPCommandResult{Envelope: hdp.Envelope{DeviceID: "mock/device", Corr: "blocked-cache"}})
	if consumerSpan != nil || time.Since(started) > 500*time.Millisecond {
		t.Fatal("trace cache blocked command result processing")
	}
}

type testPendingRepository struct {
	pending *dbinfra.PendingCorrelation
	err     error
}

func (repo *testPendingRepository) UpsertPendingCorr(_ context.Context, pending *dbinfra.PendingCorrelation) error {
	if repo.err != nil {
		return repo.err
	}
	repo.pending = pending
	return nil
}

func (repo *testPendingRepository) ConsumePendingCorr(_ context.Context, corr string) (*dbinfra.PendingCorrelation, error) {
	if repo.pending == nil || repo.pending.Corr != corr {
		return nil, nil
	}
	pending := repo.pending
	repo.pending = nil
	return pending, nil
}

func TestDispatchWithPendingCorrelation(t *testing.T) {
	repo := &testPendingRepository{}
	pending := &dbinfra.PendingCorrelation{Corr: "cmd-1"}
	if err := dispatchWithPendingCorrelation(t.Context(), repo, pending, func() error {
		if repo.pending != pending {
			t.Fatal("pending correlation was not stored before dispatch")
		}
		return errors.New("publish failed")
	}); err == nil || repo.pending != nil {
		t.Fatal("failed publish did not remove the pending correlation")
	}
	if err := dispatchWithPendingCorrelation(t.Context(), repo, pending, func() error {
		return errCommandOutcomeUnknown
	}); !errors.Is(err, errCommandOutcomeUnknown) || repo.pending == nil {
		t.Fatal("ambiguous publish did not retain the pending correlation")
	}
	repo.err = errors.New("database unavailable")
	called := false
	if err := dispatchWithPendingCorrelation(t.Context(), repo, pending, func() error {
		called = true
		return nil
	}); err == nil || called {
		t.Fatal("dispatch occurred without a pending correlation")
	}
}

func TestPendingDispatchRetriesPersistenceFailure(t *testing.T) {
	attempts := 0
	want := &dbinfra.PendingCorrelation{Corr: "command"}
	completed, err := retryPendingDispatch(t.Context(), time.Millisecond, func() (*dbinfra.PendingCorrelation, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("database unavailable")
		}
		return want, nil
	})
	if err != nil || completed != want || attempts != 2 {
		t.Fatalf("unexpected dispatch recovery: completed=%v attempts=%d err=%v", completed, attempts, err)
	}
}

func TestDispatchDoesNotWaitForRunEvents(t *testing.T) {
	repo := &testPendingRepository{}
	events := NewRunEventHub()
	engine := &Engine{events: events}
	pending := &dbinfra.PendingCorrelation{Corr: "cmd-1", RunID: uuid.New(), WorkflowID: uuid.New()}
	if err := dispatchWithPendingCorrelation(t.Context(), repo, pending, func() error {
		if repo.pending == nil {
			t.Fatal("missing pending correlation")
		}
		engine.publishRunEvent(pending.RunID, RunEvent{Type: "run_finished", Status: "success"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := events.replay[pending.RunID]
	if len(got) != 1 || got[0].Type != "run_finished" {
		t.Fatalf("unexpected event order: %v", got)
	}
}

func (store *testTraceStore) Get(_ context.Context, key string, value any) error {
	if store.err != nil {
		return store.err
	}
	data, ok := store.values[key]
	if !ok {
		return errors.New("cache miss")
	}
	return json.Unmarshal(data, value)
}

func (store *testTraceStore) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	if store.err != nil {
		return store.err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	store.values[key], store.ttl = data, ttl
	return nil
}

func (store *testTraceStore) Delete(_ context.Context, keys ...string) error {
	for _, key := range keys {
		delete(store.values, key)
	}
	return store.err
}

func TestUninstrumentedCommandResultUsesCachedTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "false")
	store := &testTraceStore{values: map[string][]byte{}}
	engine := New(nil, nil, Options{})
	engine.traceStore = store
	client := &testCommandMQTT{onPublish: func() {
		if _, ok := store.values[commandTraceKey("cmd-1")]; !ok {
			t.Error("trace context was not stored before publishing")
		}
	}}
	engine.mq = client
	ctx, root := provider.Tracer("test").Start(t.Context(), "workflow")
	if err := engine.dispatchCommand(ctx, "mock/device", "cmd-1", "set_state", ActionSendCommand{WaitForResult: true}); err != nil {
		t.Fatal(err)
	}
	if store.ttl != observability.DeferredTraceMaxAge || strings.Contains(string(client.payload), `"trace"`) {
		t.Fatal("trace cache lifetime or disabled wire propagation is incorrect")
	}
	result := HDPCommandResult{}
	payload := fmt.Appendf(nil, `{"schema":"hdp.v1","type":"command_result","device_id":"mock/device","corr":"cmd-1","ts":%d}`, time.Now().UnixMilli())
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	message := testStateMessage{payload: payload, topic: "homenavi/hdp/device/command_result/mock/device"}
	_, consumer := engine.startCommandResultConsumer(t.Context(), message, result)
	if consumer == nil {
		t.Fatal("missing result consumer span")
	}
	consumer.End()
	message.topic = "homenavi/hdp/device/command_result/mock/other"
	if _, span := engine.startCommandResultConsumer(t.Context(), message, result); span != nil {
		span.End()
		t.Fatal("mismatched topic must not use cached context")
	}
	message.topic = "homenavi/hdp/device/command_result/mock/device"
	message.retained = true
	_, replay := engine.startCommandResultConsumer(t.Context(), message, result)
	replay.End()
	root.End()
	spans := recorder.Ended()
	if len(spans) != 4 || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[1].SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("result did not continue the command producer trace")
	}
	if spans[2].SpanContext().TraceID() == root.SpanContext().TraceID() {
		t.Fatal("retained result must not continue the command trace")
	}
	store.err = errors.New("cache unavailable")
	client.onPublish = nil
	if err := engine.dispatchCommand(ctx, "mock/device", "cmd-2", "set_state", ActionSendCommand{WaitForResult: true}); err != nil {
		t.Fatalf("cache failure prevented publication: %v", err)
	}
	if _, span := engine.startCommandResultConsumer(t.Context(), message, result); span != nil {
		span.End()
		t.Fatal("cache failure must not invent a parent")
	}
	store.err = nil
	client.err = errors.New("publish failed")
	if err := engine.dispatchCommand(ctx, "mock/device", "cmd-3", "set_state", ActionSendCommand{WaitForResult: true}); err == nil {
		t.Fatal("expected publish failure")
	}
	if _, ok := store.values[commandTraceKey("cmd-3")]; ok {
		t.Fatal("failed publication left cached trace metadata")
	}
	if err := engine.dispatchCommand(ctx, "mock/device", "cmd-4", "set_state", ActionSendCommand{WaitForResult: true, Args: map[string]any{"invalid": make(chan int)}}); err == nil {
		t.Fatal("expected encoding failure")
	}
	if _, ok := store.values[commandTraceKey("cmd-4")]; ok {
		t.Fatal("failed encoding left cached trace metadata")
	}
}

func TestImmediateCommandResultRunLifecycle(t *testing.T) {
	dsn := os.Getenv("HOMENAVI_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL test endpoint is not configured")
	}
	ctx := t.Context()
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Exec(`CREATE TABLE IF NOT EXISTS hdp_devices (id uuid PRIMARY KEY, protocol text, external_id text)`).Error; err != nil {
		t.Fatal(err)
	}
	repo, err := dbinfra.New(database)
	if err != nil {
		t.Fatal(err)
	}
	consumerDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	consumerSQL, err := consumerDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = consumerSQL.Close() })
	consumerRepo, err := dbinfra.New(consumerDB)
	if err != nil {
		t.Fatal(err)
	}
	workflow := &dbinfra.Workflow{Name: "ordering test", CreatedBy: "test", Definition: datatypes.JSON(`{}`)}
	if err := repo.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.DeleteWorkflow(context.Background(), workflow.ID) })
	for _, test := range []struct {
		name                  string
		resultSuccess         bool
		delayed               bool
		expireDuringDispatch  bool
		expireAfterDispatch   bool
		publishErr            error
		runStatus, stepStatus string
	}{
		{name: "immediate success", resultSuccess: true, runStatus: "success", stepStatus: "success"},
		{name: "immediate failure", resultSuccess: false, runStatus: "failed", stepStatus: "success"},
		{name: "publish fails after result", resultSuccess: true, publishErr: errors.New("publish failed"), expireAfterDispatch: true, runStatus: "success", stepStatus: "success"},
		{name: "delayed success", resultSuccess: true, delayed: true, runStatus: "success", stepStatus: "success"},
		{name: "interrupted dispatch", resultSuccess: true, expireDuringDispatch: true, runStatus: "success", stepStatus: "success"},
		{name: "result timeout", resultSuccess: true, delayed: true, expireAfterDispatch: true, runStatus: "unknown", stepStatus: "success"},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := &dbinfra.WorkflowRun{WorkflowID: workflow.ID}
			if err := repo.CreateRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			events := NewRunEventHub()
			publisher := New(repo, nil, Options{RunEvents: events})
			consumer := New(consumerRepo, nil, Options{RunEvents: events})
			client := &testCommandMQTT{err: test.publishErr}
			var resultPayload []byte
			client.onPublish = func() {
				var command HDPCommand
				if err := json.Unmarshal(client.payload, &command); err != nil {
					t.Fatal(err)
				}
				resultPayload = fmt.Appendf(nil, `{"schema":"hdp.v1","type":"command_result","device_id":"mock/device","corr":%q,"success":%t,"error":"device failed"}`, command.Corr, test.resultSuccess)
				if test.delayed {
					return
				}
				var consumers sync.WaitGroup
				for range 2 {
					consumers.Add(1)
					go func() {
						defer consumers.Done()
						consumer.handleCommandResult(ctx, testStateMessage{payload: resultPayload})
					}()
				}
				consumers.Wait()
				if test.expireDuringDispatch {
					var recoverers sync.WaitGroup
					for _, instance := range []*Engine{publisher, consumer} {
						recoverers.Add(1)
						go func() {
							defer recoverers.Done()
							instance.expirePendingCommands(ctx, time.Now().Add(time.Hour))
						}()
					}
					recoverers.Wait()
					return
				}
				current, steps, err := repo.GetRunWithSteps(ctx, run.ID)
				if err != nil || current.Status != "running" || len(steps) != 1 || steps[0].Status != "running" {
					t.Fatalf("result completed run during dispatch: %#v %#v %v", current, steps, err)
				}
			}
			publisher.mq = client
			node := NodeDef{ID: "command", Kind: "action.send_command", Data: json.RawMessage(`{"targets":{"type":"device","ids":["mock/device"]},"command":"set_state","wait_for_result":true}`)}
			nodes := map[string]NodeDef{"command": node}
			publisher.executeRun(ctx, run.ID, workflow.ID, "trigger", nil, Definition{}, nodes, nil, []string{"command"})
			if test.delayed {
				current, steps, err := repo.GetRunWithSteps(ctx, run.ID)
				if err != nil || current.Status != "running" || len(steps) != 1 || steps[0].Status != "success" {
					t.Fatalf("unexpected waiting state: %#v %#v %v", current, steps, err)
				}
			}
			if test.expireAfterDispatch {
				publisher.expirePendingCommands(ctx, time.Now().Add(time.Hour))
			}
			consumer.handleCommandResult(ctx, testStateMessage{payload: resultPayload})
			current, steps, err := repo.GetRunWithSteps(ctx, run.ID)
			if err != nil || current.Status != test.runStatus || len(steps) != 1 || steps[0].Status != test.stepStatus {
				t.Fatalf("unexpected completed state: %#v %#v %v", current, steps, err)
			}
			var stepFinished, runFinished int
			for index, event := range events.replay[run.ID] {
				if event.Type == "node_finished" && event.NodeID == "command" {
					stepFinished = index + 1
				}
				if event.Type == "run_finished" {
					if runFinished != 0 {
						t.Fatal("duplicate run_finished")
					}
					runFinished = index + 1
				}
			}
			if stepFinished == 0 || runFinished <= stepFinished {
				t.Fatalf("unexpected event ordering: %v", events.replay[run.ID])
			}
		})
	}
	run := &dbinfra.WorkflowRun{WorkflowID: workflow.ID}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	step := &dbinfra.WorkflowRunStep{RunID: run.ID, NodeID: "unfinished"}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishRun(ctx, run.ID, "failed", "failure"); err == nil {
		t.Fatal("finished run with an unfinished step")
	}
	pending := &dbinfra.PendingCorrelation{Corr: uuid.NewString(), RunID: run.ID, WorkflowID: workflow.ID, DeviceID: "mock/device", ExpiresAt: time.Now().Add(time.Minute)}
	if err := repo.UpsertPendingCorr(ctx, pending); err != nil {
		t.Fatal(err)
	}
	status := "success"
	if _, err := consumerRepo.ResolvePendingCommand(ctx, pending.Corr, &status, ""); err == nil {
		t.Fatal("result finished run with an unfinished step")
	}
	if err := repo.FinishStep(ctx, step.ID, "success", ""); err != nil {
		t.Fatal(err)
	}
	if completed, err := consumerRepo.ResolvePendingCommand(ctx, pending.Corr, &status, ""); err != nil || completed == nil {
		t.Fatalf("result was lost after rollback: %#v %v", completed, err)
	}
	t.Run("committed step precedes completion event", func(t *testing.T) {
		run := &dbinfra.WorkflowRun{WorkflowID: workflow.ID}
		if err := repo.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		step := &dbinfra.WorkflowRunStep{RunID: run.ID, NodeID: "command"}
		if err := repo.CreateStep(ctx, step); err != nil {
			t.Fatal(err)
		}
		pending := &dbinfra.PendingCorrelation{Corr: uuid.NewString(), RunID: run.ID, WorkflowID: workflow.ID, DeviceID: "mock/device", DispatchPending: true, ExpiresAt: time.Now().Add(time.Minute)}
		if err := repo.UpsertPendingCorr(ctx, pending); err != nil {
			t.Fatal(err)
		}
		name := "test:rollback-dispatch-step"
		if err := database.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "workflow_run_steps" {
				tx.AddError(errors.New("injected transaction failure"))
			}
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = database.Callback().Update().Remove(name) })
		published := false
		if _, err := repo.FinishPendingDispatch(ctx, pending.Corr, step.ID, func() { published = true }); err == nil || published {
			t.Fatal("step event was published despite transaction failure")
		}
		if err := database.Callback().Update().Remove(name); err != nil {
			t.Fatal(err)
		}
		completed, err := repo.FinishPendingDispatch(ctx, pending.Corr, step.ID, func() {
			current, steps, err := consumerRepo.GetRunWithSteps(ctx, run.ID)
			if err != nil || current.Status != "running" || len(steps) != 1 || steps[0].Status != "success" {
				t.Fatalf("step event preceded committed state: %#v %#v %v", current, steps, err)
			}
			status := "success"
			if result, err := consumerRepo.ResolvePendingCommand(ctx, pending.Corr, &status, ""); err != nil || result == nil {
				t.Fatalf("result was blocked by the event callback: %#v %v", result, err)
			}
		})
		if err != nil || completed != nil {
			t.Fatalf("unexpected dispatch completion: %#v %v", completed, err)
		}
	})
	t.Run("step persistence failure stops traversal and retries failure", func(t *testing.T) {
		run := &dbinfra.WorkflowRun{WorkflowID: workflow.ID}
		if err := repo.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		var failures atomic.Int32
		name := "test:fail-step-persistence"
		if err := database.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
			if tx.Statement.Table == "workflow_run_steps" && failures.Load() < 2 {
				failures.Add(1)
				tx.AddError(errors.New("injected step update failure"))
			}
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = database.Callback().Update().Remove(name) })
		events := NewRunEventHub()
		engine := New(repo, nil, Options{RunEvents: events})
		first := NodeDef{ID: "first", Kind: "logic.sleep", Data: json.RawMessage(`{"duration_sec":0}`)}
		second := NodeDef{ID: "second", Kind: "logic.sleep", Data: json.RawMessage(`{"duration_sec":0}`)}
		engine.executeRun(ctx, run.ID, workflow.ID, "trigger", nil, Definition{}, map[string]NodeDef{"first": first, "second": second}, map[string][]string{"first": {"second"}}, []string{"first"})
		current, steps, err := repo.GetRunWithSteps(ctx, run.ID)
		if err != nil || failures.Load() != 2 || current.Status != "failed" || len(steps) != 1 || steps[0].NodeID != "first" || steps[0].Status != "failed" {
			t.Fatalf("unexpected failure recovery: %#v %#v %v (injections: %d)", current, steps, err, failures.Load())
		}
	})
	t.Run("staged result commits before event callback", func(t *testing.T) {
		run := &dbinfra.WorkflowRun{WorkflowID: workflow.ID}
		if err := repo.CreateRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		step := &dbinfra.WorkflowRunStep{RunID: run.ID, NodeID: "command"}
		if err := repo.CreateStep(ctx, step); err != nil {
			t.Fatal(err)
		}
		pending := &dbinfra.PendingCorrelation{Corr: uuid.NewString(), RunID: run.ID, WorkflowID: workflow.ID, DeviceID: "mock/device", DispatchPending: true, ExpiresAt: time.Now().Add(time.Minute)}
		if err := repo.UpsertPendingCorr(ctx, pending); err != nil {
			t.Fatal(err)
		}
		status := "success"
		if result, err := consumerRepo.ResolvePendingCommand(ctx, pending.Corr, &status, ""); err != nil || result != nil {
			t.Fatalf("result was not staged: %#v %v", result, err)
		}
		completed, err := repo.FinishPendingDispatch(ctx, pending.Corr, step.ID, func() {
			current, steps, err := consumerRepo.GetRunWithSteps(ctx, run.ID)
			if err != nil || current.Status != "success" || len(steps) != 1 || steps[0].Status != "success" {
				t.Fatalf("result was not committed before callback: %#v %#v %v", current, steps, err)
			}
		})
		if err != nil || completed == nil {
			t.Fatalf("staged completion failed: %#v %v", completed, err)
		}
	})
}

func TestConcurrentAutomationSchemaInitialization(t *testing.T) {
	dsn := os.Getenv("HOMENAVI_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL test endpoint is not configured")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer adminSQL.Close()
	schema := "automation_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error; err != nil {
			t.Error(err)
		}
	}()
	if err := admin.Exec(`CREATE TABLE "` + schema + `".hdp_devices (id uuid PRIMARY KEY, protocol text, external_id text)`).Error; err != nil {
		t.Fatal(err)
	}
	var databases []*gorm.DB
	for range 4 {
		database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		connection, err := database.DB()
		if err != nil {
			t.Fatal(err)
		}
		connection.SetMaxOpenConns(1)
		defer connection.Close()
		if err := database.Exec(`SET search_path TO "` + schema + `"`).Error; err != nil {
			t.Fatal(err)
		}
		databases = append(databases, database)
	}
	initialize := func() {
		start := make(chan struct{})
		results := make(chan error, len(databases))
		for _, database := range databases {
			go func() {
				<-start
				_, err := dbinfra.New(database)
				results <- err
			}()
		}
		close(start)
		for range databases {
			if err := <-results; err != nil {
				t.Errorf("concurrent schema initialization: %v", err)
			}
		}
	}
	initialize()
	if t.Failed() {
		return
	}
	if err := databases[0].Exec("ALTER TABLE pending_correlations DROP COLUMN dispatch_pending, DROP COLUMN result_status, DROP COLUMN result_error").Error; err != nil {
		t.Fatal(err)
	}
	for index, database := range databases {
		connection, err := database.DB()
		if err != nil {
			t.Fatal(err)
		}
		if err := connection.Close(); err != nil {
			t.Fatal(err)
		}
		replacement, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		replacementConnection, err := replacement.DB()
		if err != nil {
			t.Fatal(err)
		}
		replacementConnection.SetMaxOpenConns(1)
		defer replacementConnection.Close()
		if err := replacement.Exec(`SET search_path TO "` + schema + `"`).Error; err != nil {
			t.Fatal(err)
		}
		databases[index] = replacement
	}
	initialize()
}

func TestUninstrumentedResultAcrossInstances(t *testing.T) {
	address := os.Getenv("HOMENAVI_TEST_REDIS_ADDR")
	dsn := os.Getenv("HOMENAVI_TEST_POSTGRES_DSN")
	if address == "" || dsn == "" {
		t.Skip("isolated Redis and PostgreSQL test endpoints are not configured")
	}
	ctx := t.Context()
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Exec(`CREATE TABLE IF NOT EXISTS hdp_devices (id uuid PRIMARY KEY, protocol text, external_id text)`).Error; err != nil {
		t.Fatal(err)
	}
	repo, err := dbinfra.New(database)
	if err != nil {
		t.Fatal(err)
	}
	workflow := &dbinfra.Workflow{Name: "trace test", CreatedBy: "test", Definition: datatypes.JSON(`{}`)}
	if err := repo.CreateWorkflow(ctx, workflow); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.DeleteWorkflow(context.Background(), workflow.ID) })
	run := &dbinfra.WorkflowRun{WorkflowID: workflow.ID}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	store, err := cachex.NewJSONStore(ctx, redisx.Config{Addrs: []string{address}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	redisClient := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = redisClient.Close() })
	recorder := tracetest.NewSpanRecorder()
	providerOptions := []trace.TracerProviderOption{trace.WithSpanProcessor(recorder)}
	collectorURL := os.Getenv("HOMENAVI_TEST_JAEGER_OTLP")
	queryURL := os.Getenv("HOMENAVI_TEST_JAEGER_QUERY")
	if collectorURL != "" && queryURL != "" {
		exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(collectorURL+"/v1/traces"))
		if err != nil {
			t.Fatal(err)
		}
		providerOptions = append(providerOptions, trace.WithSyncer(exporter))
	}
	provider := trace.NewTracerProvider(providerOptions...)
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "false")
	correlation := "trace-test-" + uuid.NewString()
	events := NewRunEventHub()
	publisher := New(repo, nil, Options{TraceStore: store, RunEvents: events})
	consumer := New(repo, nil, Options{TraceStore: store, RunEvents: events})
	client := &testCommandMQTT{onPublish: func() {
		ttl, err := redisClient.PTTL(ctx, commandTraceKey(correlation)).Result()
		if err != nil || ttl <= 0 || ttl > observability.DeferredTraceMaxAge {
			t.Errorf("unexpected Redis trace TTL %s: %v", ttl, err)
		}
		payload := fmt.Appendf(nil, `{"schema":"hdp.v1","type":"command_result","device_id":"mock/device","corr":%q,"ts":%d,"success":true}`, correlation, time.Now().UnixMilli())
		consumer.handleCommandResult(ctx, testStateMessage{payload: payload, topic: "homenavi/hdp/device/command_result/mock/device"})
	}}
	publisher.mq = client
	workflowCtx, root := provider.Tracer("test").Start(ctx, "workflow")
	now := time.Now()
	pending := &dbinfra.PendingCorrelation{Corr: correlation, RunID: run.ID, WorkflowID: workflow.ID, DeviceID: "mock/device", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := dispatchWithPendingCorrelation(workflowCtx, repo, pending, func() error {
		return publisher.dispatchCommand(workflowCtx, pending.DeviceID, correlation, "set_state", ActionSendCommand{WaitForResult: true})
	}); err != nil {
		t.Fatal(err)
	}
	root.End()
	updated, _, err := repo.GetRunWithSteps(ctx, run.ID)
	if err != nil || updated.Status != "success" {
		t.Fatalf("immediate result did not complete run: %#v %v", updated, err)
	}
	if got := events.replay[run.ID]; len(got) != 1 || got[0].Type != "run_finished" {
		t.Fatalf("unexpected immediate-result event order: %v", got)
	}
	if exists, err := redisClient.Exists(ctx, commandTraceKey(correlation)).Result(); err != nil || exists != 0 {
		t.Fatalf("consumed trace metadata remains in Redis: %d %v", exists, err)
	}
	spans := recorder.Ended()
	if len(spans) != 3 || spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() || spans[0].SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("result did not continue the cross-instance command trace")
	}
	if collectorURL != "" && queryURL != "" {
		if err := provider.ForceFlush(ctx); err != nil {
			t.Fatal(err)
		}
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		httpClient := &http.Client{Timeout: time.Second}
		for {
			response, queryErr := httpClient.Get(queryURL + "/api/traces/" + root.SpanContext().TraceID().String())
			if queryErr == nil {
				var result struct {
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
				decodeErr := json.NewDecoder(response.Body).Decode(&result)
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK && decodeErr == nil && len(result.Data) > 0 && len(result.Data[0].Spans) >= 3 {
					parents := map[string]string{}
					for _, span := range result.Data[0].Spans {
						for _, reference := range span.References {
							if reference.RefType == "CHILD_OF" {
								parents[span.SpanID] = reference.SpanID
							}
						}
					}
					if parents[spans[0].SpanContext().SpanID().String()] != spans[1].SpanContext().SpanID().String() || parents[spans[1].SpanContext().SpanID().String()] != root.SpanContext().SpanID().String() {
						t.Fatal("Jaeger did not preserve the automation result parent graph")
					}
					break
				}
			}
			select {
			case <-deadline.C:
				t.Fatal("Jaeger did not return the automation result trace")
			case <-ticker.C:
			}
		}
	}
}

func TestStateConsumerTraceAndRetainedReplay(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	ctx, producer := observability.StartMessageProducer(t.Context())
	envelope := map[string]any{"schema": "hdp.v1", "type": "state", "ts": time.Now().UnixMilli(), "device_id": "test", "state": map[string]any{"on": true}}
	observability.InjectMessageContext(ctx, envelope)
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	producer.End()
	engine := New(nil, nil, Options{})
	engine.handleState(t.Context(), testStateMessage{payload: payload})
	engine.handleState(t.Context(), testStateMessage{payload: payload, retained: true})
	spans := recorder.Ended()
	if len(spans) != 3 || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[1].SpanContext().TraceID() != spans[0].SpanContext().TraceID() {
		t.Fatal("state consumer did not continue the producer trace")
	}
	if spans[2].SpanContext().TraceID() == spans[0].SpanContext().TraceID() || len(spans[2].Links()) != 1 || spans[2].Links()[0].SpanContext.SpanID() != spans[0].SpanContext().SpanID() {
		t.Fatal("retained state did not start a linked trace")
	}
	engine.handleState(t.Context(), testStateMessage{payload: []byte(`{"schema":"hdp.v1","type":"state","device_id":"test","state":{"on":false},"trace":"invalid"}`)})
	if len(recorder.Ended()) != 3 || engine.deviceStates["test"]["on"] != false {
		t.Fatal("invalid trace metadata changed state processing")
	}
}

func TestCommandResultConsumerTraceAndRetainedReplay(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	ctx, producer := observability.StartMessageProducer(t.Context())
	envelope := map[string]any{"schema": "hdp.v1", "type": "command_result", "ts": time.Now().UnixMilli()}
	observability.InjectMessageContext(ctx, envelope)
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	producer.End()
	engine := New(nil, nil, Options{})
	engine.handleCommandResult(t.Context(), testStateMessage{payload: payload})
	engine.handleCommandResult(t.Context(), testStateMessage{payload: payload, retained: true})
	engine.handleCommandResult(t.Context(), testStateMessage{payload: []byte(`{"schema":"hdp.v1","type":"command_result","trace":"invalid"}`)})
	spans := recorder.Ended()
	if len(spans) != 3 || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() {
		t.Fatal("result consumer did not continue the producer trace")
	}
	if spans[2].SpanContext().TraceID() == spans[0].SpanContext().TraceID() || len(spans[2].Links()) != 1 || spans[2].Links()[0].SpanContext.SpanID() != spans[0].SpanContext().SpanID() {
		t.Fatal("retained result did not start a linked trace")
	}
}

func TestWorkflowExecutionKeepsTraceWithoutRequestCancellation(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(t.Context())
	})
	requestContext, cancel := context.WithCancel(t.Context())
	_, parent := provider.Tracer("test").Start(requestContext, "request")
	cancel()
	parent.End()
	runContext, runSpan := startWorkflowExecution(parent.SpanContext())
	if runContext.Err() != nil {
		t.Fatal("workflow inherited request cancellation")
	}
	runSpan.End()
	spans := recorder.Ended()
	if len(spans) != 2 || spans[1].Parent().SpanID() != spans[0].SpanContext().SpanID() || spans[1].SpanContext().TraceID() != spans[0].SpanContext().TraceID() {
		t.Fatal("workflow execution did not continue the request trace")
	}
}

func TestMatchStateTrigger_EmptyKeyMatchesAny(t *testing.T) {
	tr := TriggerDeviceState{Key: "", Op: "exists"}
	state := map[string]any{"motion": true}
	if !matchStateTrigger(tr, state) {
		t.Fatalf("expected empty key to match any state")
	}
}

func TestMatchStateTrigger_Exists(t *testing.T) {
	tr := TriggerDeviceState{Key: "motion", Op: "exists"}
	if !matchStateTrigger(tr, map[string]any{"motion": true}) {
		t.Fatalf("expected exists to match when key present")
	}
	if matchStateTrigger(tr, map[string]any{"temperature": 21}) {
		t.Fatalf("expected exists to not match when key missing")
	}
}

func TestMatchStateTrigger_Eq_NumberLoose(t *testing.T) {
	want, _ := json.Marshal(42)
	tr := TriggerDeviceState{Key: "x", Op: "eq", Value: want}
	if !matchStateTrigger(tr, map[string]any{"x": float64(42)}) {
		t.Fatalf("expected eq to match numeric equality")
	}
}

func TestMatchStateTrigger_Comparators(t *testing.T) {
	want, _ := json.Marshal(10)
	state := map[string]any{"temp": 12}

	cases := []struct {
		op   string
		want bool
	}{
		{"gt", true},
		{"gte", true},
		{"lt", false},
		{"lte", false},
	}
	for _, c := range cases {
		tr := TriggerDeviceState{Key: "temp", Op: c.op, Value: want}
		if got := matchStateTrigger(tr, state); got != c.want {
			t.Fatalf("op=%s: expected %v, got %v", c.op, c.want, got)
		}
	}
}

func TestMatchStateTrigger_Neq(t *testing.T) {
	want, _ := json.Marshal("ON")
	tr := TriggerDeviceState{Key: "state", Op: "neq", Value: want}
	if !matchStateTrigger(tr, map[string]any{"state": "OFF"}) {
		t.Fatalf("expected neq to match when values differ")
	}
	if matchStateTrigger(tr, map[string]any{"state": "ON"}) {
		t.Fatalf("expected neq to not match when values equal")
	}
}

func TestMatchStateTrigger_ChangedRequiresPriorDifferentValue(t *testing.T) {
	trigger := TriggerDeviceState{Key: "state", Op: "changed"}
	if matchStateTriggerWithPrevious(trigger, map[string]any{"state": "ON"}, nil) {
		t.Fatal("first observation must not count as changed")
	}
	if matchStateTriggerWithPrevious(trigger, map[string]any{"state": "ON"}, map[string]any{"state": "ON"}) {
		t.Fatal("identical values must not count as changed")
	}
	if !matchStateTriggerWithPrevious(trigger, map[string]any{"state": "OFF"}, map[string]any{"state": "ON"}) {
		t.Fatal("different values must count as changed")
	}
}

func TestMatchStateTrigger_ConditionsRequireEverySameDeviceCondition(t *testing.T) {
	brightness, _ := json.Marshal(80)
	color, _ := json.Marshal("warm")
	trigger := TriggerDeviceState{Conditions: []StateCondition{
		{Key: "brightness", Op: "gte", Value: brightness},
		{Key: "color_mode", Op: "eq", Value: color},
	}}
	if !matchStateTrigger(trigger, map[string]any{"brightness": 90, "color_mode": "warm"}) {
		t.Fatal("expected all same-device conditions to match")
	}
	if matchStateTrigger(trigger, map[string]any{"brightness": 90, "color_mode": "cool"}) {
		t.Fatal("expected a failing condition to block the trigger")
	}
}

func TestRecordDeviceState_MergesSeparateCapabilityUpdates(t *testing.T) {
	engine := New(nil, nil, Options{})
	_, _ = engine.recordDeviceState("lamp", map[string]any{"brightness": 90})
	_, current := engine.recordDeviceState("lamp", map[string]any{"color_mode": "warm"})
	if current["brightness"] != 90 || current["color_mode"] != "warm" {
		t.Fatalf("expected merged state, got %v", current)
	}
}

func TestMatchesAggregation_AllRequiresEveryTargetToMatch(t *testing.T) {
	engine := New(nil, nil, Options{})
	engine.recordDeviceState("device-a", map[string]any{"temperature": 22.0})
	engine.recordDeviceState("device-b", map[string]any{"temperature": 19.0})
	trigger := TriggerDeviceState{Targets: NodeTargets{Type: "device", IDs: []string{"device-a", "device-b"}}, Key: "temperature", Op: "gte", Value: json.RawMessage("20"), Aggregation: "all"}
	if engine.matchesAggregation(context.Background(), trigger, "device-a") {
		t.Fatal("all aggregation must fail while one target does not match")
	}
	engine.recordDeviceState("device-b", map[string]any{"temperature": 21.0})
	if !engine.matchesAggregation(context.Background(), trigger, "device-b") {
		t.Fatal("all aggregation must match once every target matches")
	}
}

func TestDebounceElapsedWaitsForConfiguredDuration(t *testing.T) {
	engine := New(nil, nil, Options{})
	workflowID := uuid.New()
	if engine.debounceElapsed(workflowID, "trigger", "device", 1) {
		t.Fatal("first matching observation must start debounce")
	}
	engine.stateMu.Lock()
	engine.debounces[workflowID.String()+":trigger:device"] = time.Now().Add(-time.Second)
	engine.stateMu.Unlock()
	if !engine.debounceElapsed(workflowID, "trigger", "device", 1) {
		t.Fatal("elapsed debounce must allow the trigger")
	}
}

func TestDispatchCapabilityStatePatchUsesDeviceHub(t *testing.T) {
	var gotPath, gotCorrelation string
	var gotState map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		gotPath = request.URL.Path
		var payload struct {
			State         map[string]any `json:"state"`
			CorrelationID string         `json:"correlation_id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		gotState, gotCorrelation = payload.State, payload.CorrelationID
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	engine := New(nil, nil, Options{DeviceHubURL: server.URL})
	if err := engine.dispatchCapabilityStatePatch(context.Background(), "test/device", "corr-1", map[string]any{"setpoint": 21}); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if gotPath != "/api/hdp/devices/test/device/commands" || gotCorrelation != "corr-1" || gotState["setpoint"] != float64(21) {
		t.Fatalf("unexpected Device Hub request path=%q correlation=%q state=%v", gotPath, gotCorrelation, gotState)
	}
}

func TestDispatchCapabilityStatePatchSurfacesDeviceHubRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "property is read-only", http.StatusBadRequest)
	}))
	defer server.Close()
	engine := New(nil, nil, Options{DeviceHubURL: server.URL})
	err := engine.dispatchCapabilityStatePatch(context.Background(), "device", "corr", map[string]any{"temperature": 21})
	if err == nil || !strings.Contains(err.Error(), "property is read-only") {
		t.Fatalf("expected Device Hub rejection, got %v", err)
	}
}

func TestShouldIgnoreRetainedState_DropsOldRetainedSnapshot(t *testing.T) {
	e := New(nil, nil, Options{})
	connectedAt := time.Now().UTC()
	e.noteMQTTConnected(connectedAt)
	stateTS := connectedAt.Add(-30 * time.Second).UnixMilli()
	if !e.shouldIgnoreRetainedState(true, stateTS) {
		t.Fatalf("expected old retained snapshot to be ignored")
	}
}

func TestShouldIgnoreRetainedState_AllowsFreshRetainedLiveUpdate(t *testing.T) {
	e := New(nil, nil, Options{})
	connectedAt := time.Now().UTC()
	e.noteMQTTConnected(connectedAt)
	stateTS := connectedAt.Add(2 * time.Second).UnixMilli()
	if e.shouldIgnoreRetainedState(true, stateTS) {
		t.Fatalf("expected fresh retained live update to be processed")
	}
}

func TestShouldIgnoreRetainedState_IgnoresUnknownTimestamp(t *testing.T) {
	e := New(nil, nil, Options{})
	e.noteMQTTConnected(time.Now().UTC())
	if !e.shouldIgnoreRetainedState(true, 0) {
		t.Fatalf("expected retained message with missing timestamp to be ignored")
	}
}
