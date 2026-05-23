package zigbee

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestCanonicalExternalID(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{"hex with prefix", map[string]any{"ieee_address": "0xabcdef"}, "0xabcdef"},
		{"hex without prefix", map[string]any{"ieee_address": "A1B2C3"}, "0xa1b2c3"},
		{"missing", map[string]any{"id": "x"}, ""},
	}
	for _, tc := range cases {
		got := canonicalExternalID(tc.raw)
		if got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestResolveExternalIDAcceptsCanonicalTopicID(t *testing.T) {
	z := &ZigbeeAdapter{friendlyIndex: map[string]string{}}
	got := z.resolveExternalID(" 0X0020A716FF01963C ")
	if got != "0x0020a716ff01963c" {
		t.Fatalf("expected canonical external id from topic, got %q", got)
	}
}

func TestBridgeLifecycleStage(t *testing.T) {
	if got := bridgeLifecycleStage("device_joined"); got != "device_joined" {
		t.Fatalf("unexpected stage %q", got)
	}
	if got := bridgeLifecycleStage("device_announce"); got != "device_detected" {
		t.Fatalf("unexpected stage %q", got)
	}
	if got := bridgeLifecycleStage("other"); got != "" {
		t.Fatalf("expected empty stage, got %q", got)
	}
}

func TestInterviewStageFromStatus(t *testing.T) {
	cases := map[string]string{
		"started":    "interviewing",
		"SUCCESS":    "interview_complete",
		"failed":     "failed",
		"unexpected": "interviewing",
	}
	for input, want := range cases {
		if got := interviewStageFromStatus(input); got != want {
			t.Fatalf("status %q: expected %q, got %q", input, want, got)
		}
	}
}

func TestMockBridgeResponseLifecycleStage(t *testing.T) {
	payload := []byte(`{"type":"device_joined","data":{"friendly_name":"ikea-bulb-1","ieee_address":"0x00124b0024abcd01"}}`)
	var evt struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(payload, &evt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := bridgeLifecycleStage(evt.Type); got != "device_joined" {
		t.Fatalf("expected device_joined stage, got %q", got)
	}
	if ext := canonicalExternalID(evt.Data); ext != "0x00124b0024abcd01" {
		t.Fatalf("expected canonical external id, got %q", ext)
	}
}

func TestMockBridgeResponseInterviewStages(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   string
	}{
		{name: "started", status: "started", want: "interviewing"},
		{name: "success", status: "successful", want: "interview_complete"},
		{name: "failed", status: "failed", want: "failed"},
	}

	for _, tc := range cases {
		payload := []byte(`{"type":"device_interview","data":{"friendly_name":"ikea-bulb-1","ieee_address":"0x00124b0024abcd01","status":"` + tc.status + `"}}`)
		var evt struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(payload, &evt); err != nil {
			t.Fatalf("%s: unmarshal: %v", tc.name, err)
		}
		got := interviewStageFromStatus(asString(evt.Data["status"]))
		if got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestCorrelationSetAndConsume(t *testing.T) {
	z := &ZigbeeAdapter{correlationMap: map[string]pendingCorrelation{}}
	z.setCorrelation("dev1", "cid1")
	if got := z.consumeCorrelation("dev1"); got != "cid1" {
		t.Fatalf("expected cid1, got %q", got)
	}
	if got := z.consumeCorrelation("dev1"); got != "cid1" {
		t.Fatalf("expected retained cid1 on second publish, got %q", got)
	}
	if got := z.consumeCorrelation("dev1"); got != "cid1" {
		t.Fatalf("expected retained cid1 on third publish, got %q", got)
	}
	if got := z.consumeCorrelation("dev1"); got != "" {
		t.Fatalf("expected cleared mapping after bounded publishes, got %q", got)
	}
	z.setCorrelation("", "cid2")
	if got := z.consumeCorrelation(""); got != "" {
		t.Fatalf("expected empty for missing device id, got %q", got)
	}
}

func TestAdapterVersion(t *testing.T) {
	orig := os.Getenv("ZIGBEE_ADAPTER_VERSION")
	defer os.Setenv("ZIGBEE_ADAPTER_VERSION", orig)
	os.Unsetenv("ZIGBEE_ADAPTER_VERSION")
	os.Unsetenv("DEVICE_HUB_ZIGBEE_VERSION")
	if got := adapterVersion(); got != "dev" {
		t.Fatalf("expected default dev, got %q", got)
	}
	os.Setenv("DEVICE_HUB_ZIGBEE_VERSION", "legacy")
	if got := adapterVersion(); got != "legacy" {
		t.Fatalf("expected legacy fallback, got %q", got)
	}
	os.Setenv("ZIGBEE_ADAPTER_VERSION", "custom")
	if got := adapterVersion(); got != "custom" {
		t.Fatalf("expected custom, got %q", got)
	}
}

func TestNormalizeZigbeeStateCommandPayload(t *testing.T) {
	payload := normalizeZigbeeStateCommandPayload(map[string]any{
		"state":         true,
		"brightness":    150,
		"transition_ms": 1200,
	})

	if got := payload["state"]; got != "ON" {
		t.Fatalf("expected state to normalize to ON, got %#v", got)
	}
	if got := payload["brightness"]; got != 150 {
		t.Fatalf("expected brightness to be preserved, got %#v", got)
	}
	if got := payload["transition"]; got != 1.2 {
		t.Fatalf("expected transition seconds to be 1.2, got %#v", got)
	}
	if _, exists := payload["transition_ms"]; exists {
		t.Fatalf("expected transition_ms to be omitted from final zigbee payload")
	}
}

func TestNormalizeZigbeeStateCommandPayload_PowerAndOnBool(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{name: "power false", input: map[string]any{"power": false}, want: "OFF"},
		{name: "on true", input: map[string]any{"on": true}, want: "ON"},
	}

	for _, tc := range cases {
		payload := normalizeZigbeeStateCommandPayload(tc.input)
		if got := payload["state"]; got != tc.want {
			t.Fatalf("%s: expected %q, got %#v", tc.name, tc.want, got)
		}
	}
}

func TestZigbeeReconfigureRequestInterview(t *testing.T) {
	topic, payload, err := zigbeeReconfigureRequest("interview", "living-room-bulb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topic != "zigbee2mqtt/bridge/request/device/interview" {
		t.Fatalf("unexpected topic %q", topic)
	}
	var body map[string]string
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if body["id"] != "living-room-bulb" {
		t.Fatalf("expected target id to be preserved, got %#v", body)
	}
}

func TestZigbeeReconfigureRequestRejectsUnsupportedMode(t *testing.T) {
	if _, _, err := zigbeeReconfigureRequest("factory_reset", "living-room-bulb"); err == nil {
		t.Fatal("expected unsupported mode error")
	}
}

func TestPendingReconfigureLifecycle(t *testing.T) {
	z := &ZigbeeAdapter{reinterviews: map[string]pendingReconfigure{}}
	z.rememberPendingReconfigure("0x00124b0024abcd01", "interview", "corr-1")
	entry, ok := z.pendingReconfigureForExternal("0x00124b0024abcd01", "interview")
	if !ok {
		t.Fatal("expected pending reconfigure entry")
	}
	if entry.corr != "corr-1" {
		t.Fatalf("expected corr-1, got %q", entry.corr)
	}
	z.clearPendingReconfigure("0x00124b0024abcd01")
	if _, ok := z.pendingReconfigureForExternal("0x00124b0024abcd01", "interview"); ok {
		t.Fatal("expected entry to be cleared")
	}
}

func TestPairingProgressFromBridgeEvent_RealisticZigbeePairingFlow(t *testing.T) {
	cases := []struct {
		name       string
		payload    string
		wantStage  string
		wantStatus string
		wantExt    string
		wantOK     bool
	}{
		{
			name:       "join event",
			payload:    `{"type":"device_joined","data":{"friendly_name":"living-room-bulb","ieee_address":"0x00124b0024abcd01","manufacturer":"IKEA","model":"TRADFRI bulb E27"}}`,
			wantStage:  "device_joined",
			wantStatus: "",
			wantExt:    "0x00124b0024abcd01",
			wantOK:     true,
		},
		{
			name:       "announce event",
			payload:    `{"type":"device_announce","data":{"friendly_name":"living-room-bulb","ieee_address":"0x00124b0024abcd01"}}`,
			wantStage:  "device_detected",
			wantStatus: "",
			wantExt:    "0x00124b0024abcd01",
			wantOK:     true,
		},
		{
			name:       "interview started",
			payload:    `{"type":"device_interview","data":{"friendly_name":"living-room-bulb","ieee_address":"0x00124b0024abcd01","status":"started"}}`,
			wantStage:  "interviewing",
			wantStatus: "started",
			wantExt:    "0x00124b0024abcd01",
			wantOK:     true,
		},
		{
			name:       "interview successful",
			payload:    `{"type":"device_interview","data":{"friendly_name":"living-room-bulb","ieee_address":"0x00124b0024abcd01","status":"successful"}}`,
			wantStage:  "interview_complete",
			wantStatus: "successful",
			wantExt:    "0x00124b0024abcd01",
			wantOK:     true,
		},
		{
			name:       "unsupported event",
			payload:    `{"type":"device_leave","data":{"friendly_name":"living-room-bulb","ieee_address":"0x00124b0024abcd01"}}`,
			wantStage:  "",
			wantStatus: "",
			wantExt:    "0x00124b0024abcd01",
			wantOK:     false,
		},
	}

	for _, tc := range cases {
		var evt struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal([]byte(tc.payload), &evt); err != nil {
			t.Fatalf("%s: unmarshal: %v", tc.name, err)
		}
		stage, status, ext, ok := pairingProgressFromBridgeEvent(evt.Type, evt.Data)
		if ok != tc.wantOK || stage != tc.wantStage || status != tc.wantStatus || ext != tc.wantExt {
			t.Fatalf("%s: got ok=%v stage=%q status=%q ext=%q", tc.name, ok, stage, status, ext)
		}
	}
}

func TestPairingProgressEnvelope_RealisticFlowMessages(t *testing.T) {
	sequence := []struct {
		name       string
		stage      string
		status     string
		external   string
		deviceID   string
		wantFields map[string]any
	}{
		{
			name:     "join envelope",
			stage:    "device_joined",
			status:   "",
			external: "0x00124b0024abcd01",
			deviceID: "zigbee/0x00124b0024abcd01",
			wantFields: map[string]any{
				"protocol":    "zigbee",
				"stage":       "device_joined",
				"status":      "",
				"external_id": "0x00124b0024abcd01",
				"device_id":   "zigbee/0x00124b0024abcd01",
			},
		},
		{
			name:     "interview started envelope",
			stage:    "interviewing",
			status:   "started",
			external: "0x00124b0024abcd01",
			deviceID: "zigbee/0x00124b0024abcd01",
			wantFields: map[string]any{
				"protocol":    "zigbee",
				"stage":       "interviewing",
				"status":      "started",
				"external_id": "0x00124b0024abcd01",
				"device_id":   "zigbee/0x00124b0024abcd01",
			},
		},
		{
			name:     "final completed envelope",
			stage:    "completed",
			status:   "successful",
			external: "0x00124b0024abcd01",
			deviceID: "zigbee/0x00124b0024abcd01",
			wantFields: map[string]any{
				"protocol":    "zigbee",
				"stage":       "completed",
				"status":      "successful",
				"external_id": "0x00124b0024abcd01",
				"device_id":   "zigbee/0x00124b0024abcd01",
			},
		},
	}

	for index, tc := range sequence {
		envelope := pairingProgressEnvelope(tc.stage, tc.status, tc.external, tc.deviceID, int64(1000+index))
		if envelope == nil {
			t.Fatalf("%s: expected envelope", tc.name)
		}
		for key, want := range tc.wantFields {
			if got := envelope[key]; !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: field %s expected %#v, got %#v", tc.name, key, want, got)
			}
		}
	}
}
