package zigbee

import (
    "encoding/json"
    "fmt"
    "math"
    "strings"

    "zigbee-adapter/internal/model"
    "zigbee-adapter/internal/proto/adapterutil"
)

func extractExposes(raw map[string]any) ([]any, bool) {
    if raw == nil {
        return nil, false
    }
    if def, ok := raw["definition"].(map[string]any); ok {
        if exposes, ok := def["exposes"].([]any); ok {
            return exposes, true
        }
        if exposes, ok := def["exposes"].([]interface{}); ok {
            return exposes, true
        }
    }
    if exposes, ok := raw["exposes"].([]any); ok {
        return exposes, true
    }
    if exposes, ok := raw["exposes"].([]interface{}); ok {
        return exposes, true
    }
    return nil, false
}

func normalizeValueForCapability(cap model.Capability, v any) any {
    switch cap.ValueType {
    case "boolean":
        return adapterutil.CoerceBool(v, cap.TrueValue, cap.FalseValue)
    case "number":
        if f, ok := adapterutil.NumericValue(v); ok {
            if cap.Range != nil && cap.Range.Step > 0 {
                step := cap.Range.Step
                f = math.Round(f/step) * step
            }
            return f
        }
    case "enum", "string":
        return fmt.Sprint(v)
    default:
        return v
    }
    return v
}

func normalizeLooseValue(prop string, v any) any {
    switch strings.ToLower(prop) {
    case "state", "on":
        return adapterutil.CoerceBool(v, "", "")
    }
    if f, ok := adapterutil.NumericValue(v); ok {
        return f
    }
    return v
}

func buildCapabilitiesFromExposes(exposes []any) ([]model.Capability, []model.DeviceInput, []string, map[string]model.Capability) {
    caps := []model.Capability{}
    inputs := []model.DeviceInput{}
    refresh := []string{}
    capMap := map[string]model.Capability{}
    for _, raw := range exposes {
        extractCapability(raw, "", &caps, &inputs, &refresh, capMap)
    }
    if len(refresh) > 0 {
        refresh = adapterutil.UniqueStrings(refresh)
    }
    return caps, inputs, refresh, capMap
}

func extractCapability(raw any, parentKind string, caps *[]model.Capability, inputs *[]model.DeviceInput, refresh *[]string, capMap map[string]model.Capability) {
    m, ok := raw.(map[string]any)
    if !ok {
        return
    }

    kind := parentKind
    if t := adapterutil.StringField(m, "type"); t != "" {
        kind = strings.ToLower(t)
    }
    property := strings.ToLower(adapterutil.StringField(m, "property"))
    name := adapterutil.StringField(m, "name")
    description := adapterutil.StringField(m, "description")
    unit := adapterutil.StringField(m, "unit")
    deviceClass := adapterutil.StringField(m, "device_class")
    measurement := adapterutil.StringField(m, "measurement")
    if measurement == "" {
        measurement = adapterutil.StringField(m, "measurement_type")
    }
    access := parseAccess(m["access"])
    enumValues := adapterutil.StringSliceFromAny(m["values"])
    rng := parseRange(m)
    trueVal := adapterutil.StringField(m, "value_on")
    falseVal := adapterutil.StringField(m, "value_off")

    features, hasChildren := m["features"].([]any)

    includeSelf := !hasChildren || property != ""
    var capID string
    if includeSelf {
        capID = makeCapabilityID(property, name, len(*caps))
        cap := model.Capability{
            ID:          capID,
            Name:        humanizeName(name, property, kind, capID),
            Kind:        kind,
            Property:    property,
            ValueType:   inferValueType(kind, enumValues, rng, property, m),
            Unit:        unit,
            DeviceClass: deviceClass,
            Measurement: measurement,
            Access:      access,
            Description: description,
        }
        if parentKind != "" && parentKind != kind {
            cap.SubType = parentKind
        }
        if rng != nil {
            cap.Range = rng
        }
        if len(enumValues) > 0 {
            cap.Enum = enumValues
        }
        if trueVal != "" {
            cap.TrueValue = trueVal
        }
        if falseVal != "" {
            cap.FalseValue = falseVal
        }
        if cap.ValueType == "boolean" {
            if cap.TrueValue == "" {
                cap.TrueValue = "ON"
            }
            if cap.FalseValue == "" {
                cap.FalseValue = "OFF"
            }
        }
        *caps = append(*caps, cap)
        if property != "" {
            capMap[property] = cap
            if access.Read {
                *refresh = append(*refresh, property)
            }
        }
        if access.Write {
            input := buildInputForCapability(cap, enumValues, trueVal, falseVal, m)
            *inputs = append(*inputs, input)
        }
    }

    if hasChildren {
        for _, child := range features {
            extractCapability(child, kind, caps, inputs, refresh, capMap)
        }
    }
}

func parseAccess(v any) model.CapabilityAccess {
    access := 1
    switch val := v.(type) {
    case float64:
        access = int(val)
    case int:
        access = val
    case int64:
        access = int(val)
    case json.Number:
        if i, err := val.Int64(); err == nil {
            access = int(i)
        }
    }
    return model.CapabilityAccess{
        Read:  access&1 != 0,
        Write: access&2 != 0,
        Event: access&4 != 0,
    }
}

func parseRange(m map[string]any) *model.CapabilityRange {
    min, minOK := adapterutil.FloatFromAny(m["value_min"])
    max, maxOK := adapterutil.FloatFromAny(m["value_max"])
    if !minOK && !maxOK {
        return nil
    }
    rng := &model.CapabilityRange{}
    if minOK {
        rng.Min = min
    }
    if maxOK {
        rng.Max = max
    }
    if step, ok := adapterutil.FloatFromAny(m["value_step"]); ok {
        rng.Step = step
    }
    return rng
}

func inferValueType(kind string, enumValues []string, rng *model.CapabilityRange, property string, raw map[string]any) string {
    lowerKind := strings.ToLower(kind)
    switch lowerKind {
    case "binary", "switch":
        if property == "state" || property == "contact" || property == "occupancy" {
            return "boolean"
        }
    case "light":
        if property == "state" {
            return "boolean"
        }
        if property == "brightness" || property == "color_temp" {
            return "number"
        }
    case "numeric":
        return "number"
    case "enum":
        return "enum"
    case "composite":
        if property == "color" {
            return "object"
        }
    }
    if len(enumValues) > 0 {
        return "enum"
    }
    if rng != nil {
        return "number"
    }
    if property == "linkquality" || strings.Contains(property, "battery") {
        return "number"
    }
    if _, ok := raw["features"]; ok {
        return "object"
    }
    return "string"
}

func makeCapabilityID(property, name string, idx int) string {
    if property != "" {
        return property
    }
    if name != "" {
        return slugify(name)
    }
    return fmt.Sprintf("cap_%d", idx)
}

func slugify(v string) string {
    v = strings.TrimSpace(strings.ToLower(v))
    if v == "" {
        return ""
    }
    var b strings.Builder
    for _, r := range v {
        switch {
        case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
            b.WriteRune(r)
        case r == ' ' || r == '-' || r == '/':
            b.WriteRune('_')
        }
    }
    out := b.String()
    if out == "" {
        return v
    }
    return out
}

func humanizeName(name, property, kind, fallback string) string {
    if name != "" {
        return name
    }
    if property != "" {
        return adapterutil.TitleCase(strings.ReplaceAll(property, "_", " "))
    }
    if kind != "" {
        return adapterutil.TitleCase(kind)
    }
    return fallback
}

func buildInputForCapability(cap model.Capability, enumValues []string, trueVal, falseVal string, raw map[string]any) model.DeviceInput {
    input := model.DeviceInput{
        ID:           cap.ID,
        Label:        cap.Name,
        Type:         determineInputType(cap, enumValues),
        CapabilityID: cap.ID,
        Property:     cap.Property,
    }
    if cap.Range != nil {
        input.Range = cap.Range
    }
    switch input.Type {
    case "toggle":
        if input.Metadata == nil {
            input.Metadata = map[string]any{}
        }
        if trueVal == "" {
            trueVal = "ON"
        }
        if falseVal == "" {
            falseVal = "OFF"
        }
        input.Metadata["true_value"] = trueVal
        input.Metadata["false_value"] = falseVal
        input.Options = []model.InputOption{{Value: falseVal, Label: "Off"}, {Value: trueVal, Label: "On"}}
    case "select":
        opts := make([]model.InputOption, 0, len(enumValues))
        for _, v := range enumValues {
            opts = append(opts, model.InputOption{Value: v, Label: adapterutil.TitleCase(v)})
        }
        input.Options = opts
    case "color":
        if input.Metadata == nil {
            input.Metadata = map[string]any{}
        }
        input.Metadata["mode"] = cap.Kind
    }
    return input
}

func determineInputType(cap model.Capability, enumValues []string) string {
    prop := cap.Property
    switch cap.ValueType {
    case "boolean":
        return "toggle"
    case "number":
        if prop == "color_temp" || cap.Range != nil {
            return "slider"
        }
        return "number"
    case "enum":
        return "select"
    default:
        if strings.Contains(prop, "color") {
            return "color"
        }
        return "custom"
    }
}
