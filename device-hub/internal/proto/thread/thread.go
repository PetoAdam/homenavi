package thread

import (
	"context"
	"errors"
	"log/slog"

	"device-hub/internal/model"
	"device-hub/internal/mqtt"
	"device-hub/internal/store"
)

// Adapter is a placeholder for upcoming Thread support. It satisfies the
// proto.Adapter interface so we can wire it into the bootstrap path now.
type Adapter struct {
	client  *mqtt.Client
	repo    *store.Repository
	enabled bool
}

// Config toggles the placeholder. Keeping an explicit struct leaves room for
// protocol specific settings later (commissioning data, border router URL...).
type Config struct {
	Enabled bool
}

func New(client *mqtt.Client, repo *store.Repository, cfg Config) *Adapter {
	return &Adapter{client: client, repo: repo, enabled: cfg.Enabled}
}

func (a *Adapter) Name() string { return "thread" }

func (a *Adapter) Start(ctx context.Context) error {
	if !a.enabled {
		slog.Info("thread adapter disabled", "status", "placeholder")
		return nil
	}
	slog.Info("thread adapter placeholder running", "status", "planned")
	return nil
}

func (a *Adapter) PublishCommand(ctx context.Context, device *model.Device, cmd map[string]any) error {
	return errors.New("thread adapter not implemented yet")
}
