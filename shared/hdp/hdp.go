package hdp

import (
	"errors"
	"strings"
)

const (
	SchemaV1              = "hdp.v1"
	Root                  = "homenavi/hdp/"
	AdapterHelloTopic     = Root + "adapter/hello"
	AdapterStatusPrefix   = Root + "adapter/status/"
	MetadataPrefix        = Root + "device/metadata/"
	StatePrefix           = Root + "device/state/"
	EventPrefix           = Root + "device/event/"
	CommandPrefix         = Root + "device/command/"
	CommandResultPrefix   = Root + "device/command_result/"
	PairingCommandPrefix  = Root + "pairing/command/"
	PairingProgressPrefix = Root + "pairing/progress/"
)

var ErrTopicPrefixMismatch = errors.New("topic prefix mismatch")

func Topic(prefix, id string) string {
	return strings.TrimRight(prefix, "/") + "/" + strings.Trim(strings.TrimSpace(id), "/")
}

func DeviceIDFromTopic(prefix, topic string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(prefix), "/") + "/"
	if !strings.HasPrefix(topic, base) {
		return "", ErrTopicPrefixMismatch
	}
	id := strings.Trim(strings.TrimPrefix(topic, base), "/")
	if id == "" {
		return "", errors.New("empty device id")
	}
	return id, nil
}

type Envelope struct {
	Schema   string `json:"schema"`
	Type     string `json:"type"`
	TS       int64  `json:"ts"`
	DeviceID string `json:"device_id,omitempty"`
	Corr     string `json:"corr,omitempty"`
}

type State struct {
	Envelope
	State map[string]any `json:"state"`
}

type Command struct {
	Envelope
	Command string         `json:"command"`
	Args    map[string]any `json:"args,omitempty"`
}

type CommandResult struct {
	Envelope
	Success bool   `json:"success"`
	Status  string `json:"status,omitempty"`
	Error   string `json:"error,omitempty"`
}
