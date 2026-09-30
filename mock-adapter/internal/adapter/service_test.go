package adapter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/mockdemo"
)

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

	if countPublishedWithPrefix(client.published, hdp.MetadataPrefix+"mock/") != 8 {
		t.Fatalf("expected metadata for 8 demo devices, got %d", countPublishedWithPrefix(client.published, hdp.MetadataPrefix+"mock/"))
	}
	if countPublishedWithPrefix(client.published, hdp.StatePrefix+"mock/") != 8 {
		t.Fatalf("expected initial state for 8 demo devices, got %d", countPublishedWithPrefix(client.published, hdp.StatePrefix+"mock/"))
	}
	if _, ok := client.subscribed[hdp.PairingCommandPrefix+"mock"]; !ok {
		t.Fatal("expected pairing subscription")
	}
	if _, ok := client.subscribed[hdp.CommandPrefix+"mock/#"]; !ok {
		t.Fatal("expected command subscription")
	}
	if !hasRetainedTopic(client.published, hdp.AdapterStatusPrefix+"mock-adapter-1") {
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

func TestHandleDeviceCommandAppliesStateAndPublishesResult(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "mock-adapter-1", Version: "dev"})
	svc.initDemoCatalog()
	payload := []byte(`{"device_id":"mock/sofa-lamp","corr":"corr-1","command":"set_state","args":{"on":true,"brightness":73}}`)

	svc.handleDeviceCommand(fakeMessage{topic: hdp.CommandPrefix + "mock/sofa-lamp", payload: payload})

	if len(client.published) != 2 {
		t.Fatalf("expected state and command_result publish, got %d", len(client.published))
	}
	if client.published[0].topic != hdp.StatePrefix+mockdemo.SofaLampHDPDeviceID {
		t.Fatalf("unexpected topic %q", client.published[0].topic)
	}
	if client.published[1].topic != hdp.CommandResultPrefix+mockdemo.SofaLampHDPDeviceID {
		t.Fatalf("unexpected topic %q", client.published[1].topic)
	}
	var stateBody map[string]any
	if err := json.Unmarshal(client.published[0].payload, &stateBody); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	state, _ := stateBody["state"].(map[string]any)
	if state["brightness"] != float64(73) || state["on"] != true {
		t.Fatalf("expected updated state, got %#v", state)
	}
	var body map[string]any
	if err := json.Unmarshal(client.published[1].payload, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "applied" || body["success"] != true {
		t.Fatalf("expected applied success result, got %#v", body)
	}
}

func TestHandleDeviceCommandRejectsUnsupportedSensorCommand(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "mock-adapter-1", Version: "dev"})
	svc.initDemoCatalog()
	payload := []byte(`{"device_id":"mock/entry-sensor","corr":"corr-2","command":"set_state","args":{"contact":true}}`)

	svc.handleDeviceCommand(fakeMessage{topic: hdp.CommandPrefix + "mock/entry-sensor", payload: payload})

	if len(client.published) != 1 {
		t.Fatalf("expected one command_result publish, got %d", len(client.published))
	}
	var body map[string]any
	if err := json.Unmarshal(client.published[0].payload, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "rejected" {
		t.Fatalf("expected rejected status, got %v", body["status"])
	}
}

func TestAdvanceDemoStatesMutatesRuntimeState(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "mock-adapter-1", Version: "dev"})
	svc.initDemoCatalog()

	updates := svc.advanceDemoStates()
	if len(updates) == 0 {
		t.Fatal("expected demo state updates")
	}
	foundCoffee := false
	for _, update := range updates {
		if update.HDPDeviceID != mockdemo.CoffeeMakerHDPDeviceID {
			continue
		}
		foundCoffee = true
		if update.State["power"] != "on" {
			t.Fatalf("expected coffee maker to cycle on, got %#v", update.State)
		}
	}
	if !foundCoffee {
		t.Fatal("expected coffee maker update")
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

func countPublishedWithPrefix(messages []publishedMessage, prefix string) int {
	count := 0
	for _, msg := range messages {
		if strings.HasPrefix(msg.topic, prefix) {
			count++
		}
	}
	return count
}

func hasRetainedTopic(messages []publishedMessage, topic string) bool {
	for _, msg := range messages {
		if msg.topic == topic && msg.retain {
			return true
		}
	}
	return false
}
