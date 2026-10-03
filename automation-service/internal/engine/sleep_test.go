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

func TestDefinitionNormalizeAndValidate_AcceptsChangedCapabilityTrigger(t *testing.T) {
	definition := Definition{Version: "automation", Nodes: []NodeDef{{
		ID: "trigger", Kind: "trigger.device_state", Data: json.RawMessage(`{"targets":{"type":"device","ids":["device"]},"capability_id":"state","key":"state","op":"changed","aggregation":"each","debounce_sec":2}`),
	}}}
	if err := definition.NormalizeAndValidate(); err != nil {
		t.Fatalf("expected capability changed trigger to validate: %v", err)
	}
}

func TestDefinitionNormalizeAndValidate_CompleteCapabilityAutomation(t *testing.T) {
	definition := Definition{Version: "automation", Nodes: []NodeDef{
		{ID: "trigger", Kind: "trigger.device_state", Data: json.RawMessage(`{"targets":{"type":"selector","selector":"group:lights"},"capability_id":"brightness","key":"brightness","op":"lt","value":20,"aggregation":"all","debounce_sec":5,"cooldown_sec":30}`)},
		{ID: "command", Kind: "action.send_command", Data: json.RawMessage(`{"targets":{"type":"selector","selector":"group:lights"},"command":"set_state","capability_id":"on","capability_value":true,"args":{"on":true}}`)},
		{ID: "notify", Kind: "action.notify_email", Data: json.RawMessage(`{"user_ids":["user-1"],"subject":"Lights enabled","message":"All lights were dim."}`)},
	}, Edges: []EdgeDef{{From: "trigger", To: "command"}, {From: "command", To: "notify"}}}
	if err := definition.NormalizeAndValidate(); err != nil {
		t.Fatalf("expected complete capability workflow to validate: %v", err)
	}
}

func TestDefinitionNormalizeAndValidate_RejectsInvalidCompleteWorkflows(t *testing.T) {
	cases := []Definition{
		{Version: "automation", Nodes: []NodeDef{{ID: "trigger", Kind: "trigger.device_state", Data: json.RawMessage(`{"targets":{"type":"device","ids":["device"]},"capability_id":"on","aggregation":"sometimes"}`)}}},
		{Version: "automation", Nodes: []NodeDef{{ID: "trigger", Kind: "trigger.device_state", Data: json.RawMessage(`{"targets":{"type":"device","ids":["device"]},"op":"between"}`)}}},
		{Version: "automation", Nodes: []NodeDef{{ID: "trigger", Kind: "trigger.manual", Data: json.RawMessage(`{}`)}, {ID: "action", Kind: "action.send_command", Data: json.RawMessage(`{"targets":{"type":"device","ids":["one","two"]},"wait_for_result":true}`)}}, Edges: []EdgeDef{{From: "trigger", To: "action"}}},
		{Version: "automation", Nodes: []NodeDef{{ID: "trigger", Kind: "trigger.manual", Data: json.RawMessage(`{}`)}, {ID: "action", Kind: "logic.sleep", Data: json.RawMessage(`{"duration_sec":1}`)}}, Edges: []EdgeDef{{From: "trigger", To: "action"}, {From: "action", To: "trigger"}}},
	}
	for index, definition := range cases {
		if err := definition.NormalizeAndValidate(); err == nil {
			t.Fatalf("case %d: expected validation error", index)
		}
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
