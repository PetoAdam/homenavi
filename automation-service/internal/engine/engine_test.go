package engine

import (
	"encoding/json"
	"testing"
)

func TestMatchStateTrigger_EmptyKeyMatchesAny(t *testing.T) {
	tr := TriggerDeviceState{Key: "", Op: "exists"}
	state := map[string]any{"motion": true}
	if !matchStateTrigger(tr, state) {
		t.Fatalf("expected empty key to match any state")
	}
}

func TestMatchStateTrigger_Exists(t *testing.T) {
	tr := TriggerDeviceState{Key: "motion", Op: "exists"}
	if !matchStateTrigger(tr, map[string]any{"motion": true}) {
		t.Fatalf("expected exists to match when key present")
	}
	if matchStateTrigger(tr, map[string]any{"temperature": 21}) {
		t.Fatalf("expected exists to not match when key missing")
	}
}

func TestMatchStateTrigger_Eq_NumberLoose(t *testing.T) {
	want, _ := json.Marshal(42)
	tr := TriggerDeviceState{Key: "x", Op: "eq", Value: want}
	if !matchStateTrigger(tr, map[string]any{"x": float64(42)}) {
		t.Fatalf("expected eq to match numeric equality")
	}
}

func TestMatchStateTrigger_Comparators(t *testing.T) {
	want, _ := json.Marshal(10)
	state := map[string]any{"temp": 12}

	cases := []struct {
		op   string
		want bool
	}{
		{"gt", true},
		{"gte", true},
		{"lt", false},
		{"lte", false},
	}
	for _, c := range cases {
		tr := TriggerDeviceState{Key: "temp", Op: c.op, Value: want}
		if got := matchStateTrigger(tr, state); got != c.want {
			t.Fatalf("op=%s: expected %v, got %v", c.op, c.want, got)
		}
	}
}

func TestMatchStateTrigger_Neq(t *testing.T) {
	want, _ := json.Marshal("ON")
	tr := TriggerDeviceState{Key: "state", Op: "neq", Value: want}
	if !matchStateTrigger(tr, map[string]any{"state": "OFF"}) {
		t.Fatalf("expected neq to match when values differ")
	}
	if matchStateTrigger(tr, map[string]any{"state": "ON"}) {
		t.Fatalf("expected neq to not match when values equal")
	}
}
