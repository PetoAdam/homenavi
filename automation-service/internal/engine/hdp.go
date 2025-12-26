package engine

import "encoding/json"

// HDPEnvelope is the common HDP v1 envelope implemented in this repo.
// See doc/hdp.md.
type HDPEnvelope struct {
	Schema   string `json:"schema"`
	Type     string `json:"type"`
	TS       int64  `json:"ts"`
	DeviceID string `json:"device_id,omitempty"`
	Corr     string `json:"corr,omitempty"`
}

type HDPState struct {
	HDPEnvelope
	State map[string]any `json:"state"`
}

type HDPCommandResult struct {
	HDPEnvelope
	Success bool   `json:"success"`
	Status  string `json:"status,omitempty"`
	Error   string `json:"error,omitempty"`
}

type HDPCommand struct {
	Schema   string         `json:"schema"`
	Type     string         `json:"type"`
	DeviceID string         `json:"device_id"`
	Command  string         `json:"command"`
	Args     map[string]any `json:"args,omitempty"`
	Corr     string         `json:"corr,omitempty"`
	TS       int64          `json:"ts"`
}

func decodeJSON[T any](b []byte) (T, error) {
	var out T
	err := json.Unmarshal(b, &out)
	return out, err
}
