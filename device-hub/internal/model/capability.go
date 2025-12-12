package model

type Capability struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Kind        string           `json:"kind"`
	Property    string           `json:"property"`
	ValueType   string           `json:"value_type"`
	Unit        string           `json:"unit,omitempty"`
	DeviceClass string           `json:"device_class,omitempty"`
	Measurement string           `json:"measurement_type,omitempty"`
	Access      CapabilityAccess `json:"access"`
	Description string           `json:"description,omitempty"`
	Range       *CapabilityRange `json:"range,omitempty"`
	Enum        []string         `json:"enum,omitempty"`
	SubType     string           `json:"sub_type,omitempty"`
	TrueValue   string           `json:"true_value,omitempty"`
	FalseValue  string           `json:"false_value,omitempty"`
}

type CapabilityAccess struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
	Event bool `json:"event"`
}

type CapabilityRange struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Step float64 `json:"step,omitempty"`
}

type DeviceInput struct {
	ID           string           `json:"id"`
	Label        string           `json:"label"`
	Type         string           `json:"type"` // toggle, slider, select, color, numeric
	CapabilityID string           `json:"capability_id"`
	Property     string           `json:"property"`
	Range        *CapabilityRange `json:"range,omitempty"`
	Options      []InputOption    `json:"options,omitempty"`
	Metadata     map[string]any   `json:"metadata,omitempty"`
}

type InputOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
