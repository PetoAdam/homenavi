package engine

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDefinitionNormalizeAndValidate_AllowsFractionalSleep(t *testing.T) {
	d := Definition{
		Version: "automation",
		Nodes: []NodeDef{
			{ID: "trigger-1", Kind: "trigger.manual", Data: json.RawMessage(`{}`)},
			{ID: "sleep-1", Kind: "logic.sleep", Data: json.RawMessage(`{"duration_sec":0.2}`)},
		},
		Edges: []EdgeDef{{From: "trigger-1", To: "sleep-1"}},
	}

	if err := d.NormalizeAndValidate(); err != nil {
		t.Fatalf("expected fractional sleep duration to be valid, got %v", err)
	}
}

func TestDefinitionNormalizeAndValidate_RejectsNegativeSleep(t *testing.T) {
	d := Definition{
		Version: "automation",
		Nodes: []NodeDef{
			{ID: "trigger-1", Kind: "trigger.manual", Data: json.RawMessage(`{}`)},
			{ID: "sleep-1", Kind: "logic.sleep", Data: json.RawMessage(`{"duration_sec":-0.2}`)},
		},
		Edges: []EdgeDef{{From: "trigger-1", To: "sleep-1"}},
	}

	err := d.NormalizeAndValidate()
	if err == nil || err.Error() != "logic.sleep.duration_sec must be >= 0" {
		t.Fatalf("expected negative sleep duration validation error, got %v", err)
	}
}

func TestSleepDuration_ConvertsFractionalSeconds(t *testing.T) {
	if got := sleepDuration(0.2); got != 200*time.Millisecond {
		t.Fatalf("expected 200ms, got %s", got)
	}

	if got := sleepDuration(-1); got != 0 {
		t.Fatalf("expected negative durations to clamp to zero, got %s", got)
	}
}
