package adapter

import (
	"fmt"
	"strings"

	"github.com/PetoAdam/homenavi/shared/mockdemo"
)

type demoProfile string

const (
	demoProfileLight       demoProfile = "light"
	demoProfilePlug        demoProfile = "plug"
	demoProfileBlind       demoProfile = "blind"
	demoProfileSensor      demoProfile = "sensor"
	demoProfileAirPurifier demoProfile = "air_purifier"
)

type demoDevice struct {
	HDPDeviceID  string
	Type         string
	Manufacturer string
	Model        string
	Description  string
	Icon         string
	Profile      demoProfile
	Capabilities []map[string]any
	Inputs       []map[string]any
	State        map[string]any
}

func demoCatalog() map[string]*demoDevice {
	devices := []*demoDevice{
		{
			HDPDeviceID:  mockdemo.SofaLampHDPDeviceID,
			Type:         "light",
			Manufacturer: "MockWorks",
			Model:        "Aura Floor One",
			Description:  "Warm standing lamp by the sofa",
			Icon:         "lightbulb",
			Profile:      demoProfileLight,
			Capabilities: lightCapabilities(),
			Inputs:       lightInputs(),
			State:        map[string]any{"on": true, "brightness": 38},
		},
		{
			HDPDeviceID:  mockdemo.TVBacklightHDPDeviceID,
			Type:         "light",
			Manufacturer: "MockWorks",
			Model:        "Glow Strip Mini",
			Description:  "Ambient strip behind the TV",
			Icon:         "lightbulb",
			Profile:      demoProfileLight,
			Capabilities: lightCapabilities(),
			Inputs:       lightInputs(),
			State:        map[string]any{"on": true, "brightness": 24},
		},
		{
			HDPDeviceID:  mockdemo.KitchenPendantHDPDeviceID,
			Type:         "light",
			Manufacturer: "MockWorks",
			Model:        "Pendant Beam",
			Description:  "Main light over the island",
			Icon:         "lightbulb",
			Profile:      demoProfileLight,
			Capabilities: lightCapabilities(),
			Inputs:       lightInputs(),
			State:        map[string]any{"on": false, "brightness": 0},
		},
		{
			HDPDeviceID:  mockdemo.CoffeeMakerHDPDeviceID,
			Type:         "switch",
			Manufacturer: "MockWorks",
			Model:        "Brew Socket",
			Description:  "Countertop coffee machine plug",
			Icon:         "plug",
			Profile:      demoProfilePlug,
			Capabilities: plugCapabilities(),
			Inputs:       plugInputs(),
			State:        map[string]any{"power": "off", "power_draw": 0.6},
		},
		{
			HDPDeviceID:  mockdemo.BedroomBlindHDPDeviceID,
			Type:         "cover",
			Manufacturer: "MockWorks",
			Model:        "Shade Glide",
			Description:  "Motorized blackout blind",
			Icon:         "blinds",
			Profile:      demoProfileBlind,
			Capabilities: blindCapabilities(),
			Inputs:       blindInputs(),
			State:        map[string]any{"position": 72},
		},
		{
			HDPDeviceID:  mockdemo.DeskLampHDPDeviceID,
			Type:         "light",
			Manufacturer: "MockWorks",
			Model:        "Task Beam",
			Description:  "Task light on the office desk",
			Icon:         "lightbulb",
			Profile:      demoProfileLight,
			Capabilities: lightCapabilities(),
			Inputs:       lightInputs(),
			State:        map[string]any{"on": true, "brightness": 61},
		},
		{
			HDPDeviceID:  mockdemo.EntrySensorHDPDeviceID,
			Type:         "sensor",
			Manufacturer: "MockWorks",
			Model:        "Entry Contact",
			Description:  "Door contact sensor at the main entrance",
			Icon:         "door",
			Profile:      demoProfileSensor,
			Capabilities: contactCapabilities(),
			Inputs:       []map[string]any{},
			State:        map[string]any{"contact": false, "battery": 92},
		},
		{
			HDPDeviceID:  mockdemo.AirPurifierHDPDeviceID,
			Type:         "fan",
			Manufacturer: "MockWorks",
			Model:        "ClearAir S",
			Description:  "Living room air purifier",
			Icon:         "fan",
			Profile:      demoProfileAirPurifier,
			Capabilities: airPurifierCapabilities(),
			Inputs:       airPurifierInputs(),
			State:        map[string]any{"on": true, "fan_speed": "medium", "air_quality": 17},
		},
	}
	out := make(map[string]*demoDevice, len(devices))
	for _, device := range devices {
		copyDevice := *device
		copyDevice.Capabilities = cloneArrayOfMaps(device.Capabilities)
		copyDevice.Inputs = cloneArrayOfMaps(device.Inputs)
		copyDevice.State = cloneAnyMap(device.State)
		out[device.HDPDeviceID] = &copyDevice
	}
	return out
}

func lightCapabilities() []map[string]any {
	return []map[string]any{
		{"id": "on", "name": "Power", "kind": "binary", "property": "on", "value_type": "boolean", "access": map[string]any{"read": true, "write": true}},
		{"id": "brightness", "name": "Brightness", "kind": "numeric", "property": "brightness", "value_type": "number", "access": map[string]any{"read": true, "write": true}, "range": map[string]any{"min": 0, "max": 100, "step": 1}, "unit": "%"},
	}
}

func lightInputs() []map[string]any {
	return []map[string]any{
		{"id": "on", "label": "Power", "type": "toggle", "capability_id": "on", "property": "on"},
		{"id": "brightness", "label": "Brightness", "type": "slider", "capability_id": "brightness", "property": "brightness", "range": map[string]any{"min": 0, "max": 100, "step": 1}},
	}
}

func plugCapabilities() []map[string]any {
	return []map[string]any{
		{"id": "power", "name": "Power", "kind": "binary", "property": "power", "value_type": "boolean", "access": map[string]any{"read": true, "write": true}},
		{"id": "power_draw", "name": "Power Draw", "kind": "numeric", "property": "power_draw", "value_type": "number", "access": map[string]any{"read": true}, "unit": "W"},
	}
}

func plugInputs() []map[string]any {
	return []map[string]any{{"id": "power", "label": "Power", "type": "toggle", "capability_id": "power", "property": "power", "metadata": map[string]any{"togglePowerString": true}}}
}

func blindCapabilities() []map[string]any {
	return []map[string]any{{"id": "position", "name": "Position", "kind": "numeric", "property": "position", "value_type": "number", "access": map[string]any{"read": true, "write": true}, "range": map[string]any{"min": 0, "max": 100, "step": 1}, "unit": "%"}}
}

func blindInputs() []map[string]any {
	return []map[string]any{{"id": "position", "label": "Position", "type": "slider", "capability_id": "position", "property": "position", "range": map[string]any{"min": 0, "max": 100, "step": 1}}}
}

func contactCapabilities() []map[string]any {
	return []map[string]any{
		{"id": "contact", "name": "Contact", "kind": "binary", "property": "contact", "value_type": "boolean", "access": map[string]any{"read": true}},
		{"id": "battery", "name": "Battery", "kind": "numeric", "property": "battery", "value_type": "number", "access": map[string]any{"read": true}, "range": map[string]any{"min": 0, "max": 100, "step": 1}, "unit": "%"},
	}
}

func airPurifierCapabilities() []map[string]any {
	return []map[string]any{
		{"id": "on", "name": "Power", "kind": "binary", "property": "on", "value_type": "boolean", "access": map[string]any{"read": true, "write": true}},
		{"id": "fan_speed", "name": "Fan Speed", "kind": "enum", "property": "fan_speed", "value_type": "enum", "access": map[string]any{"read": true, "write": true}, "enum": []string{"low", "medium", "high", "auto"}},
		{"id": "air_quality", "name": "Air Quality", "kind": "numeric", "property": "air_quality", "value_type": "number", "access": map[string]any{"read": true}, "unit": "AQI"},
	}
}

func airPurifierInputs() []map[string]any {
	return []map[string]any{
		{"id": "on", "label": "Power", "type": "toggle", "capability_id": "on", "property": "on"},
		{"id": "fan_speed", "label": "Fan Speed", "type": "select", "capability_id": "fan_speed", "property": "fan_speed", "options": []map[string]any{{"value": "low", "label": "Low"}, {"value": "medium", "label": "Medium"}, {"value": "high", "label": "High"}, {"value": "auto", "label": "Auto"}}},
	}
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneArrayOfMaps(in []map[string]any) []map[string]any {
	if len(in) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(in))
	for _, item := range in {
		copied := map[string]any{}
		for key, value := range item {
			copied[key] = value
		}
		out = append(out, copied)
	}
	return out
}

func normalizeDemoHDPDeviceID(deviceID string) string {
	trimmed := strings.Trim(strings.TrimSpace(deviceID), "/")
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "mock/") {
		return trimmed
	}
	return "mock/" + trimmed
}

func clampPercent(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return int(v + 0.5)
}

func normalizeBool(v any) (bool, bool) {
	switch value := v.(type) {
	case bool:
		return value, true
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "on", "true", "1", "open":
			return true, true
		case "off", "false", "0", "closed":
			return false, true
		}
	case float64:
		return value != 0, true
	case int:
		return value != 0, true
	case int64:
		return value != 0, true
	}
	return false, false
}

func normalizeFloat(v any) (float64, bool) {
	switch value := v.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case int32:
		return float64(value), true
	}
	return 0, false
}

func normalizeFanSpeed(v any) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	switch value {
	case "low", "medium", "high", "auto":
		return value, true
	default:
		return "", false
	}
}
