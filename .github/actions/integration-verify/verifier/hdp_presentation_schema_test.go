package main

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestHDPPresentationSchema(t *testing.T) {
	schema, err := jsonschema.Compile("../../../../doc/hdp-ui-presentation.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		input string
		valid bool
	}{
		{"legacy", `{"inputs":[{"type":"slider"}],"capabilities":[{"value_type":"number"}]}`, true},
		{"stepper", `{"inputs":[{"type":"slider","ui":{"control":"stepper"}}]}`, true},
		{"gauge", `{"capabilities":[{"value_type":"number","range":{"min":0,"max":100},"ui":{"display":"gauge"}}]}`, true},
		{"wrong control", `{"inputs":[{"type":"select","ui":{"control":"stepper"}}]}`, false},
		{"wrong display", `{"capabilities":[{"value_type":"boolean","range":{"min":0,"max":100},"ui":{"display":"gauge"}}]}`, false},
		{"missing range", `{"capabilities":[{"value_type":"number","ui":{"display":"progress"}}]}`, false},
		{"unknown control", `{"inputs":[{"type":"slider","ui":{"control":"script"}}]}`, false},
		{"missing choice options", `{"inputs":[{"type":"select","ui":{"control":"radio"}}]}`, false},
		{"action buttons", `{"inputs":[{"type":"select","options":[{"value":"stop"}],"ui":{"control":"buttons"}}]}`, true},
		{"text area", `{"inputs":[{"type":"text","ui":{"control":"textarea"}}]}`, true},
		{"invalid step", `{"inputs":[{"type":"slider","range":{"step":0},"ui":{"control":"stepper"}}]}`, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var value any
			if err := json.Unmarshal([]byte(test.input), &value); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(value); (err == nil) != test.valid {
				t.Fatalf("valid=%v, validation error=%v", test.valid, err)
			}
		})
	}
}