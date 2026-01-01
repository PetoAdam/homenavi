package zigbee

import (
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

func TestCorrelationSetAndConsume(t *testing.T) {
	z := &ZigbeeAdapter{correlationMap: map[string]string{}}
	z.setCorrelation("dev1", "cid1")
	if got := z.consumeCorrelation("dev1"); got != "cid1" {
		t.Fatalf("expected cid1, got %q", got)
	}
	if got := z.consumeCorrelation("dev1"); got != "" {
		t.Fatalf("expected cleared mapping, got %q", got)
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
