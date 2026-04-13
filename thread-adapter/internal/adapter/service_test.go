package adapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	paho "github.com/eclipse/paho.mqtt.golang"
)

type publishedMessage struct {
	topic   string
	payload []byte
	retain  bool
}

type fakeClient struct {
	subscribed map[string]mqttx.Handler
	published  []publishedMessage
}

func newFakeClient() *fakeClient {
	return &fakeClient{subscribed: map[string]mqttx.Handler{}}
}

func (f *fakeClient) Publish(topic string, payload []byte) error {
	f.published = append(f.published, publishedMessage{topic: topic, payload: payload})
	return nil
}

func (f *fakeClient) PublishWith(topic string, payload []byte, retain bool) error {
	f.published = append(f.published, publishedMessage{topic: topic, payload: payload, retain: retain})
	return nil
}

func (f *fakeClient) Subscribe(topic string, cb mqttx.Handler) error {
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

var _ paho.Message = fakeMessage{}

func TestStartPublishesHelloAndStatusAndSubscribes(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "thread-adapter-1", Version: "dev"})

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer svc.Stop()

	if len(client.published) < 2 {
		t.Fatalf("expected hello and status messages, got %d", len(client.published))
	}
	if _, ok := client.subscribed[hdp.PairingCommandPrefix+"thread"]; !ok {
		t.Fatal("expected pairing subscription")
	}
	if _, ok := client.subscribed[hdp.CommandPrefix+"thread/#"]; !ok {
		t.Fatal("expected command subscription")
	}
	if client.published[1].topic != hdp.AdapterStatusPrefix+"thread-adapter-1" || !client.published[1].retain {
		t.Fatal("expected retained adapter status publish")
	}
}

func TestHandlePairingStartPublishesProgress(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "thread-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "thread", payload: []byte(`{"action":"start"}`)})

	if len(client.published) != 2 {
		t.Fatalf("expected two progress messages, got %d", len(client.published))
	}
	for _, msg := range client.published {
		if msg.topic != hdp.PairingProgressPrefix+"thread" {
			t.Fatalf("unexpected topic %q", msg.topic)
		}
	}
}

func TestHandleDeviceCommandRejectsAndPublishesResult(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "thread-adapter-1", Version: "dev"})
	payload := []byte(`{"device_id":"thread/node-1","corr":"corr-1"}`)

	svc.handleDeviceCommand(nil, fakeMessage{topic: hdp.CommandPrefix + "thread/node-1", payload: payload})

	if len(client.published) != 1 {
		t.Fatalf("expected one command_result publish, got %d", len(client.published))
	}
	if client.published[0].topic != hdp.CommandResultPrefix+"thread/node-1" {
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

func TestDeviceIDHelpers(t *testing.T) {
	svc := New(newFakeClient(), Config{})
	if got := svc.hdpDeviceID("node-1"); got != "thread/node-1" {
		t.Fatalf("unexpected hdpDeviceID: %q", got)
	}
	proto, external := svc.externalFromHDP("thread/floor1/node-1")
	if proto != "thread" || external != "node-1" {
		t.Fatalf("unexpected externalFromHDP result: %q %q", proto, external)
	}
}
