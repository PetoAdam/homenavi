package zigbee

import (
	"encoding/json"
	"os"
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
