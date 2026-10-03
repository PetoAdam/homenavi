package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestMatchStateTrigger_ChangedRequiresPriorDifferentValue(t *testing.T) {
	trigger := TriggerDeviceState{Key: "state", Op: "changed"}
	if matchStateTriggerWithPrevious(trigger, map[string]any{"state": "ON"}, nil) {
		t.Fatal("first observation must not count as changed")
	}
	if matchStateTriggerWithPrevious(trigger, map[string]any{"state": "ON"}, map[string]any{"state": "ON"}) {
		t.Fatal("identical values must not count as changed")
	}
	if !matchStateTriggerWithPrevious(trigger, map[string]any{"state": "OFF"}, map[string]any{"state": "ON"}) {
		t.Fatal("different values must count as changed")
	}
}

func TestMatchStateTrigger_ConditionsRequireEverySameDeviceCondition(t *testing.T) {
	brightness, _ := json.Marshal(80)
	color, _ := json.Marshal("warm")
	trigger := TriggerDeviceState{Conditions: []StateCondition{
		{Key: "brightness", Op: "gte", Value: brightness},
		{Key: "color_mode", Op: "eq", Value: color},
	}}
	if !matchStateTrigger(trigger, map[string]any{"brightness": 90, "color_mode": "warm"}) {
		t.Fatal("expected all same-device conditions to match")
	}
	if matchStateTrigger(trigger, map[string]any{"brightness": 90, "color_mode": "cool"}) {
		t.Fatal("expected a failing condition to block the trigger")
	}
}

func TestRecordDeviceState_MergesSeparateCapabilityUpdates(t *testing.T) {
	engine := New(nil, nil, Options{})
	_, _ = engine.recordDeviceState("lamp", map[string]any{"brightness": 90})
	_, current := engine.recordDeviceState("lamp", map[string]any{"color_mode": "warm"})
	if current["brightness"] != 90 || current["color_mode"] != "warm" {
		t.Fatalf("expected merged state, got %v", current)
	}
}

func TestMatchesAggregation_AllRequiresEveryTargetToMatch(t *testing.T) {
	engine := New(nil, nil, Options{})
	engine.recordDeviceState("device-a", map[string]any{"temperature": 22.0})
	engine.recordDeviceState("device-b", map[string]any{"temperature": 19.0})
	trigger := TriggerDeviceState{Targets: NodeTargets{Type: "device", IDs: []string{"device-a", "device-b"}}, Key: "temperature", Op: "gte", Value: json.RawMessage("20"), Aggregation: "all"}
	if engine.matchesAggregation(context.Background(), trigger, "device-a") {
		t.Fatal("all aggregation must fail while one target does not match")
	}
	engine.recordDeviceState("device-b", map[string]any{"temperature": 21.0})
	if !engine.matchesAggregation(context.Background(), trigger, "device-b") {
		t.Fatal("all aggregation must match once every target matches")
	}
}

func TestDebounceElapsedWaitsForConfiguredDuration(t *testing.T) {
	engine := New(nil, nil, Options{})
	workflowID := uuid.New()
	if engine.debounceElapsed(workflowID, "trigger", "device", 1) {
		t.Fatal("first matching observation must start debounce")
	}
	engine.stateMu.Lock()
	engine.debounces[workflowID.String()+":trigger:device"] = time.Now().Add(-time.Second)
	engine.stateMu.Unlock()
	if !engine.debounceElapsed(workflowID, "trigger", "device", 1) {
		t.Fatal("elapsed debounce must allow the trigger")
	}
}

func TestDispatchCapabilityStatePatchUsesDeviceHub(t *testing.T) {
	var gotPath, gotCorrelation string
	var gotState map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		gotPath = request.URL.Path
		var payload struct {
			State         map[string]any `json:"state"`
			CorrelationID string         `json:"correlation_id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		gotState, gotCorrelation = payload.State, payload.CorrelationID
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	engine := New(nil, nil, Options{DeviceHubURL: server.URL})
	if err := engine.dispatchCapabilityStatePatch(context.Background(), "test/device", "corr-1", map[string]any{"setpoint": 21}); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if gotPath != "/api/hdp/devices/test/device/commands" || gotCorrelation != "corr-1" || gotState["setpoint"] != float64(21) {
		t.Fatalf("unexpected Device Hub request path=%q correlation=%q state=%v", gotPath, gotCorrelation, gotState)
	}
}

func TestDispatchCapabilityStatePatchSurfacesDeviceHubRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "property is read-only", http.StatusBadRequest)
	}))
	defer server.Close()
	engine := New(nil, nil, Options{DeviceHubURL: server.URL})
	err := engine.dispatchCapabilityStatePatch(context.Background(), "device", "corr", map[string]any{"temperature": 21})
	if err == nil || !strings.Contains(err.Error(), "property is read-only") {
		t.Fatalf("expected Device Hub rejection, got %v", err)
	}
}

func TestShouldIgnoreRetainedState_DropsOldRetainedSnapshot(t *testing.T) {
	e := New(nil, nil, Options{})
	connectedAt := time.Now().UTC()
	e.noteMQTTConnected(connectedAt)
	stateTS := connectedAt.Add(-30 * time.Second).UnixMilli()
	if !e.shouldIgnoreRetainedState(true, stateTS) {
		t.Fatalf("expected old retained snapshot to be ignored")
	}
}

func TestShouldIgnoreRetainedState_AllowsFreshRetainedLiveUpdate(t *testing.T) {
	e := New(nil, nil, Options{})
	connectedAt := time.Now().UTC()
	e.noteMQTTConnected(connectedAt)
	stateTS := connectedAt.Add(2 * time.Second).UnixMilli()
	if e.shouldIgnoreRetainedState(true, stateTS) {
		t.Fatalf("expected fresh retained live update to be processed")
	}
}

func TestShouldIgnoreRetainedState_IgnoresUnknownTimestamp(t *testing.T) {
	e := New(nil, nil, Options{})
	e.noteMQTTConnected(time.Now().UTC())
	if !e.shouldIgnoreRetainedState(true, 0) {
		t.Fatalf("expected retained message with missing timestamp to be ignored")
	}
}
