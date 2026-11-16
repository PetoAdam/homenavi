package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateHDPMetadataDir(t *testing.T) {
	schema := "../../../../doc/hdp-ui-presentation.schema.json"
	cases := []struct {
		name, sample string
		required     bool
		valid        bool
	}{
		{"optional empty", "", false, true},
		{"required empty", "", true, false},
		{"malformed", "{", true, false},
		{"empty metadata", `{"capabilities":[]}`, true, false},
		{"valid", `{"capabilities":[{"id":"target","value_type":"number"}],"inputs":[{"id":"set","type":"slider","capability_id":"target","range":{"min":5,"max":35,"step":0.5},"ui":{"control":"stepper"}}]}`, true, true},
		{"incompatible type", `{"capabilities":[{"id":"target","value_type":"boolean"}],"inputs":[{"type":"slider","capability_id":"target","ui":{"control":"stepper"}}]}`, true, false},
		{"incompatible property link", `{"capabilities":[{"property":"target","value_type":"boolean"}],"inputs":[{"type":"slider","property":"target","ui":{"control":"stepper"}}]}`, true, false},
		{"missing capability", `{"inputs":[{"type":"slider","capability_id":"missing","ui":{"control":"stepper"}}]}`, true, false},
		{"reversed range", `{"inputs":[{"type":"slider","range":{"min":35,"max":5},"ui":{"control":"stepper"}}]}`, true, false},
		{"fractional integer", `{"capabilities":[{"id":"target","value_type":"integer"}],"inputs":[{"type":"slider","capability_id":"target","range":{"step":0.5},"ui":{"control":"stepper"}}]}`, true, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.sample != "" {
				sample := strings.Replace(test.sample, "{", `{"schema":"hdp.v1","type":"metadata",`, 1)
				if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(sample), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := validateHDPMetadataDir(dir, schema, test.required); (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}
