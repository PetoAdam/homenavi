package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/PetoAdam/homenavi/zigbee-adapter/internal/app"
)

func main() {
	cfg, err := app.LoadConfig()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}
	application, err := app.New(cfg, slog.Default())
	if err != nil {
		slog.Error("application init failed", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		slog.Error("zigbee-adapter stopped with error", "error", err)
		os.Exit(1)
	}

	slog.Info("zigbee-adapter stopped")
}
