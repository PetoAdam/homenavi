package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func validateHDPMetadataDir(dir, schemaPath string, required bool) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return fmt.Errorf("find samples: %w", err)
	}
	if len(files) == 0 {
		if required {
			return fmt.Errorf("no generated metadata samples in %s", dir)
		}
		fmt.Fprintln(os.Stderr, "WARN: no HDP metadata samples generated; runtime interfaces were not validated")
		return nil
	}
	schema, err := jsonschema.Compile(schemaPath)
	if err != nil {
		return fmt.Errorf("compile presentation schema: %w", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
		var metadata map[string]any
		if err := json.Unmarshal(data, &metadata); err != nil || metadata == nil {
			return fmt.Errorf("invalid metadata object %s", file)
		}
		if metadata["schema"] != "hdp.v1" || metadata["type"] != "metadata" || (len(jsonArray(metadata["capabilities"])) == 0 && len(jsonArray(metadata["inputs"])) == 0) {
			return fmt.Errorf("%s: expected HDP metadata with at least one capability or input", file)
		}
		if err := schema.Validate(metadata); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if err := validatePresentationLinks(metadata); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
	}
	return nil
}

func validatePresentationLinks(metadata map[string]any) error {
	capabilities := map[string]map[string]any{}
	for _, raw := range jsonArray(metadata["capabilities"]) {
		capability, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"id", "property"} {
			if value, ok := capability[key].(string); ok && value != "" {
				capabilities[strings.ToLower(value)] = capability
			}
		}
		ui, _ := capability["ui"].(map[string]any)
		if ui["display"] == "progress" || ui["display"] == "gauge" {
			if err := validateRange(capability["range"], false); err != nil {
				return fmt.Errorf("capability %v: %w", capability["id"], err)
			}
		}
	}
	allowed := map[string]string{
		"toggle": "boolean bool", "slider": "number integer float double int", "number": "number integer float double int",
		"select": "enum string", "text": "string", "color": "string object color",
	}
	for _, raw := range jsonArray(metadata["inputs"]) {
		input, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		ui, _ := input["ui"].(map[string]any)
		if _, present := ui["control"]; !present {
			continue
		}
		inputType, _ := input["type"].(string)
		capabilityID, _ := input["capability_id"].(string)
		capability := capabilities[strings.ToLower(capabilityID)]
		if capabilityID != "" && capability == nil {
			return fmt.Errorf("input %v: unknown capability %q", input["id"], capabilityID)
		}
		if capability == nil {
			for _, key := range []string{"property", "id"} {
				if value, ok := input[key].(string); ok && capabilities[strings.ToLower(value)] != nil {
					capability = capabilities[strings.ToLower(value)]
					break
				}
			}
		}
		valueType, _ := capability["value_type"].(string)
		valueType = strings.ToLower(valueType)
		if valueType != "" && !strings.Contains(" "+allowed[inputType]+" ", " "+strings.ToLower(valueType)+" ") {
			return fmt.Errorf("input %v: type %q conflicts with capability value_type %q", input["id"], inputType, valueType)
		}
		if inputType == "number" || inputType == "slider" {
			if err := validateRange(input["range"], valueType == "integer" || valueType == "int"); err != nil {
				return fmt.Errorf("input %v: %w", input["id"], err)
			}
		}
	}
	return nil
}

func jsonArray(value any) []any {
	items, _ := value.([]any)
	return items
}

func validateRange(value any, integer bool) error {
	if value == nil {
		return nil
	}
	rangeValue, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("range must be an object")
	}
	for _, key := range []string{"min", "max", "step"} {
		if value, exists := rangeValue[key]; exists {
			number, ok := value.(float64)
			if !ok || math.IsNaN(number) || math.IsInf(number, 0) || (integer && math.Trunc(number) != number) || (key == "step" && number <= 0) {
				return fmt.Errorf("invalid range %s", key)
			}
		}
	}
	min, hasMin := rangeValue["min"].(float64)
	max, hasMax := rangeValue["max"].(float64)
	if hasMin && hasMax && min >= max {
		return fmt.Errorf("range min must be less than max")
	}
	return nil
}
