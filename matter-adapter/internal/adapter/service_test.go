package adapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestStartPublishesHelloAndStatusAndSubscribes(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer svc.Stop()

	if len(client.published) < 2 {
		t.Fatalf("expected hello and status messages, got %d", len(client.published))
	}
	if _, ok := client.subscribed[hdp.PairingCommandPrefix+"matter"]; !ok {
		t.Fatal("expected pairing subscription")
	}
	if _, ok := client.subscribed[hdp.CommandPrefix+"matter/#"]; !ok {
		t.Fatal("expected command subscription")
	}
	if client.published[1].topic != hdp.AdapterStatusPrefix+"matter-adapter-1" || !client.published[1].retain {
		t.Fatal("expected retained adapter status publish")
	}
}

func TestHandlePairingStartPublishesProgress(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","flow_id":"flow-a","inputs":{"manual_code":"12345678","network_path":"on_network"}}`)})

	// 7 pairing progress + 1 device metadata + 1 device state = 9 total
	if len(client.published) != 9 {
		t.Fatalf("expected nine total publishes (7 progress + metadata + state), got %d", len(client.published))
	}
	progressMsgs := make([]publishedMessage, 0, 7)
	for _, msg := range client.published {
		if msg.topic == hdp.PairingProgressPrefix+"matter" {
			progressMsgs = append(progressMsgs, msg)
		}
	}
	if len(progressMsgs) != 7 {
		t.Fatalf("expected 7 pairing progress messages, got %d", len(progressMsgs))
	}
	var first map[string]any
	if err := json.Unmarshal(progressMsgs[0].payload, &first); err != nil {
		t.Fatalf("unmarshal first stage: %v", err)
	}
	if first["stage"] != "discovery" {
		t.Fatalf("expected discovery stage first, got %v", first["stage"])
	}
	var fifth map[string]any
	if err := json.Unmarshal(progressMsgs[4].payload, &fifth); err != nil {
		t.Fatalf("unmarshal fifth stage: %v", err)
	}
	if fifth["stage"] != "operational_discovery" {
		t.Fatalf("expected operational_discovery stage, got %v", fifth["stage"])
	}
	var completed map[string]any
	if err := json.Unmarshal(progressMsgs[len(progressMsgs)-1].payload, &completed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if completed["stage"] != "commissioning_complete" {
		t.Fatalf("expected commissioning_complete stage, got %v", completed["stage"])
	}
	if completed["status"] != "completed" {
		t.Fatalf("expected completed status, got %v", completed["status"])
	}
	if completed["mode"] != "manual_code" {
		t.Fatalf("expected mode to be echoed, got %v", completed["mode"])
	}
	if completed["flow_id"] != "flow-a" {
		t.Fatalf("expected flow_id to be echoed, got %v", completed["flow_id"])
	}
}

func TestHandlePairingStartNeedsInputForQRCodeMode(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"qr_code","inputs":{}}`)})

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

func TestHandlePairingStartWithoutInputsDoesNotPanic(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"qr_code"}`)})

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
}

func TestHandlePairingStartNeedsInputForThreadDataset(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"thread"}}`)})

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
	if !ok || len(required) != 1 || required[0] != "thread_operational_dataset" {
		t.Fatalf("expected required thread dataset input, got %#v", payload["required_inputs"])
	}
}

func TestHandlePairingStartUsesConfiguredThreadDataset(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{
		Enabled:                  true,
		AdapterID:                "matter-adapter-1",
		Version:                  "dev",
		DefaultNetworkPath:       "thread",
		ThreadDatasetSource:      "env",
		ThreadOperationalDataset: "hex:deadbeef",
	})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"thread"}}`)})

	if len(client.published) <= 1 {
		t.Fatalf("expected progress publishes using configured dataset, got %d", len(client.published))
	}
	var first map[string]any
	if err := json.Unmarshal(client.published[0].payload, &first); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if first["status"] == "needs_input" {
		t.Fatalf("did not expect needs_input when configured dataset is present: %#v", first)
	}
	inputs, ok := first["inputs"].(map[string]any)
	if !ok {
		t.Fatalf("expected inputs map on first progress event, got %T", first["inputs"])
	}
	if inputs["thread_operational_dataset"] != "hex:deadbeef" {
		t.Fatalf("expected configured thread dataset to be injected, got %#v", inputs["thread_operational_dataset"])
	}
}

func TestParseOTBRDiagnostics(t *testing.T) {
	body := `
		<html><body>
		OTBR state: <strong>leader</strong>
		HEX-encoded TLVs 0e080000000000010000000300000f4a
		</body></html>`
	parsed := parseOTBRDiagnostics(body)
	if parsed.State != "leader" {
		t.Fatalf("expected leader state, got %q", parsed.State)
	}
	if parsed.Dataset != "hex:0e080000000000010000000300000f4a" {
		t.Fatalf("unexpected dataset %q", parsed.Dataset)
	}
}

func TestHandlePairingStartUsesOTBRDatasetWhenConfigured(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{
		Enabled:             true,
		AdapterID:           "matter-adapter-1",
		Version:             "dev",
		DefaultNetworkPath:  "thread",
		ThreadDatasetSource: "otbr",
		OTBRBaseURL:         "http://otbr.local",
		OTBRExpectedState:   "leader",
		ThreadBorderRouterPort: 8080,
	})
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "http://otbr.local:8080/node/state":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`"leader"`)),
				Header:     make(http.Header),
			}, nil
		case "http://otbr.local:8080/node/dataset/active":
			if got := req.Header.Get("Accept"); got != "text/plain" {
				t.Fatalf("expected text/plain accept header, got %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`deadbeef`)),
				Header:     make(http.Header),
			}, nil
		default:
			t.Fatalf("unexpected url %q", req.URL.String())
		}
		return nil, nil
	})}

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"thread"}}`)})

	if len(client.published) <= 1 {
		t.Fatalf("expected progress publishes using OTBR dataset, got %d", len(client.published))
	}
	var first map[string]any
	if err := json.Unmarshal(client.published[0].payload, &first); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if first["status"] == "needs_input" {
		t.Fatalf("did not expect needs_input when OTBR dataset is available: %#v", first)
	}
	inputs, ok := first["inputs"].(map[string]any)
	if !ok {
		t.Fatalf("expected inputs map on first progress event, got %T", first["inputs"])
	}
	if inputs["thread_operational_dataset"] != "hex:deadbeef" {
		t.Fatalf("expected OTBR thread dataset to be injected, got %#v", inputs["thread_operational_dataset"])
	}
}

func TestHandlePairingStartFailsWhenOTBRUnavailable(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{
		Enabled:             true,
		AdapterID:           "matter-adapter-1",
		Version:             "dev",
		ThreadDatasetSource: "otbr",
		OTBRBaseURL:         "http://otbr.local",
		OTBRExpectedState:   "leader",
	})
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})}

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"thread"}}`)})

	if len(client.published) != 1 {
		t.Fatalf("expected one failure progress message, got %d", len(client.published))
	}
	var payload map[string]any
	if err := json.Unmarshal(client.published[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["status"] != "failed" {
		t.Fatalf("expected failed status, got %v", payload["status"])
	}
	if payload["error_code"] != "BORDER_ROUTER_UNAVAILABLE" {
		t.Fatalf("expected BORDER_ROUTER_UNAVAILABLE, got %v", payload["error_code"])
	}
}

func TestHandlePairingStartUsesExternalCommissioner(t *testing.T) {
	client := newFakeClient()
	scriptPath := filepath.Join(t.TempDir(), "commissioner.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\ncat >/dev/null\nprintf '{\"external_id\":\"matter-live-001\",\"message\":\"Commissioned via external commissioner\",\"metadata\":{\"type\":\"light\",\"manufacturer\":\"TestVendor\",\"model\":\"T1\",\"icon\":\"lightbulb\"},\"state\":{\"on\":false,\"level\":100}}'\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	svc := New(client, Config{
		Enabled:             true,
		AdapterID:           "matter-adapter-1",
		Version:             "dev",
		CommissionerEnabled: true,
		CommissionerCommand: scriptPath,
	})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"on_network"}}`)})

	var foundCompleted bool
	var foundMeta bool
	for _, msg := range client.published {
		if msg.topic == hdp.MetadataPrefix+"matter/matter-live-001" {
			foundMeta = true
		}
		if msg.topic == hdp.PairingProgressPrefix+"matter" {
			var payload map[string]any
			if err := json.Unmarshal(msg.payload, &payload); err == nil && payload["status"] == "completed" {
				foundCompleted = true
			}
		}
	}
	if !foundMeta {
		t.Fatal("expected metadata publish for external commissioner result")
	}
	if !foundCompleted {
		t.Fatal("expected completed pairing progress from external commissioner")
	}
}

func TestHandlePairingStartFailsWhenCommissionerMissing(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{
		Enabled:             true,
		AdapterID:           "matter-adapter-1",
		Version:             "dev",
		CommissionerEnabled: true,
		CommissionerCommand: "/definitely/missing/commissioner",
	})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"on_network"}}`)})

	last := client.published[len(client.published)-1]
	var payload map[string]any
	if err := json.Unmarshal(last.payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["status"] != "failed" {
		t.Fatalf("expected failed status, got %v", payload["status"])
	}
	if payload["error_code"] != "COMMISSIONER_EXECUTION_FAILED" {
		t.Fatalf("expected COMMISSIONER_EXECUTION_FAILED, got %v", payload["error_code"])
	}
}

func TestHandlePairingStartFailsForInvalidManualCode(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"123","network_path":"on_network"}}`)})

	if len(client.published) != 1 {
		t.Fatalf("expected one progress message, got %d", len(client.published))
	}
	var payload map[string]any
	if err := json.Unmarshal(client.published[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["status"] != "failed" {
		t.Fatalf("expected failed status, got %v", payload["status"])
	}
	if payload["error_code"] != "MANUAL_CODE_INVALID" {
		t.Fatalf("expected MANUAL_CODE_INVALID, got %v", payload["error_code"])
	}
}

func TestHandlePairingStartAcceptsFormattedManualCode(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"3520-203-3941","network_path":"on_network"}}`)})

	if len(client.published) == 0 {
		t.Fatal("expected pairing progress publishes")
	}
	var first map[string]any
	if err := json.Unmarshal(client.published[0].payload, &first); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if first["status"] == "failed" {
		t.Fatalf("expected formatted manual code to be accepted, got %#v", first)
	}
	inputs, ok := first["inputs"].(map[string]any)
	if !ok {
		t.Fatalf("expected normalized inputs map, got %T", first["inputs"])
	}
	if inputs["manual_code"] != "35202033941" {
		t.Fatalf("expected normalized manual code, got %#v", inputs["manual_code"])
	}
}

func TestHandlePairingStartFailsForInvalidQRCodePayload(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"qr_code","inputs":{"onboarding_payload":"INVALID"}}`)})

	if len(client.published) != 1 {
		t.Fatalf("expected one progress message, got %d", len(client.published))
	}
	var payload map[string]any
	if err := json.Unmarshal(client.published[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["status"] != "failed" {
		t.Fatalf("expected failed status, got %v", payload["status"])
	}
	if payload["error_code"] != "ONBOARDING_PAYLOAD_INVALID" {
		t.Fatalf("expected ONBOARDING_PAYLOAD_INVALID, got %v", payload["error_code"])
	}
}

func TestHandlePairingStartSupportsFailureSimulation(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"on_network","simulate_failure_stage":"attestation"}}`)})

	if len(client.published) != 1 {
		t.Fatalf("expected one progress message, got %d", len(client.published))
	}
	var payload map[string]any
	if err := json.Unmarshal(client.published[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["status"] != "failed" {
		t.Fatalf("expected failed status, got %v", payload["status"])
	}
	if payload["stage"] != "attestation" {
		t.Fatalf("expected attestation stage, got %v", payload["stage"])
	}
}

func TestHandlePairingStartSupportsTimeoutSimulation(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"on_network","simulate_timeout_stage":"pase"}}`)})

	if len(client.published) != 1 {
		t.Fatalf("expected one progress message, got %d", len(client.published))
	}
	var payload map[string]any
	if err := json.Unmarshal(client.published[0].payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["status"] != "timeout" {
		t.Fatalf("expected timeout status, got %v", payload["status"])
	}
	if payload["stage"] != "pase" {
		t.Fatalf("expected pase stage, got %v", payload["stage"])
	}
}

func TestHandlePairingStartPublishesMockDeviceMetadataOnCompletion(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{topic: hdp.PairingCommandPrefix + "matter", payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"on_network"}}`)})

	if len(client.published) == 0 {
		t.Fatal("expected pairing progress publishes")
	}
	var completed map[string]any
	if err := json.Unmarshal(client.published[len(client.published)-1].payload, &completed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	metadata, ok := completed["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %T", completed["metadata"])
	}
	if metadata["manufacturer"] != "Matter" {
		t.Fatalf("expected Matter manufacturer, got %v", metadata["manufacturer"])
	}
	if metadata["model"] != "MVP Device" {
		t.Fatalf("expected MVP Device model, got %v", metadata["model"])
	}
	if metadata["type"] != "light" {
		t.Fatalf("expected light type, got %v", metadata["type"])
	}
}

func TestHandleDeviceCommandRejectsAndPublishesResult(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})
	payload := []byte(`{"device_id":"matter/node-1","corr":"corr-1"}`)

	svc.handleDeviceCommand(nil, fakeMessage{topic: hdp.CommandPrefix + "matter/node-1", payload: payload})

	if len(client.published) != 1 {
		t.Fatalf("expected one command_result publish, got %d", len(client.published))
	}
	if client.published[0].topic != hdp.CommandResultPrefix+"matter/node-1" {
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
	if got := svc.hdpDeviceID("node-1"); got != "matter/node-1" {
		t.Fatalf("unexpected hdpDeviceID: %q", got)
	}
	proto, external := svc.externalFromHDP("matter/floor1/node-1")
	if proto != "matter" || external != "node-1" {
		t.Fatalf("unexpected externalFromHDP result: %q %q", proto, external)
	}
}
// --- M2: device registration tests ---

func TestPairingCompletionPublishesDeviceMetadataAndState(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{
		topic:   hdp.PairingCommandPrefix + "matter",
		payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"on_network"}}`),
	})

	var metaMsg, stateMsg *publishedMessage
	for i := range client.published {
		msg := &client.published[i]
		if msg.topic == hdp.MetadataPrefix+"matter/matter-device-001" {
			metaMsg = msg
		}
		if msg.topic == hdp.StatePrefix+"matter/matter-device-001" {
			stateMsg = msg
		}
	}
	if metaMsg == nil {
		t.Fatal("expected device metadata publish on MetadataPrefix topic")
	}
	if !metaMsg.retain {
		t.Fatal("device metadata must be published retained")
	}
	if stateMsg == nil {
		t.Fatal("expected device state publish on StatePrefix topic")
	}
	if !stateMsg.retain {
		t.Fatal("device state must be published retained")
	}

	var meta map[string]any
	if err := json.Unmarshal(metaMsg.payload, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["protocol"] != "matter" {
		t.Fatalf("expected matter protocol in metadata, got %v", meta["protocol"])
	}
	if meta["manufacturer"] != "Matter" {
		t.Fatalf("expected Matter manufacturer, got %v", meta["manufacturer"])
	}

	var state map[string]any
	if err := json.Unmarshal(stateMsg.payload, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	inner, ok := state["state"].(map[string]any)
	if !ok {
		t.Fatalf("expected state.state map, got %T", state["state"])
	}
	if inner["on"] != false {
		t.Fatalf("expected initial on=false, got %v", inner["on"])
	}
}

func TestDeviceIsRegisteredAfterCommissioning(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})

	svc.handlePairingCommand(nil, fakeMessage{
		topic:   hdp.PairingCommandPrefix + "matter",
		payload: []byte(`{"action":"start","mode":"on_network","inputs":{"manual_code":"12345678","network_path":"on_network"}}`),
	})

	raw, ok := svc.devices.Load("matter-device-001")
	if !ok {
		t.Fatal("expected matter-device-001 in device registry after commissioning")
	}
	dev := raw.(matterDevice)
	if dev.Type != "light" {
		t.Fatalf("expected light type, got %v", dev.Type)
	}
}

// --- M3: command/state sync tests ---

func commissionDevice(t *testing.T, svc *Service) {
	t.Helper()
	client := svc.client.(*fakeClient)
	svc.handlePairingCommand(nil, fakeMessage{
		topic:   hdp.PairingCommandPrefix + "matter",
		payload: []byte(`{"action":"start","mode":"manual_code","inputs":{"manual_code":"12345678","network_path":"on_network"}}`),
	})
	client.published = nil // reset after commissioning
}

func TestHandleDeviceCommandTurnOn(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})
	commissionDevice(t, svc)

	svc.handleDeviceCommand(nil, fakeMessage{
		topic:   hdp.CommandPrefix + "matter/matter-device-001",
		payload: []byte(`{"device_id":"matter/matter-device-001","corr":"c1","command":"turn_on"}`),
	})

	if len(client.published) != 2 {
		t.Fatalf("expected 2 publishes (state + result), got %d", len(client.published))
	}
	// state published retained
	if client.published[0].topic != hdp.StatePrefix+"matter/matter-device-001" {
		t.Fatalf("unexpected first topic %q", client.published[0].topic)
	}
	if !client.published[0].retain {
		t.Fatal("device state must be retained")
	}
	// result published
	if client.published[1].topic != hdp.CommandResultPrefix+"matter/matter-device-001" {
		t.Fatalf("unexpected second topic %q", client.published[1].topic)
	}
	var result map[string]any
	if err := json.Unmarshal(client.published[1].payload, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
	if result["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", result["status"])
	}
	// verify state persisted
	raw, _ := svc.devices.Load("matter-device-001")
	if raw.(matterDevice).State["on"] != true {
		t.Fatal("expected device on=true after turn_on")
	}
}

func TestHandleDeviceCommandSetLevel(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})
	commissionDevice(t, svc)

	svc.handleDeviceCommand(nil, fakeMessage{
		topic:   hdp.CommandPrefix + "matter/matter-device-001",
		payload: []byte(`{"device_id":"matter/matter-device-001","corr":"c2","command":"set_level","args":{"level":128}}`),
	})

	raw, _ := svc.devices.Load("matter-device-001")
	dev := raw.(matterDevice)
	if dev.State["level"] != float64(128) {
		t.Fatalf("expected level=128, got %v", dev.State["level"])
	}
	if dev.State["on"] != true {
		t.Fatal("expected on=true after set_level > 0")
	}
}

func TestHandleDeviceCommandSetColorTemp(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})
	commissionDevice(t, svc)

	svc.handleDeviceCommand(nil, fakeMessage{
		topic:   hdp.CommandPrefix + "matter/matter-device-001",
		payload: []byte(`{"device_id":"matter/matter-device-001","corr":"c3","command":"set_color_temp","args":{"color_temp":300}}`),
	})

	raw, _ := svc.devices.Load("matter-device-001")
	dev := raw.(matterDevice)
	if dev.State["color_temp"] != float64(300) {
		t.Fatalf("expected color_temp=300, got %v", dev.State["color_temp"])
	}
	if dev.State["on"] != true {
		t.Fatal("expected on=true after set_color_temp")
	}
}

func TestHandleDeviceCommandToggle(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})
	commissionDevice(t, svc)

	// initial state: on=false; first toggle → true
	svc.handleDeviceCommand(nil, fakeMessage{
		topic:   hdp.CommandPrefix + "matter/matter-device-001",
		payload: []byte(`{"device_id":"matter/matter-device-001","corr":"c4","command":"toggle"}`),
	})
	raw, _ := svc.devices.Load("matter-device-001")
	if raw.(matterDevice).State["on"] != true {
		t.Fatal("expected on=true after first toggle")
	}
	client.published = nil
	// second toggle → false
	svc.handleDeviceCommand(nil, fakeMessage{
		topic:   hdp.CommandPrefix + "matter/matter-device-001",
		payload: []byte(`{"device_id":"matter/matter-device-001","corr":"c5","command":"toggle"}`),
	})
	raw, _ = svc.devices.Load("matter-device-001")
	if raw.(matterDevice).State["on"] != false {
		t.Fatal("expected on=false after second toggle")
	}
}

func TestHandleDeviceCommandRejectsUnknownCommand(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})
	commissionDevice(t, svc)

	svc.handleDeviceCommand(nil, fakeMessage{
		topic:   hdp.CommandPrefix + "matter/matter-device-001",
		payload: []byte(`{"device_id":"matter/matter-device-001","corr":"c6","command":"explode"}`),
	})

	if len(client.published) != 1 {
		t.Fatalf("expected 1 publish (rejected result), got %d", len(client.published))
	}
	var result map[string]any
	_ = json.Unmarshal(client.published[0].payload, &result)
	if result["success"] == true {
		t.Fatal("expected success=false for unknown command")
	}
	if result["status"] != "rejected" {
		t.Fatalf("expected status=rejected, got %v", result["status"])
	}
}

func TestHandleDeviceCommandSetLevelOutOfRange(t *testing.T) {
	client := newFakeClient()
	svc := New(client, Config{Enabled: true, AdapterID: "matter-adapter-1", Version: "dev"})
	commissionDevice(t, svc)

	svc.handleDeviceCommand(nil, fakeMessage{
		topic:   hdp.CommandPrefix + "matter/matter-device-001",
		payload: []byte(`{"device_id":"matter/matter-device-001","corr":"c7","command":"set_level","args":{"level":999}}`),
	})

	var result map[string]any
	_ = json.Unmarshal(client.published[0].payload, &result)
	if result["status"] != "rejected" {
		t.Fatalf("expected rejected for out-of-range level, got %v", result["status"])
	}
}

func TestHandleDeviceCommandUsesExternalCommissioner(t *testing.T) {
	client := newFakeClient()
	scriptPath := filepath.Join(t.TempDir(), "commissioner.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nmode=\"${1:-}\"\ncat >/dev/null\nif [ \"$mode\" != \"command\" ]; then\n  echo 'unexpected mode' >&2\n  exit 2\nfi\nprintf '{\"device_id\":\"matter-device-001\",\"message\":\"command applied\",\"state\":{\"on\":true,\"level\":200}}'\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	svc := New(client, Config{
		Enabled:   true,
		AdapterID: "matter-adapter-1",
		Version:   "dev",
	})
	commissionDevice(t, svc)
	svc.commissionerEnabled = true
	svc.commissionerCommand = scriptPath

	svc.handleDeviceCommand(nil, fakeMessage{
		topic:   hdp.CommandPrefix + "matter/matter-device-001",
		payload: []byte(`{"device_id":"matter/matter-device-001","corr":"c-ext","command":"turn_on"}`),
	})

	raw, _ := svc.devices.Load("matter-device-001")
	dev := raw.(matterDevice)
	if dev.State["on"] != true {
		t.Fatal("expected on=true after commissioner-backed turn_on")
	}
	if dev.State["level"] != float64(200) {
		t.Fatalf("expected level from commissioner response, got %v", dev.State["level"])
	}
	if len(client.published) != 2 {
		t.Fatalf("expected state + result publishes, got %d", len(client.published))
	}
}