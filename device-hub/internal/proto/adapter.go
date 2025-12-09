package proto

import (
	"context"
	"device-hub/internal/model"
)

// Event represents a canonical event emitted by adapters
type Event struct {
	Type       string
	Device     *model.Device
	State      map[string]any
	RawTopic   string
	RawPayload []byte
}

// Adapter interface for all protocol handlers (zigbee, matter, thread...)
type Adapter interface {
	Name() string
	Start(ctx context.Context) error
	// PublishCommand sends a raw/canonical command
	PublishCommand(ctx context.Context, device *model.Device, cmd map[string]any) error
}
