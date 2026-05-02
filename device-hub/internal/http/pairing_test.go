package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mqttinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/mqtt"
)

type pairingTestMQTTClient struct {
	published []pairingPublishedMessage
}

type pairingPublishedMessage struct {
	topic   string
	payload []byte
	retain  bool
}

func (f *pairingTestMQTTClient) Subscribe(_ string, _ mqttinfra.Handler) error {
	return nil
}

func (f *pairingTestMQTTClient) Publish(topic string, payload []byte) error {
	f.published = append(f.published, pairingPublishedMessage{topic: topic, payload: payload})
	return nil
}

func (f *pairingTestMQTTClient) PublishWith(topic string, payload []byte, retain bool) error {
	f.published = append(f.published, pairingPublishedMessage{topic: topic, payload: payload, retain: retain})
	return nil
}

func TestHandlePairingsStartPublishesSchemaInputs(t *testing.T) {
	mqtt := &pairingTestMQTTClient{}
	srv := NewServer(nil, mqtt)
	srv.adapters.upsertFromHello([]byte(`{
		"schema":"hdp.v1",
		"type":"hello",
		"adapter_id":"mock-adapter-1",
		"protocol":"mock",
		"version":"test",
		"pairing":{
			"schema_version":"1.0",
			"label":"Mock Adapter",
			"supported":true,
			"supports_interview":false,
			"default_timeout_sec":60
		},
		"ts":1
	}`))

	body := `{
		"protocol": "mock",
		"timeout": 90,
		"mode": "qr_code",
		"flow_id": "flow-123",
		"inputs": {
			"onboarding_payload": "MT:ABC",
			"discriminator": 3840,
			"": "ignored"
		},
		"metadata": {
			"type": "light"
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/pairings", strings.NewReader(body))
	rr := httptest.NewRecorder()

	srv.handlePairings(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusAccepted)
	}

	if len(mqtt.published) == 0 {
		t.Fatal("expected at least one publish call")
	}

	var accepted pairingSession
	if err := json.Unmarshal(rr.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted session: %v", err)
	}
	if accepted.Protocol != "mock" {
		t.Fatalf("expected session protocol mock, got %q", accepted.Protocol)
	}
	if accepted.Mode != "qr_code" {
		t.Fatalf("expected session mode qr_code, got %q", accepted.Mode)
	}
	if accepted.FlowID != "flow-123" {
		t.Fatalf("expected session flow id flow-123, got %q", accepted.FlowID)
	}
	if accepted.Inputs["onboarding_payload"] != "MT:ABC" {
		t.Fatalf("expected session input onboarding payload, got %v", accepted.Inputs["onboarding_payload"])
	}

	var command map[string]any
	if err := json.Unmarshal(mqtt.published[0].payload, &command); err != nil {
		t.Fatalf("decode command payload: %v", err)
	}
	if command["type"] != "pairing_command" {
		t.Fatalf("expected pairing_command type, got %v", command["type"])
	}
	if command["protocol"] != "mock" {
		t.Fatalf("expected mock protocol, got %v", command["protocol"])
	}
	if command["mode"] != "qr_code" {
		t.Fatalf("expected mode qr_code, got %v", command["mode"])
	}
	if command["flow_id"] != "flow-123" {
		t.Fatalf("expected flow_id flow-123, got %v", command["flow_id"])
	}
	inputs, ok := command["inputs"].(map[string]any)
	if !ok {
		t.Fatalf("expected inputs map, got %T", command["inputs"])
	}
	if inputs["onboarding_payload"] != "MT:ABC" {
		t.Fatalf("expected onboarding payload preserved, got %v", inputs["onboarding_payload"])
	}
	if _, exists := inputs[""]; exists {
		t.Fatal("expected empty input key to be removed")
	}
}

func TestProcessPairingProgressCompletesMockSession(t *testing.T) {
	mqtt := &pairingTestMQTTClient{}
	srv := NewServer(nil, mqtt)
	srv.adapters.upsertFromHello([]byte(`{
		"schema":"hdp.v1",
		"type":"hello",
		"adapter_id":"mock-adapter-1",
		"protocol":"mock",
		"version":"test",
		"pairing":{
			"schema_version":"1.0",
			"label":"Mock Adapter",
			"supported":true,
			"supports_interview":false,
			"default_timeout_sec":60
		},
		"ts":1
	}`))

	if _, err := srv.startPairing("mock", 90, "manual_code", "flow-lifecycle", map[string]any{"manual_code": "12345678"}, pairingMetadata{Type: "light"}); err != nil {
		t.Fatalf("start pairing: %v", err)
	}

	srv.processPairingProgress("mock", "completed", "completed", "mock-device-001", pairingProgressUpdate{})

	sessions := srv.snapshotPairings()
	if len(sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sessions))
	}
	if sessions[0].Active {
		t.Fatal("expected session to be inactive after completion")
	}
	if sessions[0].Status != "completed" {
		t.Fatalf("expected completed status, got %q", sessions[0].Status)
	}
	if sessions[0].Mode != "manual_code" {
		t.Fatalf("expected manual_code mode preserved, got %q", sessions[0].Mode)
	}
	if sessions[0].FlowID != "flow-lifecycle" {
		t.Fatalf("expected flow-lifecycle flow id preserved, got %q", sessions[0].FlowID)
	}
	if sessions[0].Inputs["manual_code"] != "12345678" {
		t.Fatalf("expected manual_code input preserved, got %v", sessions[0].Inputs["manual_code"])
	}
}

func TestProcessPairingProgressTracksNeedsInputForMatter(t *testing.T) {
	mqtt := &pairingTestMQTTClient{}
	srv := NewServer(nil, mqtt)
	srv.adapters.upsertFromHello([]byte(`{
		"schema":"hdp.v1",
		"type":"hello",
		"adapter_id":"matter-adapter-1",
		"protocol":"matter",
		"version":"test",
		"pairing":{
			"schema_version":"1.0",
			"label":"Matter Adapter",
			"supported":true,
			"supports_interview":true,
			"default_timeout_sec":300
		},
		"ts":1
	}`))

	if _, err := srv.startPairing("matter", 120, "manual_code", "matter-flow-1", map[string]any{"manual_code": "12345678"}, pairingMetadata{Type: "light"}); err != nil {
		t.Fatalf("start pairing: %v", err)
	}

	srv.processPairingProgress("matter", "network_provisioning", "needs_input", "matter-device-001", pairingProgressUpdate{
		Message:        "Thread dataset is required",
		ErrorCode:      "THREAD_DATASET_MISSING",
		RequiredInputs: []string{"thread_operational_dataset"},
		Inputs: map[string]any{
			"manual_code": "12345678",
		},
	})

	sessions := srv.snapshotPairings()
	if len(sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sessions))
	}
	session := sessions[0]
	if !session.Active {
		t.Fatal("expected needs_input session to remain active")
	}
	if session.Stage != "network_provisioning" {
		t.Fatalf("expected stage network_provisioning, got %q", session.Stage)
	}
	if session.Status != "needs_input" {
		t.Fatalf("expected status needs_input, got %q", session.Status)
	}
	if session.Message != "Thread dataset is required" {
		t.Fatalf("expected message to be captured, got %q", session.Message)
	}
	if session.ErrorCode != "THREAD_DATASET_MISSING" {
		t.Fatalf("expected error code THREAD_DATASET_MISSING, got %q", session.ErrorCode)
	}
	if len(session.RequiredInputs) != 1 || session.RequiredInputs[0] != "thread_operational_dataset" {
		t.Fatalf("expected required input thread_operational_dataset, got %#v", session.RequiredInputs)
	}
	if session.Inputs["manual_code"] != "12345678" {
		t.Fatalf("expected inputs to preserve manual_code, got %v", session.Inputs["manual_code"])
	}
}
