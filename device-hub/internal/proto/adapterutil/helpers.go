package adapterutil

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"device-hub/internal/model"
)

// SanitizeString removes NUL runes so persistence layers accept the value.
func SanitizeString(s string) string {
	if s == "" {
		return ""
	}
	return strings.Map(func(r rune) rune {
		if r == 0 {
			return -1
		}
		return r
	}, s)
}

// SanitizeDeviceStrings applies SanitizeString to common string fields on a device.
func SanitizeDeviceStrings(dev *model.Device) {
	if dev == nil {
		return
	}
	dev.ExternalID = SanitizeString(dev.ExternalID)
	dev.Name = SanitizeString(dev.Name)
	dev.Type = SanitizeString(dev.Type)
	dev.Manufacturer = SanitizeString(dev.Manufacturer)
	dev.Model = SanitizeString(dev.Model)
	dev.Description = SanitizeString(dev.Description)
	dev.Firmware = SanitizeString(dev.Firmware)
}

// NormalizeExternalKey normalizes an external identifier for consistent map lookups.
func NormalizeExternalKey(external string) string {
	return strings.ToLower(strings.TrimSpace(external))
}

// StringField extracts a string representation from a loosely typed map entry.
func StringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case string:
			return val
		case fmt.Stringer:
			return val.String()
		default:
			return fmt.Sprint(val)
		}
	}
	return ""
}

// CoerceBool converts various representations into a boolean value.
func CoerceBool(v any, trueVal, falseVal string) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		s := strings.TrimSpace(strings.ToLower(val))
		if trueVal != "" && strings.EqualFold(val, trueVal) {
			return true
		}
		if falseVal != "" && strings.EqualFold(val, falseVal) {
			return false
		}
		if s == "on" || s == "true" || s == "1" || s == "yes" {
			return true
		}
		if s == "off" || s == "false" || s == "0" || s == "no" {
			return false
		}
	case float64:
		return val != 0
	case float32:
		return val != 0
	case int:
		return val != 0
	case int64:
		return val != 0
	case uint64:
		return val != 0
	}
	return false
}

// NumericValue attempts to coerce the input into a float64.
func NumericValue(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// StringSliceFromAny attempts to coerce a generic slice into a string slice.
func StringSliceFromAny(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		if generic, ok2 := v.([]interface{}); ok2 {
			arr = generic
			ok = true
		}
	}
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

// FloatFromAny attempts to coerce the input into a float64.
func FloatFromAny(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	}
	return 0, false
}

// UniqueStrings returns a copy of input slice with duplicates removed, preserving order.
func UniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// TitleCase converts underscores/dashes/spaces into title case words.
func TitleCase(v string) string {
	if v == "" {
		return ""
	}
	splitFn := func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	}
	parts := strings.FieldsFunc(strings.ToLower(v), splitFn)
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
