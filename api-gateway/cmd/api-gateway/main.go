package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/PetoAdam/homenavi/api-gateway/internal/app"
	"github.com/PetoAdam/homenavi/shared/envx"
)

func main() {
	cfg, err := app.LoadConfig(os.Args[1:])
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	var handler slog.Handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})
	if envx.String("LOG_FORMAT", "") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	application, err := app.New(cfg, logger)
	if err != nil {
		slog.Error("api-gateway bootstrap failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := application.Run(ctx); err != nil {
		slog.Error("api-gateway stopped with error", "error", err)
		os.Exit(1)
	}
}
