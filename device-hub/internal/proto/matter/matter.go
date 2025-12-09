package matter

import (
	"context"
	"errors"
	"log/slog"

	"device-hub/internal/model"
	"device-hub/internal/mqtt"
	"device-hub/internal/store"
)

// Adapter implements proto.Adapter but currently acts as a placeholder until
// Matter support ships. Keeping the structure ready makes it easy to wire the
// real implementation without touching callers.
type Adapter struct {
	client  *mqtt.Client
	repo    *store.Repository
	enabled bool
}

// Config controls whether the placeholder should start. When disabled the
// adapter simply logs that it is idle.
type Config struct {
	Enabled bool
}

// New creates a new placeholder adapter. Dependencies are kept so the real
// implementation can reuse the same signature later.
func New(client *mqtt.Client, repo *store.Repository, cfg Config) *Adapter {
	return &Adapter{client: client, repo: repo, enabled: cfg.Enabled}
}

func (a *Adapter) Name() string { return "matter" }

// Start currently only logs whether the adapter is enabled. Returning nil keeps
// the bootstrap path simple until Matter support is implemented.
func (a *Adapter) Start(ctx context.Context) error {
	if !a.enabled {
		slog.Info("matter adapter disabled", "status", "placeholder")
		return nil
	}
	slog.Info("matter adapter placeholder running", "status", "planned")
	return nil
}

// PublishCommand is not available yet; return a well defined error so callers
// can surface a helpful message.
func (a *Adapter) PublishCommand(ctx context.Context, device *model.Device, cmd map[string]any) error {
	return errors.New("matter adapter not implemented yet")
}
