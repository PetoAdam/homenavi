package hdp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestStateIgnoresInvalidTraceMetadata(t *testing.T) {
	for _, metadata := range []string{`"invalid"`, `42`, `[]`, `{"traceparent":42}`, `{"traceparent":"` + strings.Repeat("a", 1024) + `"}`} {
		var state State
		payload := `{"schema":"hdp.v1","type":"state","device_id":"zigbee/test","state":{"on":true},"trace":` + metadata + `}`
		if err := json.Unmarshal([]byte(payload), &state); err != nil {
			t.Fatalf("optional trace %s rejected the state: %v", metadata, err)
		}
		if state.DeviceID != "zigbee/test" || state.State["on"] != true || state.Trace == nil || state.Trace.Traceparent != "" {
			t.Fatalf("invalid metadata changed the state: %#v", state)
		}
	}
}

func TestTopic(t *testing.T) {
	if got := Topic(StatePrefix, "zigbee/device-1"); got != "homenavi/hdp/device/state/zigbee/device-1" {
		t.Fatalf("unexpected topic %q", got)
	}
}

func TestDeviceIDFromTopic(t *testing.T) {
	got, err := DeviceIDFromTopic(StatePrefix, "homenavi/hdp/device/state/zigbee/device-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "zigbee/device-1" {
		t.Fatalf("unexpected id %q", got)
	}
	_, err = DeviceIDFromTopic(MetadataPrefix, "homenavi/hdp/device/state/zigbee/device-1")
	if !errors.Is(err, ErrTopicPrefixMismatch) {
		t.Fatalf("expected prefix mismatch, got %v", err)
	}
}
