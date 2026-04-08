package hdp

import (
	"errors"
	"testing"
)

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
