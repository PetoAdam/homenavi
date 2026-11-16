package hdp

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDiscoverCommandInterfaces(t *testing.T) {
	capabilities := json.RawMessage(`[{"id":"cover.position","property":"open_percent","value_type":"number","access":{"read":true,"write":true},"range":{"min":0,"max":100,"step":1}},{"id":"cover.action","property":"action","value_type":"enum","enum":["open","stop"],"access":{"read":false,"write":true}},{"id":"cover.status","property":"status","value_type":"string","access":{"read":true,"write":false}}]`)
	inputs := json.RawMessage(`[{"id":"set_open","capability_id":"cover.position","property":"open_percent","type":"slider"},{"id":"set_action","capability_id":"cover.action","property":"action","type":"select","options":[{"value":"open"},{"value":"stop"}]}]`)
	result := DiscoverCommandInterfaces(capabilities, inputs)
	if result.Status != "ok" || len(result.Commands) != 3 || result.Commands[0].Access != "writable" || result.Commands[0].InputID != "set_open" || result.Commands[1].Operation != "action" || result.Commands[2].Access != "read_only" {
		t.Fatalf("unexpected interfaces: %#v", result)
	}
}

func TestDiscoverCommandInterfacesRejectsAmbiguousAndMalformedMetadata(t *testing.T) {
	for _, test := range []struct{ caps, inputs string }{
		{`null`, `[]`},
		{`[]`, `[]`},
		{`[{"id":"a","property":"target","value_type":"number","access":{"write":true}},{"id":"b","property":"target","value_type":"number","access":{"write":true}}]`, `[]`},
		{`[{"id":"a","property":"target","value_type":"number","access":{"write":"yes"}}]`, `[]`},
		{`[{"id":"a","property":"target","value_type":"number","access":{"write":true}}]`, `[{"id":"set","capability_id":"a","property":"other","type":"number"}]`},
		{`[{"id":"a","property":"target","value_type":"number","access":{"write":true}}]`, `null`},
		{`[{"id":"a","property":"target","value_type":"number","access":{"write":true},"range":{"min":0,"max":10}}]`, `[{"id":"set","capability_id":"a","property":"target","type":"number","range":{"min":0,"max":20}}]`},
		{`[{"id":"a","property":"action","value_type":"enum","enum":["open"],"access":{"write":true}}]`, `[{"id":"set","capability_id":"a","property":"action","type":"select","options":[{"value":"close"}]}]`},
		{`[{"id":"a","property":"action","value_type":"enum","access":{"write":true}}]`, `[{"id":"set","capability_id":"a","property":"action","type":"select","options":[{"value":"open"}]}]`},
		{`[{"id":"a","property":"target","value_type":"number","access":{"write":true},"range":{"min":0,"max":10,"step":1}}]`, `[{"id":"set","capability_id":"a","property":"target","type":"number","range":{"min":0,"max":10,"step":0.5}}]`},
		{`[{"id":"a","property":"target","value_type":"number","access":{"write":true}}]`, `[{"id":"set","capability_id":"a","property":"target","type":"number","access":{"write":false}}]`},
		{`[{"id":"a","property":"target","value_type":"number","access":{"write":true}},{"property":"target","value_type":"number"}]`, `[]`},
		{`[{"id":"a","property":"first","value_type":"number","access":{"write":true}},{"id":"b","property":"second","value_type":"number","access":{"write":true}}]`, `[{"id":"set","capability_id":"a","property":"second","type":"number"}]`},
	} {
		result := DiscoverCommandInterfaces(json.RawMessage(test.caps), json.RawMessage(test.inputs))
		if result.Status != "unknown" {
			t.Fatalf("expected unknown metadata: %#v", result)
		}
		for _, command := range result.Commands {
			if command.Access == "writable" {
				t.Fatalf("ambiguous command was writable: %#v", command)
			}
		}
	}
}

func TestDiscoverCommandInterfacesPreservesConsistentReadOnlyInput(t *testing.T) {
	result := DiscoverCommandInterfaces(json.RawMessage(`[{"id":"a","property":"status","value_type":"string","access":{"read":true,"write":false}}]`), json.RawMessage(`[{"id":"status","capability_id":"a","property":"status","type":"text","access":{"write":false,"readOnly":true}}]`))
	if result.Status != "ok" || len(result.Commands) != 1 || result.Commands[0].Access != "read_only" {
		t.Fatalf("unexpected read-only interface: %#v", result)
	}
}

func TestDiscoverCommandInterfacesReportsInvalidItems(t *testing.T) {
	result := DiscoverCommandInterfaces(json.RawMessage(`[{"id":"a","property":"target","value_type":"number","access":{"write":true},"range":"invalid"}]`), json.RawMessage(`[]`))
	if result.Status != "unknown" || len(result.Commands) != 1 || result.Commands[0].CapabilityID != "a" || result.Commands[0].Access != "unknown" || result.Commands[0].Error == "" {
		t.Fatalf("missing per-item error: %#v", result)
	}
}

func TestDiscoverCommandInterfacesReportsDeclaredBooleanMappings(t *testing.T) {
	caps := json.RawMessage(`[{"id":"power","property":"state","value_type":"boolean","true_value":"ON","false_value":"OFF","access":{"read":true,"write":true}}]`)
	inputs := json.RawMessage(`[{"id":"power","capability_id":"power","property":"state","type":"toggle","options":[{"value":"ON"},{"value":"OFF"}]}]`)
	result := DiscoverCommandInterfaces(caps, inputs)
	if result.Status != "ok" || result.Commands[0].TrueValue != "ON" || result.Commands[0].FalseValue != "OFF" || len(result.Commands[0].Options) != 0 {
		t.Fatalf("unexpected boolean schema: %#v", result)
	}
	result = DiscoverCommandInterfaces(json.RawMessage(`[{"id":"power","property":"state","value_type":"boolean","access":{"write":true}}]`), inputs)
	if result.Status != "ok" || len(result.Commands[0].Options) != 0 || result.Commands[0].TrueValue != "" {
		t.Fatalf("presentation options became command values: %#v", result)
	}
}

func TestDiscoverCommandInterfacesAcceptsRangeInput(t *testing.T) {
	caps := json.RawMessage(`[{"id":"media.volume","property":"volume","value_type":"number","access":{"read":true,"write":true}}]`)
	inputs := json.RawMessage(`[{"id":"set_volume","capability_id":"media.volume","property":"volume","type":"range","range":{"min":0,"max":100,"step":1}}]`)
	result := DiscoverCommandInterfaces(caps, inputs)
	if result.Status != "ok" || len(result.Commands) != 1 || result.Commands[0].Access != "writable" || result.Commands[0].Range == nil {
		t.Fatalf("unexpected range schema: %#v", result)
	}
}

func TestDiscoverCommandInterfacesPreservesRangeExclusions(t *testing.T) {
	caps := json.RawMessage(`[{"id":"temperature","property":"temperature","value_type":"number","access":{"read":true,"write":true},"range":{"min":0,"max":10,"step":1,"except":[2]}}]`)
	inputs := json.RawMessage(`[{"id":"set_temperature","capability_id":"temperature","property":"temperature","type":"range","range":{"min":0,"max":10,"step":1,"except":[3]}}]`)
	result := DiscoverCommandInterfaces(caps, inputs)
	if result.Status != "ok" || len(result.Commands) != 1 || result.Commands[0].Range == nil || !reflect.DeepEqual(result.Commands[0].Range.Except, []float64{2, 3}) {
		t.Fatalf("range exclusions lost: %#v", result)
	}
	invalid := json.RawMessage(`[{"id":"set_temperature","capability_id":"temperature","property":"temperature","type":"range","range":{"except":["invalid"]}}]`)
	if result := DiscoverCommandInterfaces(caps, invalid); result.Status != "unknown" {
		t.Fatalf("invalid exclusion accepted: %#v", result)
	}
}

func TestDiscoverCommandInterfacesIgnoresButtonAliases(t *testing.T) {
	caps := json.RawMessage(`[{"id":"washer.control","property":"operation_mode","value_type":"string","access":{"read":true,"write":true}}]`)
	inputs := json.RawMessage(`[{"id":"start","capability_id":"washer.control","property":"start","type":"button"},{"id":"stop","capability_id":"washer.control","property":"stop","type":"button"},{"id":"set_operation_mode","capability_id":"washer.control","property":"operation_mode","type":"select","options":[{"value":"START"},{"value":"STOP"}]}]`)
	result := DiscoverCommandInterfaces(caps, inputs)
	if result.Status != "ok" || len(result.Commands) != 1 || result.Commands[0].Access != "writable" || result.Commands[0].Property != "operation_mode" || result.Commands[0].InputID != "set_operation_mode" {
		t.Fatalf("unexpected washer schema: %#v", result)
	}
}

func TestDiscoverCommandInterfacesHandlesCustomInputs(t *testing.T) {
	caps := json.RawMessage(`[{"id":"level_config","property":"level_config","value_type":"object","access":{"read":true,"write":true}},{"id":"execute_if_off","property":"execute_if_off","value_type":"string","true_value":"true","false_value":"false","access":{"read":true,"write":true}}]`)
	inputs := json.RawMessage(`[{"id":"level_config","capability_id":"level_config","property":"level_config","type":"custom"},{"id":"execute_if_off","capability_id":"execute_if_off","property":"execute_if_off","type":"custom"}]`)
	result := DiscoverCommandInterfaces(caps, inputs)
	if result.Status != "ok" || len(result.Commands) != 2 || result.Commands[0].Access != "writable" || result.Commands[1].Access != "writable" || result.Commands[1].TrueValue != "" || result.Commands[1].FalseValue != "" {
		t.Fatalf("unexpected custom input schemas: %#v", result)
	}
}

func TestDiscoverCommandInterfacesDoesNotGrantAccessFromPresentation(t *testing.T) {
	caps := json.RawMessage(`[{"id":"status","property":"status","value_type":"string","access":{"read":true,"write":false}}]`)
	for _, inputs := range []json.RawMessage{
		json.RawMessage(`[{"id":"status","capability_id":"status","property":"status","type":"custom"}]`),
		json.RawMessage(`[{"id":"start","capability_id":"status","property":"start","type":"button"}]`),
	} {
		result := DiscoverCommandInterfaces(caps, inputs)
		if result.Status != "ok" || len(result.Commands) != 1 || result.Commands[0].Access != "read_only" || result.Commands[0].Property != "status" {
			t.Fatalf("presentation granted write access: %#v", result)
		}
	}
}
