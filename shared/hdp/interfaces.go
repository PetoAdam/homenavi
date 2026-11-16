package hdp

import (
	"encoding/json"
	"math"
	"strings"
)

const maxInterfaceItems = 128
const maxInterfaceString = 128
const maxInterfaceMetadataBytes = 128 << 10

type InterfaceRange struct {
	Min    *float64  `json:"min,omitempty"`
	Max    *float64  `json:"max,omitempty"`
	Step   *float64  `json:"step,omitempty"`
	Except []float64 `json:"except,omitempty"`
}

type CommandInterface struct {
	CapabilityID string          `json:"capability_id,omitempty"`
	InputID      string          `json:"input_id,omitempty"`
	Property     string          `json:"property,omitempty"`
	ValueType    string          `json:"value_type,omitempty"`
	Access       string          `json:"access"`
	Readable     *bool           `json:"readable,omitempty"`
	Operation    string          `json:"operation"`
	Unit         string          `json:"unit,omitempty"`
	Range        *InterfaceRange `json:"range,omitempty"`
	Options      []string        `json:"options,omitempty"`
	TrueValue    string          `json:"true_value,omitempty"`
	FalseValue   string          `json:"false_value,omitempty"`
	Error        string          `json:"error,omitempty"`
}

type CommandInterfaces struct {
	Status   string             `json:"status"`
	Commands []CommandInterface `json:"commands"`
	Errors   []string           `json:"errors,omitempty"`
}

type interfaceCapability struct {
	ID         string          `json:"id"`
	Property   string          `json:"property"`
	ValueType  string          `json:"value_type"`
	Kind       string          `json:"kind"`
	Unit       string          `json:"unit"`
	Access     json.RawMessage `json:"access"`
	Range      *InterfaceRange `json:"range"`
	Enum       []string        `json:"enum"`
	TrueValue  string          `json:"true_value"`
	FalseValue string          `json:"false_value"`
}

type interfaceInput struct {
	ID           string          `json:"id"`
	CapabilityID string          `json:"capability_id"`
	Property     string          `json:"property"`
	Type         string          `json:"type"`
	Range        *InterfaceRange `json:"range"`
	Access       json.RawMessage `json:"access"`
	Metadata     struct {
		Access json.RawMessage `json:"access"`
	} `json:"metadata"`
	Options []struct {
		Value string `json:"value"`
	} `json:"options"`
}

func compatibleInputAccess(raw json.RawMessage, capabilityAccess string) bool {
	if len(raw) == 0 {
		return true
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	for _, key := range []string{"write", "set", "readOnly"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		var enabled bool
		if json.Unmarshal(value, &enabled) != nil || string(value) == "null" {
			return false
		}
		if key == "readOnly" && enabled && capabilityAccess == "writable" || key != "readOnly" && (capabilityAccess == "writable" && !enabled || capabilityAccess == "read_only" && enabled) {
			return false
		}
	}
	return true
}

func mergeInterfaceRanges(capRange, inputRange *InterfaceRange) (*InterfaceRange, bool) {
	if inputRange == nil {
		return capRange, true
	}
	if capRange == nil {
		return inputRange, true
	}
	merged := *inputRange
	merged.Except = append(append([]float64(nil), capRange.Except...), inputRange.Except...)
	if len(merged.Except) > maxInterfaceItems {
		return nil, false
	}
	if merged.Min == nil {
		merged.Min = capRange.Min
	}
	if merged.Max == nil {
		merged.Max = capRange.Max
	}
	if merged.Step == nil {
		merged.Step = capRange.Step
	}
	if capRange.Min != nil && merged.Min != nil && *merged.Min < *capRange.Min || capRange.Max != nil && merged.Max != nil && *merged.Max > *capRange.Max || !validInterfaceRange(&merged, false) {
		return nil, false
	}
	if capRange.Step != nil && merged.Step != nil {
		ratio := *merged.Step / *capRange.Step
		if math.Abs(ratio-math.Round(ratio)) > 1e-9 {
			return nil, false
		}
		if capRange.Min != nil && merged.Min != nil {
			offset := (*merged.Min - *capRange.Min) / *capRange.Step
			if math.Abs(offset-math.Round(offset)) > 1e-9 {
				return nil, false
			}
		}
	}
	return &merged, true
}

func validInterfaceString(value string, required bool) bool {
	return len(value) <= maxInterfaceString && strings.TrimSpace(value) == value && (!required || value != "")
}

func validInterfaceRange(value *InterfaceRange, integer bool) bool {
	if value == nil {
		return true
	}
	for _, number := range []*float64{value.Min, value.Max, value.Step} {
		if number != nil && (math.IsNaN(*number) || math.IsInf(*number, 0) || integer && math.Trunc(*number) != *number) {
			return false
		}
	}
	if len(value.Except) > maxInterfaceItems {
		return false
	}
	for _, number := range value.Except {
		if math.IsNaN(number) || math.IsInf(number, 0) || integer && math.Trunc(number) != number {
			return false
		}
	}
	return (value.Min == nil || value.Max == nil || *value.Min <= *value.Max) && (value.Step == nil || *value.Step > 0)
}

func interfaceArray(raw json.RawMessage) ([]json.RawMessage, bool) {
	if len(raw) == 0 || len(raw) > maxInterfaceMetadataBytes || string(raw) == "null" {
		return nil, false
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil || values == nil || len(values) > maxInterfaceItems {
		return nil, false
	}
	return values, true
}

func interfaceItemIdentifier(raw json.RawMessage, key string) string {
	var fields map[string]json.RawMessage
	var value string
	if json.Unmarshal(raw, &fields) != nil || json.Unmarshal(fields[key], &value) != nil || !validInterfaceString(value, true) {
		return ""
	}
	return value
}

func interfaceAccess(raw json.RawMessage) (string, bool) {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &fields) != nil || fields == nil {
		return "unknown", false
	}
	var writable bool
	if value, ok := fields["write"]; !ok || json.Unmarshal(value, &writable) != nil || string(value) == "null" {
		return "unknown", false
	}
	if value, ok := fields["read"]; ok {
		var readable bool
		if json.Unmarshal(value, &readable) != nil || string(value) == "null" {
			return "unknown", false
		}
	}
	if writable {
		return "writable", true
	}
	return "read_only", true
}

func compatibleInterfaceType(inputType, valueType string) bool {
	switch inputType {
	case "toggle":
		return valueType == "boolean"
	case "slider", "range", "number", "stepper":
		return valueType == "number" || valueType == "integer"
	case "select":
		return valueType == "enum" || valueType == "string"
	case "text":
		return valueType == "string"
	case "color":
		return valueType == "string" || valueType == "object"
	case "action":
		return true
	case "custom":
		return true
	default:
		return false
	}
}

// DiscoverCommandInterfaces projects untrusted HDP metadata into bounded command schemas.
func DiscoverCommandInterfaces(capabilities, inputs json.RawMessage) CommandInterfaces {
	result := CommandInterfaces{Status: "ok", Commands: []CommandInterface{}}
	capItems, validCaps := interfaceArray(capabilities)
	inputItems, validInputs := interfaceArray(inputs)
	if validCaps && len(capItems) == 0 {
		validCaps = false
	}
	malformed := !validCaps || !validInputs
	if !validCaps {
		result.Status = "unknown"
		result.Errors = append(result.Errors, "capabilities unavailable or malformed")
	}
	if !validInputs {
		result.Status = "unknown"
		result.Errors = append(result.Errors, "inputs unavailable or malformed")
	}
	var caps []interfaceCapability
	for _, item := range capItems {
		var cap interfaceCapability
		if json.Unmarshal(item, &cap) != nil || !validInterfaceString(cap.ID, true) || !validInterfaceString(cap.Property, true) || !validInterfaceString(cap.Unit, false) {
			malformed = true
			result.Status = "unknown"
			result.Errors = append(result.Errors, "invalid capability")
			result.Commands = append(result.Commands, CommandInterface{CapabilityID: interfaceItemIdentifier(item, "id"), Property: interfaceItemIdentifier(item, "property"), Access: "unknown", Operation: "unknown", Error: "invalid capability metadata"})
			continue
		}
		caps = append(caps, cap)
	}
	var parsedInputs []interfaceInput
	for _, item := range inputItems {
		var input interfaceInput
		if json.Unmarshal(item, &input) != nil || !validInterfaceString(input.ID, true) || !validInterfaceString(input.CapabilityID, false) || !validInterfaceString(input.Property, false) || len(input.Options) > maxInterfaceItems {
			malformed = true
			result.Status = "unknown"
			result.Errors = append(result.Errors, "invalid input")
			result.Commands = append(result.Commands, CommandInterface{InputID: interfaceItemIdentifier(item, "id"), Access: "unknown", Operation: "unknown", Error: "invalid input metadata"})
			continue
		}
		parsedInputs = append(parsedInputs, input)
	}
	for _, cap := range caps {
		entry := CommandInterface{CapabilityID: cap.ID, Property: cap.Property, ValueType: cap.ValueType, Unit: cap.Unit, Range: cap.Range, TrueValue: cap.TrueValue, FalseValue: cap.FalseValue, Operation: "unknown"}
		entry.Access, _ = interfaceAccess(cap.Access)
		var access struct {
			Read *bool `json:"read"`
		}
		if json.Unmarshal(cap.Access, &access) == nil {
			entry.Readable = access.Read
		}
		if entry.Access == "writable" {
			if entry.Readable != nil && *entry.Readable {
				entry.Operation = "settable"
			} else if entry.Readable != nil && !*entry.Readable && (cap.Kind == "action" || cap.Property == "action") {
				entry.Operation = "action"
			}
		}
		invalid := malformed || entry.Access == "unknown" || !validInterfaceString(cap.ValueType, true) || !validInterfaceRange(cap.Range, cap.ValueType == "integer") || len(cap.Enum) > maxInterfaceItems
		if !validInterfaceString(cap.TrueValue, false) || !validInterfaceString(cap.FalseValue, false) || (cap.TrueValue != "" || cap.FalseValue != "") && (cap.TrueValue == "" || cap.FalseValue == "" || cap.TrueValue == cap.FalseValue) {
			invalid = true
		}
		if cap.ValueType != "boolean" {
			entry.TrueValue = ""
			entry.FalseValue = ""
		}
		switch cap.ValueType {
		case "boolean", "number", "integer", "string", "enum", "object":
		default:
			invalid = true
		}
		for _, option := range cap.Enum {
			if !validInterfaceString(option, true) {
				invalid = true
			}
		}
		entry.Options = cap.Enum
		matches := 0
		for _, other := range caps {
			if other.ID == cap.ID || other.Property == cap.Property {
				matches++
			}
		}
		if matches != 1 {
			invalid = true
		}
		for _, input := range parsedInputs {
			linked := input.CapabilityID == cap.ID || input.CapabilityID == "" && input.Property == cap.Property
			if !linked {
				if input.CapabilityID != "" && input.Property == cap.Property {
					invalid = true
				}
				continue
			}
			if input.Type == "button" && input.Property != "" && input.Property != cap.Property {
				continue
			}
			if entry.InputID != "" || input.Property != "" && input.Property != cap.Property || !compatibleInterfaceType(input.Type, cap.ValueType) || !validInterfaceRange(input.Range, cap.ValueType == "integer") || !compatibleInputAccess(input.Access, entry.Access) || !compatibleInputAccess(input.Metadata.Access, entry.Access) {
				invalid = true
			}
			entry.InputID = input.ID
			if input.Range != nil {
				merged, ok := mergeInterfaceRanges(cap.Range, input.Range)
				if !ok {
					invalid = true
				}
				entry.Range = merged
			}
			if input.Type == "select" && len(input.Options) > 0 {
				entry.Options = nil
				for _, option := range input.Options {
					if !validInterfaceString(option.Value, true) {
						invalid = true
					}
					if len(cap.Enum) > 0 {
						found := false
						for _, allowed := range cap.Enum {
							if option.Value == allowed {
								found = true
							}
						}
						if !found {
							invalid = true
						}
					}
					entry.Options = append(entry.Options, option.Value)
				}
			}
		}
		if cap.ValueType == "enum" && (len(cap.Enum) == 0 || len(entry.Options) == 0) {
			invalid = true
		}
		if invalid {
			entry.Access = "unknown"
			entry.Operation = "unknown"
			entry.Error = "incomplete or contradictory command metadata"
			entry.Options = nil
			entry.Range = nil
			entry.TrueValue = ""
			entry.FalseValue = ""
			if !validInterfaceString(entry.ValueType, true) {
				entry.ValueType = ""
			}
			result.Status = "unknown"
		}
		result.Commands = append(result.Commands, entry)
	}
	for _, input := range parsedInputs {
		matches := 0
		for _, cap := range caps {
			if input.CapabilityID == cap.ID || input.CapabilityID == "" && input.Property != "" && input.Property == cap.Property {
				matches++
			}
		}
		if matches != 1 {
			result.Status = "unknown"
			result.Commands = append(result.Commands, CommandInterface{InputID: input.ID, Access: "unknown", Operation: "unknown", Error: "input capability unavailable or ambiguous"})
		}
	}
	if encoded, err := json.Marshal(result); err != nil || len(encoded) > 256<<10 {
		return CommandInterfaces{Status: "unknown", Commands: []CommandInterface{}, Errors: []string{"interface metadata exceeds size limit"}}
	}
	return result
}
