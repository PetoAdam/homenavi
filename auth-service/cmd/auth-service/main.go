package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/PetoAdam/homenavi/auth-service/internal/app"
	"github.com/PetoAdam/homenavi/shared/envx"
)

func main() {
	cfg, err := app.LoadConfig()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	var handler slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if envx.String("LOG_FORMAT", "") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	application, err := app.New(cfg, logger)
	if err != nil {
		slog.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		slog.Error("auth service stopped with error", "error", err)
		os.Exit(1)
	}
}
