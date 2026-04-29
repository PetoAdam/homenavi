package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/PetoAdam/homenavi/mock-adapter/internal/app"
)

func main() {
	cfg := app.LoadConfig()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	slog.SetDefault(logger)
	application, err := app.New(cfg, logger)
	if err != nil {
		slog.Error("mock-adapter bootstrap failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := application.Run(ctx); err != nil {
		slog.Error("mock-adapter stopped with error", "error", err)
		os.Exit(1)
	}
}
