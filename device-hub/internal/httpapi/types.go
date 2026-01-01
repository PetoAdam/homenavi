package httpapi

import "encoding/json"

type commandRequest struct {
	State         map[string]any `json:"state"`
	TransitionMs  *int           `json:"transition_ms"`
	CorrelationID string         `json:"correlation_id"`
}

type commandResponse struct {
	Status        string `json:"status"`
	DeviceID      string `json:"device_id"`
	TransitionMs  *int   `json:"transition_ms,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

type deviceUpdateRequest struct {
	Icon *string `json:"icon"`
}

type deviceUpdateResponse struct {
	Status   string `json:"status"`
	DeviceID string `json:"device_id"`
	Icon     string `json:"icon,omitempty"`
}

type deviceCreateRequest struct {
	Protocol     string          `json:"protocol"`
	ExternalID   string          `json:"external_id"`
	Type         string          `json:"type"`
	Manufacturer string          `json:"manufacturer"`
	Model        string          `json:"model"`
	Description  string          `json:"description"`
	Firmware     string          `json:"firmware"`
	Icon         string          `json:"icon"`
	Capabilities json.RawMessage `json:"capabilities"`
	Inputs       json.RawMessage `json:"inputs"`
}

type refreshRequest struct {
	Metadata   *bool    `json:"metadata,omitempty"`
	State      *bool    `json:"state,omitempty"`
	Properties []string `json:"properties"`
}
